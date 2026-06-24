package builtins

import (
	"errors"
	"testing"

	"barn/kernel"
	"barn/types"
)

func TestBuiltinShutdownPassesMessageAndPanicFlag(t *testing.T) {
	previous := globalShutdownFunc
	t.Cleanup(func() { globalShutdownFunc = previous })

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	ctx.Programmer = 7

	called := false
	var gotCtx *kernel.TaskContext
	var gotMessage string
	var gotUnclean bool
	SetShutdownFunc(func(ctx *kernel.TaskContext, message string, unclean bool) error {
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
	previous := globalShutdownFunc
	t.Cleanup(func() { globalShutdownFunc = previous })
	SetShutdownFunc(func(ctx *kernel.TaskContext, message string, unclean bool) error {
		t.Fatalf("shutdown callback should not run")
		return nil
	})

	ctx := kernel.NewTaskContext()
	result := builtinShutdown(ctx, []types.Value{types.NewInt(1)})
	if !result.IsError() || result.Error != types.E_TYPE {
		t.Fatalf("shutdown result = %+v, want E_TYPE", result)
	}
}

func TestBuiltinDumpDatabaseRequestsCheckpoint(t *testing.T) {
	previous := globalDumpFunc
	t.Cleanup(func() { globalDumpFunc = previous })

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	ctx.Programmer = 7

	called := false
	SetDumpFunc(func() error {
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
