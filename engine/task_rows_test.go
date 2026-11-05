package engine

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// kill_task() and resume() read the target's row in the store and stage a
// write to it. Two writers of one row are an ordinary commit conflict: the
// loser re-runs and reads what the winner wrote.

func newRowTask(t *testing.T, s *Runtime, id int64, owner types.ObjID, code string) *Task {
	t.Helper()
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	tk := NewTaskFull(id, owner, compileTestProgram(t, s.registry, code), ticks, seconds)
	tk.Context.IsWizard = true
	s.populateContextDependencies(tk.Context)
	tk.ForkCreator = s
	return tk
}

// registerRowTask registers a queued task, which gives it its row.
func registerRowTask(t *testing.T, s *Runtime, id int64, owner types.ObjID, code string) *Task {
	t.Helper()
	tk := newRowTask(t, s, id, owner, code)
	tk.StartTime = time.Now()
	tk.SetState(task.TaskQueued)
	s.taskManager.RegisterTask(tk)
	t.Cleanup(func() { s.taskManager.RemoveTaskIf(tk.ID, tk) })
	return tk
}

// suspendRowTarget registers a task and runs it to its suspend().
func suspendRowTarget(t *testing.T, s *Runtime, id int64, owner types.ObjID, code string) *Task {
	t.Helper()
	target := registerRowTask(t, s, id, owner, code)
	if err := s.runTask(target); err != nil {
		t.Fatalf("target runTask failed: %v", err)
	}
	if got := target.GetState(); got != task.TaskSuspended {
		t.Fatalf("target state = %v, want suspended", got)
	}
	return target
}

// runOtherOnce describes a builtin that, the first time it is called, runs
// *other to the end on another goroutine and waits for it. Later calls (from a
// re-run attempt) do nothing.
func runOtherOnce(t *testing.T, s **Runtime, other **Task) builtins.Descriptor {
	t.Helper()
	var once sync.Once
	var fn builtins.BuiltinFunc = func(*builtins.Execution, []types.Value) types.Result {
		failed := false
		once.Do(func() {
			done := make(chan error, 1)
			go func() { done <- (*s).runTask(*other) }()
			select {
			case err := <-done:
				failed = err != nil
			case <-time.After(10 * time.Second):
				failed = true
			}
		})
		if failed {
			return types.Err(types.E_QUOTA)
		}
		return types.Ok(types.NewInt(0))
	}
	return testBuiltinSlot("run_other", 0, 0, []int64{}, &fn)
}

func assertNoGateTaken(t *testing.T, s *Runtime, store *dbstore.Store) {
	t.Helper()
	if holds := s.GateHolds(); len(holds) != 0 {
		t.Fatalf("gate holds = %+v, want none", holds)
	}
	if got := store.CommitEscalations(); got != 0 {
		t.Fatalf("commit escalations = %d, want 0", got)
	}
	assertCommitGateReleased(t, store)
}

// resumeVia runs a wizard task that calls resume(id, value) and returns what
// the call returned.
func resumeVia(t *testing.T, s *Runtime, id int64, value string) string {
	t.Helper()
	resumer := newRowTask(t, s, s.newTaskID()+1_000_000, 0, fmt.Sprintf("return `resume(%d, %s) ! ANY';", id, value))
	if err := s.runTask(resumer); err != nil {
		t.Fatalf("resumer runTask failed: %v", err)
	}
	return resultString(resumer)
}

func resultString(tk *Task) string {
	if tk.Result.Flow != types.FlowReturn {
		return "flow " + tk.Result.Error.String()
	}
	return tk.Result.Val.String()
}

// Two killers of one task: the one that commits first kills it; the other
// loses its commit, re-runs, and finds the task gone.
func TestTwoKillersOfOneTaskOneWins(t *testing.T) {
	store := newBoundaryTestStore(t)
	var s *Runtime
	var second *Task
	s = newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 2, runOtherOnce(t, &s, &second))
	defer s.Stop()
	victim := suspendRowTarget(t, s, 6101, 5, "return suspend();")
	second = newRowTask(t, s, 6103, 7, "return `kill_task(6101) ! ANY';")
	first := newRowTask(t, s, 6102, 6, `
r = `+"`kill_task(6101) ! ANY'"+`;
run_other();
return r;
`)
	if err := s.runTask(first); err != nil {
		t.Fatalf("first runTask failed: %v", err)
	}
	if got := resultString(second); got != "0" {
		t.Fatalf("second killer = %s, want 0: it committed first", got)
	}
	if got := resultString(first); got != "E_INVARG" {
		t.Fatalf("first killer = %s, want E_INVARG from its re-run", got)
	}
	if got := store.CommitRetries(); got != 1 {
		t.Fatalf("commit retries = %d, want 1", got)
	}
	if victim.GetState() != task.TaskKilled {
		t.Fatalf("victim state = %v, want killed", victim.GetState())
	}
	assertNoGateTaken(t, s, store)
}

// A resume() that commits while the target's timer-woken slice runs makes that
// slice lose its commit; the re-run's suspend() returns the resume's value,
// the order in which the resume came first.
func TestResumeRacingTimerWakeDeliversResumeValue(t *testing.T) {
	store := newBoundaryTestStore(t)
	var s *Runtime
	var resumer *Task
	s = newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 2, runOtherOnce(t, &s, &resumer))
	defer s.Stop()
	target := suspendRowTarget(t, s, 6201, 5, `
v = suspend(3600);
run_other();
return v;
`)
	target.Mu.Lock()
	target.WakeTime = time.Now().Add(-time.Second) // its timeout is due
	target.Mu.Unlock()
	resumer = newRowTask(t, s, 6202, 5, `return resume(6201, "mine");`)

	s.ProcessReadyTasks()
	if got := target.GetState(); got != task.TaskCompleted {
		t.Fatalf("target state = %v, want completed", got)
	}
	if got := resultString(resumer); got != "0" {
		t.Fatalf("resume() = %s, want 0", got)
	}
	if got := resultString(target); got != `"mine"` {
		t.Fatalf("target's suspend() returned %s, want \"mine\"", got)
	}
	if got := store.CommitRetries(); got != 1 {
		t.Fatalf("commit retries = %d, want 1: the woken slice re-ran once", got)
	}
	assertNoGateTaken(t, s, store)
}

// kill_task() of a task in the middle of a slice: the victim's commit loses
// to the kill, publishes nothing, and the victim stops.
func TestKillOfRunningTaskStopsIt(t *testing.T) {
	store := newBoundaryTestStore(t)
	var s *Runtime
	var killer *Task
	s = newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 2, runOtherOnce(t, &s, &killer))
	defer s.Stop()
	victim := registerRowTask(t, s, 6301, 5, `
#0.write_value = 5;
run_other();
return 1;
`)
	killer = newRowTask(t, s, 6302, 5, "return kill_task(6301);")
	if err := s.runTask(victim); err != nil {
		t.Fatalf("victim runTask failed: %v", err)
	}
	if got := resultString(killer); got != "0" {
		t.Fatalf("kill_task() = %s, want 0", got)
	}
	if victim.GetState() != task.TaskKilled {
		t.Fatalf("victim state = %v, want killed", victim.GetState())
	}
	if written, _ := store.DirectTxn().PropertyValue(0, "write_value"); written.Int() != 0 {
		t.Fatalf("write_value = %v, want 0: the killed slice publishes nothing", written)
	}
	if _, ok := store.TaskRow(6301); ok {
		t.Fatal("the killed task still has a row")
	}
	assertNoGateTaken(t, s, store)
}

// A second resume() before the target runs replaces the value, whether it is
// in the same slice or a later one; once the target has run, resume() of it
// is E_INVARG.
func TestResumeReplacesPendingValue(t *testing.T) {
	store := newBoundaryTestStore(t)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
	defer s.Stop()
	target := suspendRowTarget(t, s, 6401, 5, "return suspend();")

	first := newRowTask(t, s, 6402, 5, "return {resume(6401, 1), resume(6401, 2)};")
	if err := s.runTask(first); err != nil {
		t.Fatalf("first runTask failed: %v", err)
	}
	if got := resultString(first); got != "{0, 0}" {
		t.Fatalf("first resumer = %s, want {0, 0}", got)
	}
	second := newRowTask(t, s, 6403, 5, "return resume(6401, 3);")
	if err := s.runTask(second); err != nil {
		t.Fatalf("second runTask failed: %v", err)
	}
	if got := resultString(second); got != "0" {
		t.Fatalf("second resumer = %s, want 0", got)
	}
	s.ProcessReadyTasks()
	if got := target.GetState(); got != task.TaskCompleted {
		t.Fatalf("target state = %v, want completed", got)
	}
	if got := resultString(target); got != "3" {
		t.Fatalf("target's suspend() returned %s, want 3 from the last resume()", got)
	}
	late := newRowTask(t, s, 6404, 5, "return `resume(6401) ! ANY';")
	if err := s.runTask(late); err != nil {
		t.Fatalf("late runTask failed: %v", err)
	}
	if got := resultString(late); got != "E_INVARG" {
		t.Fatalf("resume() of a finished task = %s, want E_INVARG", got)
	}
	assertNoGateTaken(t, s, store)
}

// A fork child's row is written by its parent's commit, so the parent can kill
// the child in the same slice and the child never runs.
func TestKillOfOwnForkInSameSlice(t *testing.T) {
	store := newBoundaryTestStore(t)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
	defer s.Stop()
	parent := newRowTask(t, s, 6501, 5, `
fork id (3600)
  #0.write_value = 1;
endfork
return {id, kill_task(id)};
`)
	if err := s.runTask(parent); err != nil {
		t.Fatalf("parent runTask failed: %v", err)
	}
	if parent.Result.Flow != types.FlowReturn || parent.Result.Val.Get(2).String() != "0" {
		t.Fatalf("parent = %s, want {id, 0}", resultString(parent))
	}
	child := s.taskManager.GetTask(parent.Result.Val.Get(1).Int())
	if child == nil || child.GetState() != task.TaskKilled {
		t.Fatalf("child = %v, want killed", child)
	}
	defer removeTasksForOwner(s, 5)
	assertNoGateTaken(t, s, store)
}

// A fork child's row exists once its parent's commit publishes it: another task
// that looks before then finds no such task, as if it ran before the parent.
func TestForkRowPublishedByParentCommit(t *testing.T) {
	store := newBoundaryTestStore(t)
	var s *Runtime
	var during, after string
	var look builtins.BuiltinFunc = func(_ *builtins.Execution, args []types.Value) types.Result {
		during = rowLookup(t, s, args[0].Int())
		return types.Ok(types.NewInt(0))
	}
	s = newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 2, testBuiltinSlot("look_from_other", 1, 1, []int64{int64(types.TYPE_INT)}, &look))
	defer s.Stop()
	defer removeTasksForOwner(s, 5)
	parent := newRowTask(t, s, 6701, 5, `
fork id (3600)
endfork
look_from_other(id);
return id;
`)
	if err := s.runTask(parent); err != nil {
		t.Fatalf("parent runTask failed: %v", err)
	}
	child := parent.Result.Val.Int()
	after = rowLookup(t, s, child)
	if during != "absent" || after != "live" {
		t.Fatalf("child seen by another task before / after the parent's commit = %s / %s, want absent / live", during, after)
	}
}

// rowLookup runs, on another goroutine, a task that asks kill_task() whether
// task id exists without killing it, as LambdaCore's $code_utils:task_valid
// does: a non-wizard probe gets E_PERM for a live task of another owner and
// E_INVARG for none.
func rowLookup(t *testing.T, s *Runtime, id int64) string {
	t.Helper()
	probe := newRowTask(t, s, s.newTaskID()+2_000_000, 99, fmt.Sprintf("return `kill_task(%d) ! ANY';", id))
	probe.Context.IsWizard = false
	probe.Context.Programmer = 99
	done := make(chan error, 1)
	go func() { done <- s.runTask(probe) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("probe runTask failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("probe did not finish")
	}
	switch got := resultString(probe); got {
	case "E_PERM":
		return "live"
	case "E_INVARG":
		return "absent"
	default:
		return got
	}
}

// Permissions and the cases resume() cannot reach.
func TestKillAndResumeRefusals(t *testing.T) {
	store := newBoundaryTestStore(t)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
	defer s.Stop()
	suspendRowTarget(t, s, 6601, 5, "return suspend();")
	registerRowTask(t, s, 6602, 5, "return 0;") // registered, not suspended

	other := newRowTask(t, s, 6603, 9, "return {`kill_task(6601) ! ANY', `resume(6601) ! ANY', `resume(6602) ! ANY', `resume(6603) ! ANY', `kill_task(6603) ! ANY', `kill_task(999999) ! ANY'};")
	other.Context.IsWizard = false
	other.Context.Programmer = 9
	if err := s.runTask(other); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if got := resultString(other); got != "{E_PERM, E_PERM, E_INVARG, E_INVARG, E_INTRPT, E_INVARG}" {
		t.Fatalf("result = %s", got)
	}
}
