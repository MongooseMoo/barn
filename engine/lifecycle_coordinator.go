package engine

import (
	"sync"
	"time"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// lifecycleCoordinator owns the state machine that connects physical VM
// execution to deferred finalization. Keeping the barriers and the state they
// protect together makes their lock order visible in one place:
// sweepMu -> vmStartMu -> Runtime.mu -> task locks.
//
// Runtime owns the task catalog. Operations which need task roots take a
// snapshot through Runtime while holding the coordinator barriers; the
// coordinator deliberately does not maintain a second catalog.
type lifecycleCoordinator struct {
	// Physical execution and context provenance. Runtime.mu protects these maps
	// because root snapshots must atomically inspect them and the task catalog.
	executingTasks         map[int64]int
	executionContexts      map[*kernel.TaskContext]map[int64]int
	sweepContexts          map[*kernel.TaskContext]int
	executionStartObserver func() // test-only hook before vmStartMu acquisition

	// sweepMu serializes sweeps; vmStartMu prevents a VM from becoming visible
	// while roots are captured and recycle hooks execute.
	sweepMu   sync.Mutex
	vmStartMu sync.Mutex

	// mu protects deferred work, throttling, producer admission, and shutdown
	// publication. These fields are one ownership-transfer state machine.
	mu                          sync.Mutex
	shutdownRequested           bool
	shutdownPublishing          bool
	shutdownPublished           bool
	shutdownReady               chan struct{}
	activeFinalizationProducers int
	gcRunning                   bool
	pendingShutdownRoots        []types.Value
	pendingWaifs                []pendingWaifEntry
	pendingAnonGC               []vm.AnonGCRequest
	lastGCSweep                 time.Time
	lastGCCost                  time.Duration
}

func newLifecycleCoordinator() lifecycleCoordinator {
	return lifecycleCoordinator{
		executingTasks:    make(map[int64]int),
		executionContexts: make(map[*kernel.TaskContext]map[int64]int),
		sweepContexts:     make(map[*kernel.TaskContext]int),
		shutdownReady:     make(chan struct{}),
	}
}
