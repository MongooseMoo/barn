//go:build finalizationoracle

package engine

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/MongooseMoo/barn/engine/internal/finalization"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// The finalization oracle re-checks every "dead" verdict of the boundary fast
// path (finalization_fast.go) against the batched sweep's full root walk: the
// next sweep after a verdict must find nothing that references the recycled
// value. A disagreement means the fast path recycled a value something still
// held, and fails the run.

const finalizationOracleEnabled = true

var finalizationOracle struct {
	mu        sync.Mutex
	waifs     []types.Value
	anons     []types.ObjID
	recorded  atomic.Int64
	checked   atomic.Int64
	disagreed atomic.Int64
}

func (s *Runtime) oracleNoteFastDead(value types.Value) {
	finalizationOracle.mu.Lock()
	if value.Type() == types.TYPE_WAIF {
		finalizationOracle.waifs = append(finalizationOracle.waifs, value)
	} else {
		finalizationOracle.anons = append(finalizationOracle.anons, value.ID())
	}
	finalizationOracle.mu.Unlock()
	finalizationOracle.recorded.Add(1)
	// Make sure a sweep comes to check it.
	s.lifecycle.Mu.Lock()
	s.noteDeferredGCQueuedLocked()
	s.lifecycle.Mu.Unlock()
}

func (s *Runtime) oraclePendingLocked() bool {
	finalizationOracle.mu.Lock()
	defer finalizationOracle.mu.Unlock()
	return len(finalizationOracle.waifs) != 0 || len(finalizationOracle.anons) != 0
}

// oracleCheck runs inside a sweep, with every VM parked or idle and the roots
// captured: siblingAnon and siblingWaifs are the live tasks' references, and
// the batches carry the references of tasks that finished.
func (s *Runtime) oracleCheck(siblingAnon map[types.ObjID]struct{}, siblingWaifs []types.Value, waifBatch []finalization.PendingWaif, anonBatch []vm.AnonGCRequest) {
	finalizationOracle.mu.Lock()
	waifs, anons := finalizationOracle.waifs, finalizationOracle.anons
	finalizationOracle.waifs, finalizationOracle.anons = nil, nil
	finalizationOracle.mu.Unlock()
	if len(waifs) == 0 && len(anons) == 0 {
		return
	}
	var failures []string
	if len(waifs) != 0 {
		roots := append([]types.Value(nil), siblingWaifs...)
		for _, entry := range waifBatch {
			roots = append(roots, entry.OwnRefs...)
		}
		live := s.liveWaifs(roots)
		for _, waif := range waifs {
			finalizationOracle.checked.Add(1)
			if live.Has(waif) {
				failures = append(failures, fmt.Sprintf("WAIF %s of class #%d", waif.WaifIdentity(), waif.Class()))
			}
		}
	}
	if len(anons) != 0 {
		seeds := make(map[types.ObjID]struct{}, len(siblingAnon))
		for id := range siblingAnon {
			seeds[id] = struct{}{}
		}
		for _, request := range anonBatch {
			for id := range request.OwnRefs {
				seeds[id] = struct{}{}
			}
		}
		referenced := s.store.ReferencedAnonymousIDs(seeds)
		for _, id := range anons {
			finalizationOracle.checked.Add(1)
			if _, held := referenced[id]; held {
				failures = append(failures, fmt.Sprintf("anonymous #%d", id))
			}
		}
	}
	if len(failures) != 0 {
		finalizationOracle.disagreed.Add(int64(len(failures)))
		slog.Error("finalization oracle: fast path recycled a value the sweep finds held", slog.Any("values", failures))
		panic(fmt.Sprintf("finalization oracle: fast path recycled held values: %v", failures))
	}
}

// FinalizationOracleCounts reports the oracle's tallies.
func FinalizationOracleCounts() (recorded, checked, disagreed int64) {
	return finalizationOracle.recorded.Load(), finalizationOracle.checked.Load(), finalizationOracle.disagreed.Load()
}
