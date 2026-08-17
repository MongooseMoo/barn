package builtins

import (
	"errors"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestBuiltinShutdownPassesMessageAndPanicFlag(t *testing.T) {
	r := NewSession(NewRegistry(), NoHost())
	ctx := newTestExecutionForSession(r)
	ctx.IsWizard = true
	ctx.Programmer = 7

	called := false
	var gotCtx *Execution
	var gotMessage string
	var gotUnclean bool
	configureTestHost(r, func(host *Host) {
		host.Shutdown = func(ctx *Execution, message string, unclean bool) error {
			called = true
			gotCtx = ctx
			gotMessage = message
			gotUnclean = unclean
			return nil
		}
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
	r := NewSession(NewRegistry(), NoHost())
	configureTestHost(r, func(host *Host) {
		host.Shutdown = func(ctx *Execution, message string, unclean bool) error {
			t.Fatalf("shutdown callback should not run")
			return nil
		}
	})

	ctx := newTestExecutionForSession(r)
	result := builtinShutdown(ctx, []types.Value{types.NewInt(1)})
	if !result.IsError() || result.Error != types.E_TYPE {
		t.Fatalf("shutdown result = %+v, want E_TYPE", result)
	}
}

func TestBuiltinDumpDatabaseRequestsCheckpoint(t *testing.T) {
	r := NewSession(NewRegistry(), NoHost())
	ctx := newTestExecutionForSession(r)
	ctx.IsWizard = true
	ctx.Programmer = 7

	called := false
	configureTestHost(r, func(host *Host) {
		host.Checkpoint = func() error {
			called = true
			return errors.New("request failed")
		}
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
			ctx := newTestExecution()
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
