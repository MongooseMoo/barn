package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestCallersRedactsAnonymousThisFromUnrelatedViewer(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	anon, errCode := store.CreateObject([]types.ObjID{0}, 1, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous object: %v", errCode)
	}

	taskValue := task.NewTask(1, 2, 1000, 1)
	taskValue.PushFrame(task.ActivationFrame{
		This:       anon,
		ThisValue:  types.NewAnon(anon),
		Programmer: 1,
		Verb:       "entry",
		VerbLoc:    0,
		Player:     2,
	})
	taskValue.PushFrame(task.ActivationFrame{
		This:       0,
		ThisValue:  types.NewObj(0),
		Programmer: 2,
		Verb:       "probe",
		VerbLoc:    0,
		Player:     2,
	})

	ctx := kernel.NewTaskContext()
	ctx.Store = store
	ctx.Task = taskValue
	ctx.Programmer = 2
	ctx.Player = 2
	ctx.Verb = "probe"

	result := builtinCallers(ctx, nil)
	if result.IsError() {
		t.Fatalf("callers failed: %v", result.Error)
	}
	thisValue := result.Val.Get(1).Get(1)
	if thisValue.Type() != types.TYPE_ANON {
		t.Fatalf("redacted this type = %v, want ANON", thisValue.Type())
	}
	if store.Valid(thisValue.ID()) {
		t.Fatalf("redacted this = %s is still valid", thisValue.String())
	}
}

func TestQueuedTasksOmitsAnonymousThisFromUnrelatedViewer(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	anon, errCode := store.CreateObject([]types.ObjID{0}, 1, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous object: %v", errCode)
	}

	const taskID = int64(91872)
	taskValue := task.NewTask(taskID, 2, 1000, 1)
	taskValue.VerbName = "delayed"
	taskValue.PushFrame(task.ActivationFrame{
		This:       anon,
		ThisValue:  types.NewAnon(anon),
		Programmer: 2,
		Verb:       "delayed",
		VerbLoc:    0,
		Player:     2,
	})
	ctx := kernel.NewTaskContext()
	ctx.Store = store
	ctx.Programmer = 2
	ctx.Player = 2
	manager := wireTestTaskManager(ctx)
	manager.RegisterTask(taskValue)
	manager.SuspendTask(taskValue, 100)
	defer manager.RemoveTask(taskID)

	result := builtinQueuedTasks(ctx, nil)
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

	programmerTwoTask := task.NewTask(programmerTwoTaskID, 3, 1000, 1)
	programmerTwoTask.VerbName = "programmer_two_delayed"
	programmerTwoTask.ForkInfo = &types.ForkInfo{
		Variables: map[string]types.Value{
			"marker": types.NewStr("programmer-two"),
		},
	}
	programmerTwoTask.PushFrame(task.ActivationFrame{
		This:       0,
		ThisValue:  types.NewObj(0),
		Programmer: 2,
		Verb:       "programmer_two_delayed",
		VerbLoc:    0,
		Player:     3,
	})
	programmerTwoTask.SetState(task.TaskQueued)

	programmerThreeTask := task.NewTask(programmerThreeTaskID, 2, 1000, 1)
	programmerThreeTask.VerbName = "programmer_three_delayed"
	programmerThreeTask.ForkInfo = &types.ForkInfo{
		Variables: map[string]types.Value{
			"marker": types.NewStr("programmer-three"),
		},
	}
	programmerThreeTask.PushFrame(task.ActivationFrame{
		This:       0,
		ThisValue:  types.NewObj(0),
		Programmer: 3,
		Verb:       "programmer_three_delayed",
		VerbLoc:    0,
		Player:     2,
	})
	programmerThreeTask.SetState(task.TaskQueued)

	wizard := kernel.NewTaskContext()
	wizard.Programmer = 99
	wizard.IsWizard = true
	manager := wireTestTaskManager(wizard)
	manager.RegisterTask(programmerTwoTask)
	manager.RegisterTask(programmerThreeTask)
	defer manager.RemoveTask(programmerTwoTaskID)
	defer manager.RemoveTask(programmerThreeTaskID)

	plainResult := builtinQueuedTasks(wizard, nil)
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

	extendedResult := builtinQueuedTasks(wizard, []types.Value{types.NewInt(1)})
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

	programmer := kernel.NewTaskContext()
	programmer.Programmer = 2
	programmer.Registry = wizard.Registry

	visibleResult := builtinQueuedTasks(programmer, nil)
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

	countResult := builtinQueuedTasks(
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
	taskValue := task.NewTask(taskID, 2, 1000, 1)
	taskValue.PushFrame(task.ActivationFrame{
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

	ctx := kernel.NewTaskContext()
	ctx.TaskID = 1
	ctx.Programmer = 2
	manager := wireTestTaskManager(ctx)
	manager.RegisterTask(taskValue)
	defer manager.RemoveTask(taskID)

	withLines := builtinTaskStack(ctx, []types.Value{
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

	withoutLines := builtinTaskStack(ctx, []types.Value{
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

	withoutVariables := builtinTaskStack(ctx, []types.Value{
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
