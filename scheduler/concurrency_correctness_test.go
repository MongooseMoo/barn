package scheduler

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"barn/config"
	dbstore "barn/db/store"
	"barn/task"
	"barn/types"
)

// runTasksThroughPool queues every task and drains them through the worker pool,
// mirroring how ProcessReadyTasks dispatches a ready batch in production.
func runTasksThroughPool(t *testing.T, s *Scheduler, tasks []*task.Task) {
	t.Helper()
	for _, tk := range tasks {
		s.QueueTask(tk)
	}
	if ran := s.ProcessReadyTasks(); ran != len(tasks) {
		t.Fatalf("ProcessReadyTasks ran %d tasks, want %d", ran, len(tasks))
	}
}

// TestOptimisticConflictingWritersAreSerializable is the load-bearing correctness
// test for optimistic batching. N fresh AST tasks each increment the SAME property
// through an opaque verb call (so every task has an "unknown" footprint and is
// co-scheduled optimistically). They genuinely conflict at commit. The invariant:
// the result must be identical to running them one at a time — the counter ends at
// exactly N (no lost updates) and not a single task surfaces a spurious E_INVARG
// (every conflict is absorbed by retry). If optimism were unsound, the counter
// would land below N or tasks would error.
func TestOptimisticConflictingWritersAreSerializable(t *testing.T) {
	cores := runtime.GOMAXPROCS(0)
	if cores < 2 {
		t.Skipf("need >=2 GOMAXPROCS to exercise concurrent commits, have %d", cores)
	}

	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("counter", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}
	// Opaque increment: a verb call makes the task footprint "unknown" so it takes
	// the optimistic co-scheduling path rather than the proven-commute path.
	bump := dbstore.NewVerb(
		"bump",
		[]string{"bump"},
		0,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "none"},
		[]string{"#0.counter = #0.counter + 1;", "return #0.counter;"},
	)
	if _, errCode := store.AddVerb(0, bump); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	s := newSchedulerWithWorkerCount(store, config.Options{}, cores)
	defer s.Stop()
	defer removeTasksForOwner(s, 0)

	const n = 64
	tasks := make([]*task.Task, n)
	for k := 0; k < n; k++ {
		tk := task.NewTaskFull(int64(6000+k), types.ObjID(0), compileTestProgram(t, s.registry, "return #0:bump();"), 1<<50, 1e9)
		s.populateTaskContextDependencies(tk.Context)
		tk.Context.IsWizard = true
		tk.StartTime = time.Now().Add(-time.Second)
		tk.Done = make(chan struct{})
		tk.ForkCreator = s
		tasks[k] = tk
	}

	runTasksThroughPool(t, s, tasks)

	for _, tk := range tasks {
		if tk.Result.Flow != types.FlowReturn {
			t.Fatalf("task %d flow = %v err=%v, want return (optimistic conflict leaked?)", tk.ID, tk.Result.Flow, tk.Result.Error)
		}
	}

	final, errCode := store.PropertyValue(0, "counter")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue failed: %s", errCode)
	}
	if final.Type() != types.TYPE_INT {
		t.Fatalf("counter = %T, want int", final)
	}
	if final.Int() != int64(n) {
		t.Fatalf("counter = %d, want %d (lost updates under optimistic concurrency)", final.Int(), n)
	}
}

// TestOptimisticConcurrentSuspendsNoRace probes the suspend path specifically: many
// fresh AST tasks call suspend(0), so they batch optimistically and run concurrently.
// When one saves its VM on suspend (t.BytecodeVM = ...) while a sibling's orphan-GC
// scans live task VMs, an unsynchronized field access would surface here under -race.
// Run with `go test -race -run TestOptimisticConcurrentSuspendsNoRace -count=5`.
func TestOptimisticConcurrentSuspendsNoRace(t *testing.T) {
	cores := runtime.GOMAXPROCS(0)
	if cores < 2 {
		t.Skipf("need >=2 GOMAXPROCS, have %d", cores)
	}

	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newSchedulerWithWorkerCount(store, config.Options{}, cores)
	defer s.Stop()
	defer removeTasksForOwner(s, 0)

	const n = 48
	tasks := make([]*task.Task, n)
	for k := 0; k < n; k++ {
		tk := task.NewTaskFull(int64(7000+k), types.ObjID(0), compileTestProgram(t, s.registry, "suspend(0); return 1;"), 1<<50, 1e9)
		s.populateTaskContextDependencies(tk.Context)
		tk.Context.IsWizard = true
		tk.StartTime = time.Now().Add(-time.Second)
		// Deliberately no Done channel: this probe targets the suspend-path data race,
		// not command-completion signaling. (runTaskBatch's close(Done)-on-suspend is a
		// separate lifecycle issue, only reachable once foreground tasks use the pool.)
		tk.ForkCreator = s
		tasks[k] = tk
	}
	for _, tk := range tasks {
		s.QueueTask(tk)
	}
	// Drain: the first pass runs and suspends the batch; later passes resume them.
	for pass := 0; pass < 8; pass++ {
		if s.ProcessReadyTasks() == 0 {
			break
		}
	}
}

// TestOptimisticDisjointLiveMutatorsDoNotCorrupt stresses the one residual hazard:
// fresh AST tasks that mutate the live store directly (add_verb) and therefore set
// LiveStoreMutated. They are co-scheduled optimistically (we cannot know statically
// that they will live-mutate). Each targets a DISJOINT object, so there is no real
// conflict; the test asserts every task completes, every verb lands, and — under
// `go test -race` — that concurrent live mutation is data-race free.
func TestOptimisticDisjointLiveMutatorsDoNotCorrupt(t *testing.T) {
	cores := runtime.GOMAXPROCS(0)
	if cores < 2 {
		t.Skipf("need >=2 GOMAXPROCS, have %d", cores)
	}

	store := dbstore.NewStore()
	wiz := dbstore.NewObjectBuilder(3)
	wiz.SetOwner(3)
	wiz.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(wiz.Build()); err != nil {
		t.Fatalf("Add wizard failed: %v", err)
	}

	const n = 32
	ids := make([]types.ObjID, n)
	for k := 0; k < n; k++ {
		id, errCode := store.CreateObject(nil, 3, false)
		if errCode != types.E_NONE {
			t.Fatalf("CreateObject failed: %v", errCode)
		}
		ids[k] = id
	}

	s := newSchedulerWithWorkerCount(store, config.Options{}, cores)
	defer s.Stop()
	defer removeTasksForOwner(s, 3)

	tasks := make([]*task.Task, n)
	for k := 0; k < n; k++ {
		code := fmt.Sprintf("add_verb(#%d, {#3, \"rxd\", \"poked\"}, {\"this\", \"none\", \"none\"}); return verbs(#%d);", ids[k], ids[k])
		tk := task.NewTaskFull(int64(6500+k), types.ObjID(3), compileTestProgram(t, s.registry, code), 1<<50, 1e9)
		s.populateTaskContextDependencies(tk.Context)
		tk.Context.IsWizard = true
		tk.Programmer = 3
		tk.Context.Programmer = 3
		tk.StartTime = time.Now().Add(-time.Second)
		tk.Done = make(chan struct{})
		tk.ForkCreator = s
		tasks[k] = tk
	}

	runTasksThroughPool(t, s, tasks)

	for _, tk := range tasks {
		if tk.Result.Flow != types.FlowReturn {
			t.Fatalf("task %d flow = %v err=%v, want return", tk.ID, tk.Result.Flow, tk.Result.Error)
		}
	}
	for _, id := range ids {
		names, errCode := store.VerbNames(id)
		if errCode != types.E_NONE {
			t.Fatalf("VerbNames(#%d) failed: %s", id, errCode)
		}
		if len(names) != 1 || names[0] != "poked" {
			t.Fatalf("object #%d verb names = %v, want [poked]", id, names)
		}
	}
}
