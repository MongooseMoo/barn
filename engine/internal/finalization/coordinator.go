// Package finalization owns the engine's private execution-liveness and
// deferred-finalization coordination state. It does not define MOO-visible GC
// behavior; engine.Runtime remains the composition root for that behavior.
package finalization

import (
	"sync"
	"time"

	"github.com/MongooseMoo/barn/kernel"
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
	Ctx         *kernel.TaskContext
	Task        *task.Task
	OwnRefs     []types.Value
	DirectRoots vm.DirectFinalizationRoots
}

// Coordinator owns the state connecting physical VM execution, root capture,
// deferred collection, and shutdown publication. Fields are exposed only to
// the parent engine tree; Go's internal-package boundary keeps this private.
type Coordinator struct {
	ExecutingTasks         map[int64]int
	ExecutionContexts      map[*kernel.TaskContext]map[int64]int
	SweepContexts          map[*kernel.TaskContext]int
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
	PendingShutdownRoots        []types.Value
	PendingWaifs                []PendingWaif
	PendingAnonGC               []vm.AnonGCRequest
	LastGCSweep                 time.Time
	LastGCCost                  time.Duration
}

// NewCoordinator creates an empty coordinator.
func NewCoordinator() Coordinator {
	return Coordinator{
		ExecutingTasks:    make(map[int64]int),
		ExecutionContexts: make(map[*kernel.TaskContext]map[int64]int),
		SweepContexts:     make(map[*kernel.TaskContext]int),
		ShutdownReady:     make(chan struct{}),
	}
}
