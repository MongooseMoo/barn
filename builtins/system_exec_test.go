package builtins

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestExecEnvironmentHelper(t *testing.T) {
	if os.Getenv("BARN_EXEC_ENV_HELPER") == "1" {
		fmt.Printf("%s|%s|%s", os.Getenv("ISSUE53_CHILD"), os.Getenv("ISSUE53_PARENT"), os.Getenv("PATH"))
		os.Exit(0)
	}
}

func TestExecInputHelper(t *testing.T) {
	if os.Getenv("BARN_EXEC_INPUT_HELPER") == "1" {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Print(string(input))
		os.Exit(0)
	}
}

func TestExecSleepHelper(t *testing.T) {
	if os.Getenv("BARN_EXEC_SLEEP_HELPER") == "1" {
		time.Sleep(31 * time.Second)
		fmt.Print("done")
		os.Exit(0)
	}
}

// Toast's exec() has no time limit: the task resumes only when the child
// exits or the task is killed (src/exec.cc exec_complete,
// deal_with_child_exit). A child that outlives 30 seconds must still deliver
// its exit code and output.
func TestBuiltinExecWaitsForChildPastThirtySeconds(t *testing.T) {
	program := installExecTestBinary(t)
	ctx, taskValue := sqliteAsyncCtx()
	ctx.BeforeIrreversibleEffect = func(string) bool { return false }
	result := admitTestBuiltin("exec", ctx, []types.Value{
		types.NewList([]types.Value{types.NewStr(program), types.NewStr("-test.run=^TestExecSleepHelper$")}),
		types.NewStr(""),
		types.NewList([]types.Value{types.NewStr("BARN_EXEC_SLEEP_HELPER=1")}),
	})
	if result.Flow != types.FlowSuspend {
		t.Fatalf("exec flow = %v, want suspend", result.Flow)
	}
	deadline := time.Now().Add(60 * time.Second)
	for taskValue.GetState() == task.TaskSuspended && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if got := taskValue.GetState(); got != task.TaskQueued {
		t.Fatalf("task state = %v, want queued after the process finished", got)
	}
	want := types.NewList([]types.Value{types.NewInt(0), types.NewStr("done"), types.NewStr("")})
	if !taskValue.WakeValue.Equal(want) {
		t.Fatalf("exec result = %v, want %v", taskValue.WakeValue, want)
	}
}

func TestBuiltinExecPassesInputWithEnvironmentArgument(t *testing.T) {
	program := installExecTestBinary(t)

	ctx := newTestExecution()
	ctx.IsWizard = true
	result := admitTestBuiltin("exec", ctx, []types.Value{
		types.NewList([]types.Value{
			types.NewStr(program),
			types.NewStr("-test.run=^TestExecInputHelper$"),
		}),
		types.NewStr("hello"),
		types.NewList([]types.Value{types.NewStr("BARN_EXEC_INPUT_HELPER=1")}),
	})
	if !result.IsNormal() {
		t.Fatalf("exec result = flow %v error %v, want normal", result.Flow, result.Error)
	}
	if got, want := result.Val.Get(2).Str(), "hello"; got != want {
		t.Fatalf("child stdin = %q, want %q", got, want)
	}
}

func TestBuiltinExecRejectsInvalidInputWithEnvironmentArgument(t *testing.T) {
	program := installExecTestBinary(t)

	ctx := newTestExecution()
	ctx.IsWizard = true
	result := admitTestBuiltin("exec", ctx, []types.Value{
		types.NewList([]types.Value{types.NewStr(program)}),
		types.NewStr("invalid~input"),
		types.NewList(nil),
	})
	if result.Error != types.E_INVARG {
		t.Fatalf("exec error = %v, want %v", result.Error, types.E_INVARG)
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

	ctx := newTestExecution()
	ctx.IsWizard = true
	result := admitTestBuiltin("exec", ctx, []types.Value{
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

	ctx := newTestExecution()
	ctx.IsWizard = true
	result := admitTestBuiltin("exec", ctx, []types.Value{
		types.NewList([]types.Value{types.NewStr(program)}),
		types.NewStr(""),
		types.NewList([]types.Value{types.NewInt(1)}),
	})
	if result.Error != types.E_INVARG {
		t.Fatalf("exec error = %v, want %v", result.Error, types.E_INVARG)
	}
}

// An exec() that fails its own checks starts no process, so it must not cross
// the irreversible-effect boundary: no exclusive commit gate, no
// commit-and-renew, and the attempt stays re-runnable.
func TestBuiltinExecFailedChecksDoNotCrossIrreversibleBoundary(t *testing.T) {
	program := installExecTestBinary(t)
	str := types.NewStr
	cmd := func(parts ...types.Value) types.Value { return types.NewList(parts) }
	cases := []struct {
		name   string
		wizard bool
		args   []types.Value
		want   types.ErrorCode
	}{
		{"not wizard", false, []types.Value{cmd(str(program))}, types.E_PERM},
		{"empty command", true, []types.Value{cmd()}, types.E_INVARG},
		{"non-string program", true, []types.Value{cmd(types.NewInt(1))}, types.E_TYPE},
		{"non-string argument", true, []types.Value{cmd(str(program), types.NewInt(1))}, types.E_TYPE},
		{"missing program", true, []types.Value{cmd(str("test_does_not_exist"))}, types.E_INVARG},
		{"absolute path", true, []types.Value{cmd(str("/" + program))}, types.E_INVARG},
		{"parent escape", true, []types.Value{cmd(str("../" + program))}, types.E_INVARG},
		{"inner escape", true, []types.Value{cmd(str("foo/../../" + program))}, types.E_INVARG},
		{"invalid input", true, []types.Value{cmd(str(program)), str("invalid~input")}, types.E_INVARG},
		{"non-string environment", true, []types.Value{cmd(str(program)), str(""), cmd(types.NewInt(1))}, types.E_INVARG},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, taskValue := sqliteAsyncCtx()
			ctx.IsWizard = tc.wizard
			crossings := 0
			ctx.BeforeIrreversibleEffect = func(string) bool {
				crossings++
				return false
			}
			result := admitTestBuiltin("exec", ctx, tc.args)
			if result.Flow != types.FlowException || result.Error != tc.want {
				t.Fatalf("exec = %+v, want %v", result, tc.want)
			}
			if crossings != 0 || ctx.IrreversibleSideEffect {
				t.Fatalf("crossings = %d irreversible = %v, want 0 and false", crossings, ctx.IrreversibleSideEffect)
			}
			if taskValue.IsExecSuspended || taskValue.GetState() != task.TaskRunning {
				t.Fatalf("exec-suspended = %v state = %v, want a running task", taskValue.IsExecSuspended, taskValue.GetState())
			}
		})
	}
}

// A valid exec() crosses the boundary once, naming itself, before the process
// starts, and then suspends the task until the process finishes.
func TestBuiltinExecCrossesBoundaryBeforeStartingProcess(t *testing.T) {
	program := installExecTestBinary(t)
	ctx, taskValue := sqliteAsyncCtx()
	crossings := 0
	ctx.BeforeIrreversibleEffect = func(builtin string) bool {
		crossings++
		if builtin != "exec" {
			t.Errorf("boundary crossed as %q, want exec", builtin)
		}
		if taskValue.IsExecSuspended || taskValue.GetState() != task.TaskRunning {
			t.Errorf("boundary crossed after the process started")
		}
		return false
	}
	result := admitTestBuiltin("exec", ctx, []types.Value{
		types.NewList([]types.Value{types.NewStr(program), types.NewStr("-test.run=^TestExecInputHelper$")}),
		types.NewStr("hello"),
		types.NewList([]types.Value{types.NewStr("BARN_EXEC_INPUT_HELPER=1")}),
	})
	if result.Flow != types.FlowSuspend {
		t.Fatalf("exec flow = %v, want suspend", result.Flow)
	}
	if crossings != 1 || !ctx.IrreversibleSideEffect {
		t.Fatalf("crossings = %d irreversible = %v, want 1 and true", crossings, ctx.IrreversibleSideEffect)
	}
	deadline := time.Now().Add(30 * time.Second)
	for taskValue.GetState() == task.TaskSuspended && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := taskValue.GetState(); got != task.TaskQueued {
		t.Fatalf("task state = %v, want queued after the process finished", got)
	}
	if got := taskValue.WakeValue.Get(2).Str(); got != "hello" {
		t.Fatalf("child stdin = %q, want %q", got, "hello")
	}
}

// When the runtime abandons a stale attempt at the boundary, exec() returns
// without starting the process, leaving the task running and re-runnable.
func TestBuiltinExecStaleAttemptAbortsBeforeStartingProcess(t *testing.T) {
	program := installExecTestBinary(t)
	ctx, taskValue := sqliteAsyncCtx()
	crossings := 0
	ctx.BeforeIrreversibleEffect = func(string) bool {
		crossings++
		ctx.ConflictRetryRequested = true
		return true
	}
	result := admitTestBuiltin("exec", ctx, []types.Value{types.NewList([]types.Value{types.NewStr(program)})})
	if result.Flow != types.FlowAbortAttempt || crossings != 1 || ctx.IrreversibleSideEffect {
		t.Fatalf("exec = %+v crossings = %d irreversible = %v, want aborted before effect", result, crossings, ctx.IrreversibleSideEffect)
	}
	if taskValue.IsExecSuspended || taskValue.ExecCancelFunc != nil || taskValue.GetState() != task.TaskRunning {
		t.Fatalf("exec-suspended = %v state = %v, want a running task with no process", taskValue.IsExecSuspended, taskValue.GetState())
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
