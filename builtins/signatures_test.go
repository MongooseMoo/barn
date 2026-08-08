package builtins

import (
	"errors"
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func TestBuiltinShutdownPassesMessageAndPanicFlag(t *testing.T) {
	r := NewRegistry()
	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	ctx.Programmer = 7
	ctx.Registry = r

	called := false
	var gotCtx *kernel.TaskContext
	var gotMessage string
	var gotUnclean bool
	r.SetShutdownFunc(func(ctx *kernel.TaskContext, message string, unclean bool) error {
		called = true
		gotCtx = ctx
		gotMessage = message
		gotUnclean = unclean
		return nil
	})

	result := builtinShutdown(ctx, []types.Value{types.NewStr("Maintenance"), types.NewInt(1)})
	if !result.IsNormal() {
		t.Fatalf("shutdown result = %+v, want normal", result)
	}
	if !called {
		t.Fatalf("shutdown callback was not called")
	}
	if gotCtx != ctx {
		t.Fatalf("callback ctx = %p, want %p", gotCtx, ctx)
	}
	if gotMessage != "Maintenance" {
		t.Fatalf("callback message = %q, want Maintenance", gotMessage)
	}
	if !gotUnclean {
		t.Fatalf("callback unclean = false, want true")
	}
}

func TestBuiltinShutdownValidatesMessageBeforePermissions(t *testing.T) {
	r := NewRegistry()
	r.SetShutdownFunc(func(ctx *kernel.TaskContext, message string, unclean bool) error {
		t.Fatalf("shutdown callback should not run")
		return nil
	})

	ctx := kernel.NewTaskContext()
	ctx.Registry = r
	result := builtinShutdown(ctx, []types.Value{types.NewInt(1)})
	if !result.IsError() || result.Error != types.E_TYPE {
		t.Fatalf("shutdown result = %+v, want E_TYPE", result)
	}
}

func TestBuiltinDumpDatabaseRequestsCheckpoint(t *testing.T) {
	r := NewRegistry()
	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	ctx.Programmer = 7
	ctx.Registry = r

	called := false
	r.SetDumpFunc(func() error {
		called = true
		return errors.New("request failed")
	})

	result := builtinDumpDatabase(ctx, nil)
	if !result.IsNormal() {
		t.Fatalf("dump_database result = %+v, want normal", result)
	}
	if !called {
		t.Fatalf("dump_database did not call checkpoint request function")
	}
}

func TestBuiltinFinishedTasksPermissionAndValidation(t *testing.T) {
	tests := []struct {
		name      string
		isWizard  bool
		args      []types.Value
		wantError types.ErrorCode
		wantList  bool
	}{
		{
			name:      "non_wizard_denied",
			wantError: types.E_PERM,
		},
		{
			name:      "arity_precedes_permission",
			args:      []types.Value{types.NewInt(1)},
			wantError: types.E_ARGS,
		},
		{
			name:     "wizard_receives_list",
			isWizard: true,
			wantList: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := kernel.NewTaskContext()
			ctx.IsWizard = tc.isWizard
			wireTestTaskManager(ctx)

			result := builtinFinishedTasks(ctx, tc.args)
			if tc.wantError != types.E_NONE {
				if !result.IsError() || result.Error != tc.wantError {
					t.Fatalf("finished_tasks result = %+v, want %s", result, tc.wantError)
				}
				return
			}
			if !result.IsNormal() {
				t.Fatalf("finished_tasks result = %+v, want normal", result)
			}
			if tc.wantList && result.Val.Type() != types.TYPE_LIST {
				t.Fatalf("finished_tasks result type = %s, want LIST", result.Val.Type())
			}
		})
	}
}
