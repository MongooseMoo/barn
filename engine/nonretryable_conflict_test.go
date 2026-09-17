package engine

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Issue #296: a slice that cannot be re-executed after a lost commit (a forked
// task's first run, a resumed task, a forked task's inline suspend(0) yields)
// used to fall through to `result = types.Err(E_INVARG)` and hand MOO code a
// frameless E_INVARG no serial execution produces. Toast structurally cannot
// produce it. These tests pin the two remedies: a forked first run is re-run
// from the fork statement like a fresh task, and any slice that truly cannot be
// re-run holds the exclusive commit gate so it cannot lose.

func newConflictTestStore(t *testing.T) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("v", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}
	return store
}

func readRootV(t *testing.T, store *dbstore.Store) int64 {
	t.Helper()
	value, errCode := store.DirectTxn().PropertyValue(0, "v")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue(#0.v) failed: %s", errCode)
	}
	return value.Int()
}

// competingWriter is an ordinary commit-based writer racing the task under test.
// start_competitor() snapshots #0.v, stages v+100 and commits on another
// goroutine. With the slice gated, that commit parks on the gate until the slice
// commits (so the builtin's wait times out and the competitor later loses);
// without the gate it publishes within microseconds and the slice's own read of
// #0.v is stale at commit time, deterministically.
type competingWriter struct {
	done   chan struct{}
	result types.ErrorCode
}

func registerCompetingWriter(s *Runtime, store *dbstore.Store) *competingWriter {
	c := &competingWriter{done: make(chan struct{})}
	s.registry.Register("start_competitor", func(ctx *builtins.Execution, args []types.Value) types.Result {
		tx := store.BeginReadOnly(0)
		cur, errCode := tx.PropertyValue(0, "v")
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		if errCode := tx.SetPropertyValue(0, "v", types.NewInt(cur.Int()+100)); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		go func() {
			c.result = tx.Commit()
			close(c.done)
		}()
		select {
		case <-c.done:
		case <-time.After(200 * time.Millisecond):
		}
		return types.Ok(types.NewInt(0))
	})
	return c
}

func (c *competingWriter) wait(t *testing.T) types.ErrorCode {
	t.Helper()
	select {
	case <-c.done:
		return c.result
	case <-time.After(5 * time.Second):
		t.Fatal("competing commit never finished: the commit gate was not released")
		return types.E_NONE
	}
}

func findForkedChild(s *Runtime, owner types.ObjID) *task.Task {
	for _, candidate := range s.taskManager.Snapshot() {
		if candidate != nil && candidate.IsForked && candidate.Owner == owner {
			return candidate
		}
	}
	return nil
}

func runUntilTerminal(t *testing.T, s *Runtime, target *task.Task) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if state := target.GetState(); state == task.TaskCompleted || state == task.TaskKilled {
			return
		}
		s.ProcessReadyTasks()
	}
	t.Fatalf("task %d did not finish: state %v", target.ID, target.GetState())
}

// A forked task's first run that loses validation is re-run from the fork
// statement instead of failing with E_INVARG: its VM is a pure function of the
// fork record, so nothing distinguishes it from a fresh task.
func TestForkedFirstRunRetriesLostCommit(t *testing.T) {
	store := newConflictTestStore(t)
	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()

	conflictCalls := 0
	s.registry.Register("conflict_once", func(ctx *builtins.Execution, args []types.Value) types.Result {
		conflictCalls++
		if conflictCalls == 1 {
			// Another task's commit lands after this slice's snapshot. Deliberately
			// not a live mutation: this is a retryable conflict.
			if errCode := store.DirectTxn().SetPropertyValue(0, "v", types.NewInt(50)); errCode != types.E_NONE {
				return types.Err(errCode)
			}
		}
		return types.Ok(types.NewInt(0))
	})

	owner := types.ObjID(7801)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	parent := task.NewTaskFull(3101, owner, compileTestProgram(t, s.registry, `
fork (0)
  before = #0.v;
  conflict_once();
  #0.v = before + 1;
endfork
return 0;
`), ticks, seconds)
	parent.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)

	if err := s.runTask(parent); err != nil {
		t.Fatalf("runTask(parent) failed: %v", err)
	}
	child := findForkedChild(s, owner)
	if child == nil {
		t.Fatal("fork did not create a child task")
	}
	runUntilTerminal(t, s, child)

	if child.GetState() != task.TaskCompleted || child.Result.Flow == types.FlowException {
		t.Fatalf("forked child = state %v flow %v err %v, want completed without exception", child.GetState(), child.Result.Flow, child.Result.Error)
	}
	if conflictCalls != 2 {
		t.Fatalf("fork body executions = %d, want 2 (one lost commit, one retry)", conflictCalls)
	}
	if got := store.CommitRetries(); got != 1 {
		t.Fatalf("commit retries = %d, want 1", got)
	}
	if got := readRootV(t, store); got != 51 {
		t.Fatalf("#0.v = %d, want 51 (retry read the competing write)", got)
	}
}

// A resumed task's slice cannot be re-executed, so it holds the exclusive commit
// gate: a competing commit-based writer waits, and it is the competitor that
// loses validation afterwards, never the slice.
func TestResumedSliceCannotLoseCommitToConcurrentWriter(t *testing.T) {
	store := newConflictTestStore(t)
	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()
	competitor := registerCompetingWriter(s, store)

	owner := types.ObjID(7802)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3102, owner, compileTestProgram(t, s.registry, `
suspend(0);
start_competitor();
#0.v = #0.v + 1;
return #0.v;
`), ticks, seconds)
	queued.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	runUntilTerminal(t, s, queued)

	if queued.Result.Flow != types.FlowReturn {
		t.Fatalf("resumed task = flow %v err %v, want return", queued.Result.Flow, queued.Result.Error)
	}
	if got := queued.Result.Val.Int(); got != 1 {
		t.Fatalf("resumed task returned %d, want 1", got)
	}
	if code := competitor.wait(t); code != types.E_INVARG {
		t.Fatalf("competing commit = %v, want E_INVARG (it must be the one that loses)", code)
	}
	if got := readRootV(t, store); got != 1 {
		t.Fatalf("#0.v = %d, want 1", got)
	}
	if got := store.CommitEscalations(); got == 0 {
		t.Fatal("resumed slice did not take the commit gate")
	}
}

// A forked task's suspend(0) resumes inline inside the same runTask; those
// slices are committed by a separate path that never retried. They hold the
// gate too.
func TestForkedInlineYieldCannotLoseCommitToConcurrentWriter(t *testing.T) {
	store := newConflictTestStore(t)
	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()
	competitor := registerCompetingWriter(s, store)

	owner := types.ObjID(7803)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	parent := task.NewTaskFull(3103, owner, compileTestProgram(t, s.registry, `
fork (0)
  suspend(0);
  start_competitor();
  #0.v = #0.v + 1;
endfork
return 0;
`), ticks, seconds)
	parent.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)

	if err := s.runTask(parent); err != nil {
		t.Fatalf("runTask(parent) failed: %v", err)
	}
	child := findForkedChild(s, owner)
	if child == nil {
		t.Fatal("fork did not create a child task")
	}
	runUntilTerminal(t, s, child)

	if child.GetState() != task.TaskCompleted || child.Result.Flow == types.FlowException {
		t.Fatalf("forked child = state %v flow %v err %v, want completed without exception", child.GetState(), child.Result.Flow, child.Result.Error)
	}
	if code := competitor.wait(t); code != types.E_INVARG {
		t.Fatalf("competing commit = %v, want E_INVARG (it must be the one that loses)", code)
	}
	if got := readRootV(t, store); got != 1 {
		t.Fatalf("#0.v = %d, want 1", got)
	}
}

// dump_database() inside a gate-holding slice must not take the gate again
// (the checkpoint walk locks it, and its hook tasks commit through it). The
// checkpoint is deferred until the slice's commit is published and the gate is
// released.
func TestDumpDatabaseInGatedSliceDefersUntilGateReleased(t *testing.T) {
	store := newConflictTestStore(t)
	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()

	checkpoints := 0
	gateFree := false
	sawCommittedWrite := false
	configureTestHost(s.Session(), func(host *builtins.Host) {
		host.Checkpoint = func() error {
			checkpoints++
			acquired := make(chan struct{})
			go func() {
				store.EscalationLock()
				store.EscalationUnlock()
				close(acquired)
			}()
			select {
			case <-acquired:
				gateFree = true
			case <-time.After(2 * time.Second):
			}
			sawCommittedWrite = readRootV(t, store) == 7
			return nil
		}
	})

	owner := types.ObjID(7804)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3104, owner, compileTestProgram(t, s.registry, `
suspend(0);
#0.v = 7;
return dump_database();
`), ticks, seconds)
	queued.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	runUntilTerminal(t, s, queued)

	if queued.Result.Flow != types.FlowReturn || queued.Result.Val.Int() != 0 {
		t.Fatalf("task = flow %v err %v val %v, want return 0", queued.Result.Flow, queued.Result.Error, queued.Result.Val)
	}
	if checkpoints != 1 {
		t.Fatalf("checkpoints = %d, want exactly 1", checkpoints)
	}
	if !gateFree {
		t.Fatal("checkpoint ran while the slice still held the commit gate")
	}
	if !sawCommittedWrite {
		t.Fatal("checkpoint ran before the slice's write was published")
	}
}
