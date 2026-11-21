//go:build !finalizationoracle

package engine

import (
	"github.com/MongooseMoo/barn/engine/internal/finalization"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// The finalization oracle (finalization_oracle_on.go) is compiled in only with
// -tags finalizationoracle.

const finalizationOracleEnabled = false

func (s *Runtime) oracleNoteFastDead(types.Value) {}

func (s *Runtime) oraclePendingLocked() bool { return false }

func (s *Runtime) oracleCheck(map[types.ObjID]struct{}, []types.Value, []finalization.PendingWaif, []vm.AnonGCRequest) {
}

// FinalizationOracleCounts reports the oracle's tallies; all zero without it.
func FinalizationOracleCounts() (recorded, checked, disagreed int64) { return 0, 0, 0 }
