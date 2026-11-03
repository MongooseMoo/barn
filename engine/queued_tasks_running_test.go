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

// Toast runs one task at a time. A forked task, or a task resumed from a
// suspension, waits in a background queue until it runs, and bf_queued_tasks
// (tasks.cc) lists every task there. Barn runs such a task's slice beside the
// observer's, so queued_tasks() lists it while it runs. A command task's first
// slice is Toast's unstarted input, which is not listed, and the caller is the
// current task, in no queue.

type blockingRuntime struct {
	s       *Runtime
	entered chan struct{}
	release chan struct{}
}

func newBlockingRuntime(t *testing.T) *blockingRuntime {
	t.Helper()
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}
	blocker := dbstore.NewVerb([]string{"blocker"}, 0, dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"block_here();", "return 1;"})
	if _, ec := store.AddVerb(0, blocker); ec != types.E_NONE {
		t.Fatalf("AddVerb blocker: %v", ec)
	}
	b := &blockingRuntime{entered: make(chan struct{}), release: make(chan struct{})}
	var blockHere builtins.BuiltinFunc = func(ctx *builtins.Execution, args []types.Value) types.Result {
		b.entered <- struct{}{}
		<-b.release
		return types.Ok(types.NewInt(0))
	}
	b.s = newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 4, testBuiltinSlot("block_here", 0, 0, []int64{}, &blockHere))
	return b
}

func (b *blockingRuntime) newTask(t *testing.T, id int64, owner types.ObjID, code string) *Task {
	t.Helper()
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	tk := NewTaskFull(id, owner, compileTestProgram(t, b.s.registry, code), ticks, seconds)
	tk.Context.IsWizard = true
	b.s.taskManager.RegisterTask(tk)
	return tk
}

// runBlocked runs one slice of tk on another goroutine and returns once it is
// inside block_here(). The returned channel yields runTask's error after release.
func (b *blockingRuntime) runBlocked(t *testing.T, tk *Task) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- b.s.runTask(tk) }()
	select {
	case <-b.entered:
	case err := <-done:
		t.Fatalf("task %d finished its slice without blocking: %v (result %+v)", tk.ID, err, tk.Result)
	case <-time.After(5 * time.Second):
		t.Fatalf("task %d did not reach block_here()", tk.ID)
	}
	return done
}

func (b *blockingRuntime) finish(t *testing.T, done <-chan error) {
	t.Helper()
	close(b.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("blocked task failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked task did not finish")
	}
}

// listQueued runs an observer task that returns {id, start, verb, line} for
// each task queued_tasks() lists.
func (b *blockingRuntime) listQueued(t *testing.T, id int64, owner types.ObjID) map[int64]types.Value {
	t.Helper()
	observer := b.newTask(t, id, owner, `
rows = {};
for q in (queued_tasks())
  rows = {@rows, {q[1], q[2], q[7], q[8]}};
endfor
return rows;
`)
	if err := b.s.runTask(observer); err != nil {
		t.Fatalf("observer runTask failed: %v", err)
	}
	if observer.Result.Flow != types.FlowNormal && observer.Result.Flow != types.FlowReturn {
		t.Fatalf("observer result = %+v", observer.Result)
	}
	return queuedRows(t, observer.Result.Val)
}

func queuedRows(t *testing.T, value types.Value) map[int64]types.Value {
	t.Helper()
	rows := map[int64]types.Value{}
	if value.Type() != types.TYPE_LIST {
		t.Fatalf("queued rows = %v, want a list", value)
	}
	for i := 1; i <= value.Len(); i++ {
		row := value.Get(i)
		rows[row.Get(1).Int()] = row
	}
	return rows
}

func TestQueuedTasksListsRunningForkedTask(t *testing.T) {
	b := newBlockingRuntime(t)
	defer b.s.Stop()
	owner := types.ObjID(7901)
	defer removeTasksForOwner(b.s, owner)

	parent := b.newTask(t, 3301, owner, `
fork child (0)
  #0:blocker();
endfork
for q in (queued_tasks())
  if (q[1] == child)
    return {child, q[2], q[7], q[8]};
  endif
endfor
return {child};
`)
	if err := b.s.runTask(parent); err != nil {
		t.Fatalf("parent runTask failed: %v", err)
	}
	whileQueued := parent.Result.Val
	if whileQueued.Len() != 4 {
		t.Fatalf("parent did not see its queued fork: %v", whileQueued)
	}
	childID := whileQueued.Get(1).Int()
	child := b.s.taskManager.GetTask(childID)
	if child == nil {
		t.Fatalf("fork %d not in the catalog", childID)
	}

	done := b.runBlocked(t, child)
	row, listed := b.listQueued(t, 3302, owner)[childID]
	if !listed {
		b.finish(t, done)
		t.Fatalf("running fork %d missing from queued_tasks()", childID)
	}
	// While it runs, the fork is listed as it was queued (Toast's
	// list_for_forked_task): its start time, the fork's verb and first line,
	// not the verb it has since called.
	if got, want := row.String(), whileQueued.String(); got != want {
		b.finish(t, done)
		t.Fatalf("running fork row = %s, want the queued row %s", got, want)
	}
	b.finish(t, done)

	if _, listed := b.listQueued(t, 3303, owner)[childID]; listed {
		t.Fatalf("completed fork %d still listed", childID)
	}
}

func TestQueuedTasksListsRunningResumedTask(t *testing.T) {
	b := newBlockingRuntime(t)
	defer b.s.Stop()
	owner := types.ObjID(7902)
	defer removeTasksForOwner(b.s, owner)

	resumed := b.newTask(t, 3311, owner, `
suspend(0);
#0:blocker();
return 1;
`)
	if err := b.s.runTask(resumed); err != nil {
		t.Fatalf("first slice failed: %v", err)
	}
	if state := resumed.GetState(); state != task.TaskQueued {
		t.Fatalf("state after suspend(0) = %v, want queued", state)
	}
	whileQueued, listed := b.listQueued(t, 3312, owner)[resumed.ID]
	if !listed {
		t.Fatalf("yielded task %d missing from queued_tasks()", resumed.ID)
	}
	// The resumed slice starts more than a second after the yield, so its
	// start time differs from the yield's when rounded to seconds.
	time.Sleep(1100 * time.Millisecond)

	done := b.runBlocked(t, resumed)
	row, listed := b.listQueued(t, 3313, owner)[resumed.ID]
	if !listed {
		b.finish(t, done)
		t.Fatalf("running resumed task %d missing from queued_tasks()", resumed.ID)
	}
	// Toast lists a resumed task by its suspended activation and start_tv,
	// the time it yielded (enqueue_suspended_task), as while it was queued.
	if got, want := row.String(), whileQueued.String(); got != want {
		b.finish(t, done)
		t.Fatalf("running resumed row = %s, want the queued row %s", got, want)
	}
	b.finish(t, done)
}

func TestQueuedTasksOmitsCommandTaskFirstSlice(t *testing.T) {
	b := newBlockingRuntime(t)
	defer b.s.Stop()
	owner := types.ObjID(7903)
	defer removeTasksForOwner(b.s, owner)

	command := b.newTask(t, 3321, owner, `
#0:blocker();
return 1;
`)
	done := b.runBlocked(t, command)
	if state := command.GetState(); state != task.TaskRunning {
		b.finish(t, done)
		t.Fatalf("command task state = %v, want running", state)
	}
	_, listed := b.listQueued(t, 3322, owner)[command.ID]
	b.finish(t, done)
	if listed {
		t.Fatalf("command task %d in its first slice listed by queued_tasks()", command.ID)
	}
}

func TestQueuedTasksOmitsCaller(t *testing.T) {
	b := newBlockingRuntime(t)
	defer b.s.Stop()
	owner := types.ObjID(7904)
	defer removeTasksForOwner(b.s, owner)

	ticks, seconds := backgroundTaskLimits(newTestRegistry())
	caller := NewTaskFull(3331, owner, compileTestProgram(t, b.s.registry, `
ids = {};
for q in (queued_tasks())
  ids = {@ids, q[1]};
endfor
return {ids, queued_tasks(0, 1)};
`), ticks, seconds)
	caller.Context.IsWizard = true
	caller.IsForked = true
	caller.Kind = task.TaskForked
	b.s.taskManager.RegisterTask(caller)
	if err := b.s.runTask(caller); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if got, want := caller.Result.Val.String(), "{{}, 0}"; got != want {
		t.Fatalf("caller's queued_tasks() = %s, want %s (the caller is never listed)", got, want)
	}
}
