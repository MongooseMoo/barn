package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestTaskStackRejectsRetainedTerminalTasks(t *testing.T) {
	for _, state := range []task.TaskState{task.TaskCompleted, task.TaskKilled} {
		t.Run(state.String(), func(t *testing.T) {
			ctx := newTestExecution()
			ctx.TaskID = 1
			ctx.Programmer = 2
			manager := wireTestTaskManager(ctx)
			finished := task.NewTask(42, 2, 1000, 1)
			finished.SetState(state)
			manager.RegisterTask(finished)
			result := builtinTaskStack(ctx, []types.Value{types.NewInt(42)})
			if !result.IsError() || result.Error != types.E_INVARG {
				t.Fatalf("task_stack retained %s = %#v, want E_INVARG", state, result)
			}
		})
	}
}

func TestQueueInfoReportsLastInputTaskID(t *testing.T) {
	ctx := ctxWithConnManager(&stubConnManager{conn: &stubConn{lastInputTaskID: 913}})
	ctx.IsWizard = true
	ctx.Player = 2
	wireTestTaskManager(ctx)
	result := builtinQueueInfo(ctx, []types.Value{types.NewObj(2)})
	if !result.IsNormal() {
		t.Fatal(result.Error)
	}
	got, ok := result.Val.MapGet(types.NewStr("last_input_task_id"))
	if !ok || got.Type() != types.TYPE_INT || got.Int() != 913 {
		t.Fatalf("queue_info last_input_task_id = %v, %t; want 913", got, ok)
	}
}
