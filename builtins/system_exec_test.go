package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func TestExecEnvironmentHelper(t *testing.T) {
	if os.Getenv("BARN_EXEC_ENV_HELPER") == "1" {
		fmt.Printf("%s|%s|%s", os.Getenv("ISSUE53_CHILD"), os.Getenv("ISSUE53_PARENT"), os.Getenv("PATH"))
		os.Exit(0)
	}
}

func TestExecCommandWithContextUsesToastEnvironmentSemantics(t *testing.T) {
	t.Setenv("ISSUE53_PARENT", "parent")
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}

	result := execCommandWithContext(
		context.Background(),
		executable,
		[]string{"-test.run=^TestExecEnvironmentHelper$"},
		"",
		[]string{"PATH=/bin:/usr/bin", "BARN_EXEC_ENV_HELPER=1", "ISSUE53_CHILD=child"},
	)
	if !result.IsNormal() {
		t.Fatalf("exec result = flow %v error %v, want normal", result.Flow, result.Error)
	}
	if got, want := result.Val.Get(2).Str(), "child||/bin:/usr/bin"; got != want {
		t.Fatalf("child environment = %q, want %q", got, want)
	}
	if got := result.Val.Get(3).Str(); got != "" {
		t.Fatalf("child stderr = %q, want empty", got)
	}
}

func TestBuiltinExecPassesToastEnvironmentToChild(t *testing.T) {
	t.Setenv("ISSUE53_PARENT", "parent")
	program := installExecTestBinary(t)

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	result := builtinExec(ctx, []types.Value{
		types.NewList([]types.Value{
			types.NewStr(program),
			types.NewStr("-test.run=^TestExecEnvironmentHelper$"),
		}),
		types.NewStr(""),
		types.NewList([]types.Value{
			types.NewStr("BARN_EXEC_ENV_HELPER=1"),
			types.NewStr("ISSUE53_CHILD=child"),
		}),
	})
	if !result.IsNormal() {
		t.Fatalf("exec result = flow %v error %v, want normal", result.Flow, result.Error)
	}
	if got, want := result.Val.Get(2).Str(), "child||/bin:/usr/bin"; got != want {
		t.Fatalf("child environment = %q, want %q", got, want)
	}
}

func TestBuiltinExecRejectsNonStringEnvironmentEntries(t *testing.T) {
	program := installExecTestBinary(t)

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	result := builtinExec(ctx, []types.Value{
		types.NewList([]types.Value{types.NewStr(program)}),
		types.NewStr(""),
		types.NewList([]types.Value{types.NewInt(1)}),
	})
	if result.Error != types.E_INVARG {
		t.Fatalf("exec error = %v, want %v", result.Error, types.E_INVARG)
	}
}

func installExecTestBinary(t *testing.T) string {
	t.Helper()
	serverDir := t.TempDir()
	execDir := filepath.Join(serverDir, "executables")
	if err := os.Mkdir(execDir, 0o755); err != nil {
		t.Fatalf("create executables directory: %v", err)
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	contents, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("read test executable: %v", err)
	}
	program := "issue53-env-helper"
	fixtureName := program
	if runtime.GOOS == "windows" {
		fixtureName += ".exe"
	}
	if err := os.WriteFile(filepath.Join(execDir, fixtureName), contents, 0o755); err != nil {
		t.Fatalf("write executable fixture: %v", err)
	}
	t.Chdir(serverDir)
	return program
}

func TestValidateAndResolvePathUsesServerExecutablesDirectory(t *testing.T) {
	serverDir := t.TempDir()
	execDir := filepath.Join(serverDir, "executables")
	if err := os.Mkdir(execDir, 0o755); err != nil {
		t.Fatalf("create executables directory: %v", err)
	}
	fixture := filepath.Join(execDir, "sleep")
	if err := os.WriteFile(fixture, nil, 0o755); err != nil {
		t.Fatalf("create executable fixture: %v", err)
	}
	t.Chdir(serverDir)

	got, err := validateAndResolvePath("sleep")
	if err != nil {
		t.Fatalf("resolve executables/sleep: %v", err)
	}
	want := filepath.Join("executables", "sleep")
	if got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
}
