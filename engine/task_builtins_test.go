package engine

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func newTaskBuiltinExecution() *builtins.Execution {
	session := builtins.NewSession(builtins.NewRegistry(), builtins.NoHost())
	return session.NewExecution(task.NewContext(), nil)
}

func wireTestTaskManager(ctx *builtins.Execution) *TaskManager {
	manager := NewTaskManager()
	host := ctx.Session.Host()
	host.TaskManager = manager
	ctx.Session.ConfigureHost(host)
	return manager
}

func callTaskBuiltin(name string, ctx *builtins.Execution, args []types.Value) types.Result {
	fn, ok := ctx.Registry.Get(name)
	if !ok {
		panic("unknown test builtin: " + name)
	}
	return fn(ctx, args)
}

func TestQueuedTasksOmitsAnonymousThisFromUnrelatedViewer(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	anon, errCode := store.DirectTxn().CreateObject([]types.ObjID{0}, 1, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous object: %v", errCode)
	}

	const taskID = int64(91872)
	taskValue := NewTask(taskID, 2, 1000, 1)
	taskValue.VerbName = "delayed"
	taskValue.PushFrame(types.ActivationFrame{
		This:       anon,
		ThisValue:  types.NewAnon(anon),
		Programmer: 2,
		Verb:       "delayed",
		VerbLoc:    0,
		Player:     2,
	})
	ctx := newTaskBuiltinExecution()
	ctx.Store = store
	ctx.Programmer = 2
	ctx.Player = 2
	manager := wireTestTaskManager(ctx)
	manager.RegisterTask(taskValue)
	taskValue.SuspendSeconds(100)
	defer manager.RemoveTask(taskID)

	result := callTaskBuiltin("queued_tasks", ctx, nil)
	if result.IsError() {
		t.Fatalf("queued_tasks failed: %v", result.Error)
	}
	for _, entry := range result.Val.Elements() {
		if entry.Get(1).Int() == taskID {
			t.Fatalf("queued_tasks exposed task %d with unrelated anonymous this", taskID)
		}
	}
}

func TestQueuedTasksUsesToastVisibilityAndArgumentSemantics(t *testing.T) {
	const programmerTwoTaskID = int64(91873)
	const programmerThreeTaskID = int64(91874)

	programmerTwoTask := NewTask(programmerTwoTaskID, 3, 1000, 1)
	programmerTwoTask.VerbName = "programmer_two_delayed"
	programmerTwoTask.ForkInfo = &types.ForkInfo{
		Variables: map[string]types.Value{
			"marker": types.NewStr("programmer-two"),
		},
	}
	programmerTwoTask.PushFrame(types.ActivationFrame{
		This:       0,
		ThisValue:  types.NewObj(0),
		Programmer: 2,
		Verb:       "programmer_two_delayed",
		VerbLoc:    0,
		Player:     3,
	})
	programmerTwoTask.SetState(task.TaskQueued)

	programmerThreeTask := NewTask(programmerThreeTaskID, 2, 1000, 1)
	programmerThreeTask.VerbName = "programmer_three_delayed"
	programmerThreeTask.ForkInfo = &types.ForkInfo{
		Variables: map[string]types.Value{
			"marker": types.NewStr("programmer-three"),
		},
	}
	programmerThreeTask.PushFrame(types.ActivationFrame{
		This:       0,
		ThisValue:  types.NewObj(0),
		Programmer: 3,
		Verb:       "programmer_three_delayed",
		VerbLoc:    0,
		Player:     2,
	})
	programmerThreeTask.SetState(task.TaskQueued)

	wizard := newTaskBuiltinExecution()
	wizard.Programmer = 99
	wizard.IsWizard = true
	manager := wireTestTaskManager(wizard)
	manager.RegisterTask(programmerTwoTask)
	manager.RegisterTask(programmerThreeTask)
	defer manager.RemoveTask(programmerTwoTaskID)
	defer manager.RemoveTask(programmerThreeTaskID)

	plainResult := callTaskBuiltin("queued_tasks", wizard, nil)
	if plainResult.IsError() {
		t.Fatalf("queued_tasks() failed: %v", plainResult.Error)
	}
	plainFound := false
	for _, entry := range plainResult.Val.Elements() {
		if entry.Get(1).Int() != programmerTwoTaskID {
			continue
		}
		plainFound = true
		if entry.Len() != 10 {
			t.Fatalf("queued_tasks() entry length = %d, want 10", entry.Len())
		}
	}
	if !plainFound {
		t.Fatal("wizard queued_tasks() did not expose another programmer's task")
	}

	extendedResult := callTaskBuiltin("queued_tasks", wizard, []types.Value{types.NewInt(1)})
	if extendedResult.IsError() {
		t.Fatalf("queued_tasks(1) failed: %v", extendedResult.Error)
	}
	extendedFound := false
	for _, entry := range extendedResult.Val.Elements() {
		if entry.Get(1).Int() != programmerTwoTaskID {
			continue
		}
		extendedFound = true
		if entry.Len() != 11 {
			t.Fatalf("queued_tasks(1) entry length = %d, want 11", entry.Len())
		}
		marker, ok := entry.Get(11).MapGet(types.NewStr("marker"))
		if !ok || marker.Str() != "programmer-two" {
			t.Fatalf("queued_tasks(1) marker = %v, %v; want programmer-two, true", marker, ok)
		}
	}
	if !extendedFound {
		t.Fatal("wizard queued_tasks(1) did not expose another programmer's task")
	}

	extendedTwoArgResult := callTaskBuiltin("queued_tasks",
		wizard,
		[]types.Value{types.NewInt(1), types.NewInt(0)},
	)
	if extendedTwoArgResult.IsError() {
		t.Fatalf("queued_tasks(1, 0) failed: %v", extendedTwoArgResult.Error)
	}
	extendedTwoArgFound := false
	for _, entry := range extendedTwoArgResult.Val.Elements() {
		if entry.Get(1).Int() != programmerTwoTaskID {
			continue
		}
		extendedTwoArgFound = true
		if entry.Len() != 11 {
			t.Fatalf("queued_tasks(1, 0) entry length = %d, want 11", entry.Len())
		}
		marker, ok := entry.Get(11).MapGet(types.NewStr("marker"))
		if !ok || marker.Str() != "programmer-two" {
			t.Fatalf("queued_tasks(1, 0) marker = %v, %v; want programmer-two, true", marker, ok)
		}
	}
	if !extendedTwoArgFound {
		t.Fatal("wizard queued_tasks(1, 0) did not expose another programmer's task")
	}

	programmer := newTaskBuiltinExecution()
	programmer.Programmer = 2
	programmer.Registry = wizard.Registry
	programmer.Session = wizard.Session

	visibleResult := callTaskBuiltin("queued_tasks", programmer, nil)
	if visibleResult.IsError() {
		t.Fatalf("programmer queued_tasks() failed: %v", visibleResult.Error)
	}
	sawOwnProgrammer := false
	for _, entry := range visibleResult.Val.Elements() {
		switch entry.Get(1).Int() {
		case programmerTwoTaskID:
			sawOwnProgrammer = true
		case programmerThreeTaskID:
			t.Fatal("nonwizard queued_tasks() exposed another programmer's task")
		}
	}
	if !sawOwnProgrammer {
		t.Fatal("nonwizard queued_tasks() omitted its own programmer task")
	}

	countResult := callTaskBuiltin("queued_tasks",
		programmer,
		[]types.Value{types.NewInt(0), types.NewInt(1)},
	)
	if countResult.IsError() {
		t.Fatalf("queued_tasks(0, 1) failed: %v", countResult.Error)
	}
	if got, want := countResult.Val.Int(), int64(visibleResult.Val.Len()); got != want {
		t.Fatalf("queued_tasks(0, 1) = %d, want visible count %d", got, want)
	}
}

func TestTaskStackThirdArgumentIncludesRuntimeVariables(t *testing.T) {
	const taskID = int64(91875)
	runtimeVariables := types.NewMap([][2]types.Value{
		{types.NewStr("zeta"), types.NewInt(1)},
		{types.NewStr("alpha"), types.NewInt(2)},
	})
	taskValue := NewTask(taskID, 2, 1000, 1)
	taskValue.PushFrame(types.ActivationFrame{
		This:             7,
		ThisValue:        types.NewObj(7),
		Programmer:       2,
		Verb:             "suspended",
		VerbLoc:          8,
		Player:           3,
		LineNumber:       14,
		RuntimeVariables: runtimeVariables,
	})
	taskValue.SetState(task.TaskSuspended)

	ctx := newTaskBuiltinExecution()
	ctx.TaskID = 1
	ctx.Programmer = 2
	manager := wireTestTaskManager(ctx)
	manager.RegisterTask(taskValue)
	defer manager.RemoveTask(taskID)

	withLines := callTaskBuiltin("task_stack", ctx, []types.Value{
		types.NewInt(taskID),
		types.NewInt(1),
		types.NewInt(1),
	})
	if withLines.IsError() {
		t.Fatalf("task_stack(id, 1, 1) failed: %v", withLines.Error)
	}
	frame := withLines.Val.Get(1)
	if got, want := frame.Len(), 7; got != want {
		t.Fatalf("task_stack(id, 1, 1) frame length = %d, want %d", got, want)
	}
	if got, want := frame.Get(6).Int(), int64(14); got != want {
		t.Fatalf("task_stack(id, 1, 1) line = %d, want %d", got, want)
	}
	keys := frame.Get(7).Keys()
	if len(keys) != 2 || keys[0].Str() != "alpha" || keys[1].Str() != "zeta" {
		t.Fatalf("task_stack(id, 1, 1) variable keys = %v, want [alpha zeta]", keys)
	}
	zeta, ok := frame.Get(7).MapGet(types.NewStr("zeta"))
	if !ok || zeta.Int() != 1 {
		t.Fatalf("task_stack(id, 1, 1) zeta = %v, %v; want 1, true", zeta, ok)
	}
	alpha, ok := frame.Get(7).MapGet(types.NewStr("alpha"))
	if !ok || alpha.Int() != 2 {
		t.Fatalf("task_stack(id, 1, 1) alpha = %v, %v; want 2, true", alpha, ok)
	}

	withoutLines := callTaskBuiltin("task_stack", ctx, []types.Value{
		types.NewInt(taskID),
		types.NewInt(0),
		types.NewInt(1),
	})
	if withoutLines.IsError() {
		t.Fatalf("task_stack(id, 0, 1) failed: %v", withoutLines.Error)
	}
	frame = withoutLines.Val.Get(1)
	if got, want := frame.Len(), 6; got != want {
		t.Fatalf("task_stack(id, 0, 1) frame length = %d, want %d", got, want)
	}
	if got := frame.Get(6); !got.Equal(runtimeVariables) {
		t.Fatalf("task_stack(id, 0, 1) variables = %v, want %v", got, runtimeVariables)
	}

	withoutVariables := callTaskBuiltin("task_stack", ctx, []types.Value{
		types.NewInt(taskID),
		types.NewInt(1),
		types.NewInt(0),
	})
	if withoutVariables.IsError() {
		t.Fatalf("task_stack(id, 1, 0) failed: %v", withoutVariables.Error)
	}
	frame = withoutVariables.Val.Get(1)
	if got, want := frame.Len(), 6; got != want {
		t.Fatalf("task_stack(id, 1, 0) frame length = %d, want %d", got, want)
	}
	if got, want := frame.Get(6).Int(), int64(14); got != want {
		t.Fatalf("task_stack(id, 1, 0) line = %d, want %d", got, want)
	}
}

func TestTaskStackRejectsRetainedTerminalTasks(t *testing.T) {
	for _, state := range []task.TaskState{task.TaskCompleted, task.TaskKilled} {
		t.Run(state.String(), func(t *testing.T) {
			ctx := newTaskBuiltinExecution()
			ctx.TaskID = 1
			ctx.Programmer = 2
			manager := wireTestTaskManager(ctx)
			finished := NewTask(42, 2, 1000, 1)
			finished.SetState(state)
			manager.RegisterTask(finished)
			result := callTaskBuiltin("task_stack", ctx, []types.Value{types.NewInt(42)})
			if !result.IsError() || result.Error != types.E_INVARG {
				t.Fatalf("task_stack retained %s = %#v, want E_INVARG", state, result)
			}
		})
	}
}

func TestReview_IO_QueuedTasksSortOrder(t *testing.T) {
	ctx := newTaskBuiltinExecution()
	ctx.IsWizard = true
	mgr := wireTestTaskManager(ctx)

	// Use large IDs unlikely to collide with other test tasks.
	const earlierID = int64(88801)
	const laterID = int64(88802)

	earlier := NewTask(earlierID, 1, 10000, 30)
	later := NewTask(laterID, 1, 10000, 30)

	// Set a non-empty verb name so GetQueuedTasks does not filter them as
	// eval scaffolding (which filters IsForked && VerbName == "").
	earlier.VerbName = "review_earlier_verb"
	later.VerbName = "review_later_verb"

	mgr.RegisterTask(earlier)
	mgr.RegisterTask(later)
	earlier.SuspendSeconds(-1)
	later.SuspendSeconds(-1)
	// Give them clearly distinct start times. SuspendTask(-1) stamps the
	// indefinite-suspend sentinel onto StartTime, so override it AFTERWARDS to
	// exercise the ascending comparator with two distinct, finite start times.
	earlier.StartTime = time.Now().Add(-10 * time.Second)
	later.StartTime = time.Now()
	defer mgr.RemoveTask(earlierID)
	defer mgr.RemoveTask(laterID)

	res := callTaskBuiltin("queued_tasks", ctx, []types.Value{})
	if res.IsError() {
		t.Fatalf("queued_tasks() error: %v", res.Error)
	}

	list := res.Val
	if list.Type() != types.TYPE_LIST {
		t.Fatalf("expected list result, got %v", res.Val)
	}

	// Locate our two tasks in the result.
	earlierIdx, laterIdx := 0, 0
	for i := 1; i <= list.Len(); i++ {
		entry := list.Get(i)
		if entry.Type() != types.TYPE_LIST {
			continue
		}
		idVal := entry.Get(1)
		if idVal.Type() != types.TYPE_INT {
			continue
		}
		switch idVal.Int() {
		case earlierID:
			earlierIdx = i
		case laterID:
			laterIdx = i
		}
	}

	if earlierIdx == 0 || laterIdx == 0 {
		t.Fatalf("could not find both test tasks in queued_tasks() output (earlierIdx=%d laterIdx=%d)",
			earlierIdx, laterIdx)
	}

	// Ascending order: the older task must appear before the newer one.
	if earlierIdx > laterIdx {
		t.Fatalf(
			"CONFIRMED BUG (H4): queued_tasks() returned newer task at index %d before older task at index %d.\n"+
				"Cause: builtinQueuedTasks sorts with StartTime.After() (descending);\n"+
				"Toast returns tasks in ascending StartTime order (oldest first).\n"+
				"Fix: change After to Before in the sort.SliceStable call.",
			laterIdx, earlierIdx,
		)
	}
}

func TestReview_IO_QueuedTasksIndefiniteSuspendSortsLast(t *testing.T) {
	ctx := newTaskBuiltinExecution()
	ctx.IsWizard = true
	mgr := wireTestTaskManager(ctx)

	const timedID = int64(88811)
	const indefID = int64(88812)

	timed := NewTask(timedID, 1, 10000, 30)
	indef := NewTask(indefID, 1, 10000, 30)

	timed.VerbName = "review_timed_verb"
	indef.VerbName = "review_indef_verb"

	mgr.RegisterTask(timed)
	mgr.RegisterTask(indef)

	// timed: a future-scheduled task (mirrors fork(100)). Finite start time.
	timed.StartTime = time.Now().Add(100 * time.Second)
	// indef: indefinite suspend() — SuspendTask(-1) stamps the far-future
	// IndefiniteSuspendStartTime sentinel onto StartTime.
	indef.SuspendSeconds(-1)
	timed.SetState(task.TaskSuspended)

	defer mgr.RemoveTask(timedID)
	defer mgr.RemoveTask(indefID)

	// Sanity: the indefinite suspend got the far-future sentinel and no wake
	// deadline (so the scheduler never auto-wakes it).
	if !indef.StartTime.Equal(task.IndefiniteSuspendStartTime) {
		t.Fatalf("indefinite-suspended task StartTime = %v, want sentinel %v",
			indef.StartTime, task.IndefiniteSuspendStartTime)
	}
	if indef.WakeDue(time.Now()) {
		t.Fatalf("indefinite-suspended task reported WakeDue=true; it must never auto-wake")
	}

	res := callTaskBuiltin("queued_tasks", ctx, []types.Value{})
	if res.IsError() {
		t.Fatalf("queued_tasks() error: %v", res.Error)
	}
	list := res.Val
	if list.Type() != types.TYPE_LIST {
		t.Fatalf("expected list result, got %v", res.Val)
	}

	timedIdx, indefIdx := 0, 0
	for i := 1; i <= list.Len(); i++ {
		entry := list.Get(i)
		if entry.Type() != types.TYPE_LIST {
			continue
		}
		idVal := entry.Get(1)
		if idVal.Type() != types.TYPE_INT {
			continue
		}
		switch idVal.Int() {
		case timedID:
			timedIdx = i
		case indefID:
			indefIdx = i
		}
	}

	if timedIdx == 0 || indefIdx == 0 {
		t.Fatalf("could not find both tasks in queued_tasks() (timedIdx=%d indefIdx=%d)", timedIdx, indefIdx)
	}
	if indefIdx < timedIdx {
		t.Fatalf("indefinite-suspended task sorted at %d BEFORE timed task at %d; "+
			"it must sort LAST (mirror Toast INTNUM_MAX start_tv)", indefIdx, timedIdx)
	}
}
