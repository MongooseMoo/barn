// Package finalization owns the engine's private execution-liveness and
// deferred-finalization coordination state. It does not define MOO-visible GC
// behavior; engine.Runtime remains the composition root for that behavior.
package finalization

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// PendingWaif is one deferred waif liveness request and its captured roots.
// DirectRoots are the owning VM's direct finalizable references, captured at
// defer time; they are canonicalized against persistent state only if a
// shutdown claims them before the deferred sweep runs.
type PendingWaif struct {
	Waif        types.Value
	Ctx         *task.Context
	Task        *task.State
	OwnRefs     []types.Value
	DirectRoots vm.DirectFinalizationRoots
}

// Coordinator owns the state connecting physical VM execution, root capture,
// deferred collection, and shutdown publication. Fields are exposed only to
// the parent engine tree; Go's internal-package boundary keeps this private.
type Coordinator struct {
	ExecutingTasks         map[int64]int
	ExecutionContexts      map[*task.Context]map[int64]int
	SweepContexts          map[*task.Context]int
	ExecutionStartObserver func()

	SweepMu   sync.Mutex
	VMStartMu sync.Mutex

	Mu                          sync.Mutex
	ShutdownRequested           bool
	ShutdownPublishing          bool
	ShutdownPublished           bool
	ShutdownReady               chan struct{}
	ActiveFinalizationProducers int
	GCRunning                   bool
	FinalizationHeld            bool
	PendingShutdownRoots        []types.Value
	PendingWaifs                []PendingWaif
	PendingAnonGC               []vm.AnonGCRequest
	LastGCSweep                 time.Time
	LastGCCost                  time.Duration
	PendingSince                time.Time
	RetryAfter                  time.Time
	MaintenanceWake             chan struct{}
	MaintenanceDone             chan struct{}
	// QueuedSeq numbers each deferred request as it is queued; SweptSeq is the
	// highest number a completed sweep (or shutdown hand-off) has drained.
	// WaitSeq is the highest number a suspended task is held for, so the
	// sweepers run without the cheap-sweep throttle until SweptSeq reaches it.
	// WaiterUnserved is set when a slice-boundary flush left such a task held;
	// the maintenance worker then sweeps at once instead of on its cadence.
	QueuedSeq      uint64
	WaitSeq        uint64
	SweptSeq       atomic.Uint64
	WaiterUnserved bool
}

// NewCoordinator creates an empty coordinator.
func NewCoordinator() Coordinator {
	return Coordinator{
		ExecutingTasks:    make(map[int64]int),
		ExecutionContexts: make(map[*task.Context]map[int64]int),
		SweepContexts:     make(map[*task.Context]int),
		ShutdownReady:     make(chan struct{}),
		MaintenanceWake:   make(chan struct{}, 1),
		MaintenanceDone:   make(chan struct{}),
	}
}
