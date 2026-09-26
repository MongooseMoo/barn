package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// threadedCtx is a wizard execution on a task's root VM with thread mode on.
func threadedCtx() (*Execution, *task.Task) {
	ctx, taskValue := sqliteAsyncCtx()
	ctx.BeforeIrreversibleEffect = func() bool {
		panic("a threaded call crossed the irreversible-effect boundary")
	}
	return ctx, taskValue
}

// A threaded call suspends, starts its work only when the slice commits, and
// resumes the task with the work's value.
func TestThreadedSortSuspendsAndResumesWithValue(t *testing.T) {
	ctx, taskValue := threadedCtx()
	list := types.NewList([]types.Value{types.NewInt(3), types.NewInt(1), types.NewInt(2)})

	result := builtinSort(ctx, []types.Value{list})
	if result.Flow != types.FlowSuspend {
		t.Fatalf("sort flow = %v, want suspend", result.Flow)
	}
	if len(ctx.PendingEffects) != 1 {
		t.Fatalf("pending effects = %d, want the deferred start", len(ctx.PendingEffects))
	}
	if got := taskValue.GetState(); got != task.TaskSuspended {
		t.Fatalf("task state = %v, want suspended", got)
	}
	FlushPendingEffects(ctx)
	waitForSQLiteResume(t, taskValue)
	if got := taskValue.WakeValue.String(); got != "{1, 2, 3}" {
		t.Fatalf("wake value = %s, want {1, 2, 3}", got)
	}
}

// A threaded sort's element error is the wake value, which the VM raises on
// resume; an inline sort returns the same error as its value.
func TestSortCallbackErrorByThreadMode(t *testing.T) {
	mixed := types.NewList([]types.Value{types.NewInt(1), types.NewStr("a")})

	ctx, taskValue := threadedCtx()
	if result := builtinSort(ctx, []types.Value{mixed}); result.Flow != types.FlowSuspend {
		t.Fatalf("threaded sort flow = %v, want suspend", result.Flow)
	}
	FlushPendingEffects(ctx)
	waitForSQLiteResume(t, taskValue)
	if taskValue.WakeValue.Type() != types.TYPE_ERR || taskValue.WakeValue.ErrCode() != types.E_TYPE {
		t.Fatalf("threaded wake value = %v, want E_TYPE", taskValue.WakeValue)
	}

	inline, _ := threadedCtx()
	inline.ThreadMode = false
	result := builtinSort(inline, []types.Value{mixed})
	if result.Flow != types.FlowNormal || result.Val.Type() != types.TYPE_ERR || result.Val.ErrCode() != types.E_TYPE {
		t.Fatalf("unthreaded sort = %+v, want the value E_TYPE", result)
	}
}

// A nested VM cannot suspend, so the work runs inline, and it gets what a
// resumed task would: the value, or its error raised.
func TestThreadedBuiltinsRunInlineWhereSuspendIsNotHonored(t *testing.T) {
	ctx, taskValue := threadedCtx()
	ctx.CanSuspend = false

	list := types.NewList([]types.Value{types.NewInt(3), types.NewInt(1), types.NewInt(3)})
	sorted := builtinSort(ctx, []types.Value{list})
	if sorted.Flow != types.FlowNormal || sorted.Val.String() != "{1, 3, 3}" {
		t.Fatalf("nested sort = %+v, want {1, 3, 3}", sorted)
	}
	members := builtinAllMembers(ctx, []types.Value{types.NewInt(3), list})
	if members.Flow != types.FlowNormal || members.Val.String() != "{1, 3}" {
		t.Fatalf("nested all_members = %+v, want {1, 3}", members)
	}
	mixed := types.NewList([]types.Value{types.NewInt(1), types.NewStr("a")})
	if raised := builtinSort(ctx, []types.Value{mixed}); raised.Flow != types.FlowException || raised.Error != types.E_TYPE {
		t.Fatalf("nested sort error = %+v, want E_TYPE raised", raised)
	}
	if got := taskValue.GetState(); got != task.TaskRunning {
		t.Fatalf("task state = %v, want still running", got)
	}
	if len(ctx.PendingEffects) != 0 {
		t.Fatalf("pending effects = %d, want none", len(ctx.PendingEffects))
	}
}
