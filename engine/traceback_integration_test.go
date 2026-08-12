package engine

import (
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"strings"
	"testing"
)

// A verb that raises an uncaught error must produce exactly one log record
// carrying the whole traceback. This drives the real scheduler — a compiled verb
// raising a real E_TYPE — rather than a fabricated call stack, so it covers the
// actual call site in runTask rather than the formatting in isolation.
func TestUncaughtExceptionInRealTaskLogsOneRecord(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	store.AddVerb(0, dbstore.NewVerb("boom", []string{"boom"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{`x = 1 + "a";`}))

	s := NewRuntime(store)

	records := captureRecords(t, func() {
		if _, err := s.RunServerVerbTask(0, "boom", nil, 2); err != nil {
			t.Fatalf("run verb task: %v", err)
		}
	})

	var found map[string]any
	for _, rec := range records {
		if rec["msg"] == "uncaught exception" {
			if found != nil {
				t.Fatal("the traceback was logged as more than one record")
			}
			found = rec
		}
	}
	if found == nil {
		t.Fatalf("no uncaught-exception record was logged; got %#v", records)
	}

	if found["error"] != "E_TYPE" {
		t.Errorf("error = %v, want E_TYPE", found["error"])
	}
	if found["verb"] != "boom" {
		t.Errorf("verb = %v, want boom", found["verb"])
	}

	// The frames must locate the failure: the raising verb, and the line of source
	// that raised. That is the "why" an operator is reading the log for.
	frames, ok := found["frames"].([]any)
	if !ok || len(frames) == 0 {
		t.Fatalf("record carries no structured frames: %#v", found["frames"])
	}
	top, ok := frames[0].(map[string]any)
	if !ok {
		t.Fatalf("frame is not an object: %#v", frames[0])
	}
	if top["verb"] != "boom" {
		t.Errorf("top frame verb = %v, want boom", top["verb"])
	}
	if top["source"] != `x = 1 + "a";` {
		t.Errorf("top frame source = %v, want the raising line", top["source"])
	}
	if top["line"] != float64(1) {
		t.Errorf("top frame line = %v, want 1", top["line"])
	}

	// And the rendered text must be present and complete, so a human reads the
	// same traceback the player would have seen.
	tb, _ := found["traceback"].(string)
	if !strings.Contains(tb, "(End of traceback)") || !strings.Contains(tb, "Type mismatch") {
		t.Errorf("traceback text is missing or truncated: %q", tb)
	}
}

func TestUncaughtForkInvokesDatabaseErrorHandler(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(0, "handled_args", dbstore.NewProperty(
		types.NewEmptyList(), 2,
		dbstore.PropRead|dbstore.PropWrite, false, true,
	)); errCode != types.E_NONE {
		t.Fatalf("define handled_args: %v", errCode)
	}
	store.AddVerb(0, dbstore.NewVerb("handle_uncaught_error", []string{"handle_uncaught_error"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"this.handled_args = args;", "return 1;"}))
	store.AddVerb(0, dbstore.NewVerb("forkboom", []string{"forkboom"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"fork (0)", `  raise(E_INVARG, "custom uncaught message", {7, 8});`, "endfork", "return 1;"}))

	s := NewRuntime(store)
	fallbacks := 0
	s.SetTracebackSender(func(types.ObjID, types.ErrorCode, []task.ActivationFrame) {
		fallbacks++
	})
	if _, err := s.RunServerVerbTask(0, "forkboom", nil, 2); err != nil {
		t.Fatalf("run forkboom: %v", err)
	}
	s.ProcessReadyTasks()

	handledArgs, errCode := store.PropertyValue(0, "handled_args")
	if errCode != types.E_NONE {
		t.Fatalf("read handled_args: %v", errCode)
	}
	if handledArgs.Type() != types.TYPE_LIST || len(handledArgs.Elements()) != 5 {
		t.Fatalf("handle_uncaught_error args = %s, want five-element list", handledArgs.String())
	}
	if got := handledArgs.Elements()[0]; got.Type() != types.TYPE_ERR || got.ErrCode() != types.E_INVARG {
		t.Errorf("error code argument = %s, want E_INVARG", got.String())
	}
	if got := handledArgs.Elements()[1]; got.Type() != types.TYPE_STR || got.Str() != "custom uncaught message" {
		t.Errorf("message argument = %s, want %q", got.String(), "custom uncaught message")
	}
	wantValue := types.NewList([]types.Value{types.NewInt(7), types.NewInt(8)})
	if got := handledArgs.Elements()[2]; !got.Equal(wantValue) {
		t.Errorf("value argument = %s, want %s", got.String(), wantValue.String())
	}
	if got := handledArgs.Elements()[3]; got.Type() != types.TYPE_LIST || len(got.Elements()) == 0 {
		t.Errorf("stack argument = %s, want non-empty list", got.String())
	} else {
		frame := got.Elements()[0]
		if frame.Type() != types.TYPE_LIST || len(frame.Elements()) < 3 {
			t.Errorf("first stack frame = %s, want six-field list", frame.String())
		} else if programmer := frame.Elements()[2]; programmer.Type() != types.TYPE_OBJ || programmer.Obj() != 2 {
			t.Errorf("first stack frame programmer = %s, want #2", programmer.String())
		}
	}
	if got := handledArgs.Elements()[4]; got.Type() != types.TYPE_LIST || len(got.Elements()) == 0 {
		t.Errorf("formatted argument = %s, want non-empty list", got.String())
	}
	if fallbacks != 0 {
		t.Fatalf("fallback traceback sends = %d, want 0 after truthy handler", fallbacks)
	}
}

func TestTruthyTaskTimeoutHandlerSuppressesGenericExceptionFallback(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(0, "timeout_args", dbstore.NewProperty(
		types.NewEmptyList(), 2,
		dbstore.PropRead|dbstore.PropWrite, false, true,
	)); errCode != types.E_NONE {
		t.Fatalf("define timeout_args: %v", errCode)
	}
	store.AddVerb(0, dbstore.NewVerb("handle_task_timeout", []string{"handle_task_timeout"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"this.timeout_args = args;", "return 1;"}))
	spinCode := []string{"while (1)", "endwhile"}
	store.AddVerb(0, dbstore.NewVerb("spin", []string{"spin"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, spinCode))

	s := NewRuntime(store)
	program, diagnostics := s.registry.Compiler().CompileMOO(spinCode)
	if len(diagnostics) != 0 {
		t.Fatalf("compile spin: %v", diagnostics)
	}
	timeoutTask := task.NewTaskFull(1, 2, program, 20, 1)
	s.populateTaskContextDependencies(timeoutTask.Context)
	timeoutTask.Programmer = 2
	timeoutTask.This = 0
	timeoutTask.Caller = 2
	timeoutTask.VerbName = "spin"
	timeoutTask.VerbLoc = 0
	timeoutTask.IsForked = true
	timeoutTask.Kind = task.TaskForked

	fallbacks := 0
	s.SetTracebackSender(func(types.ObjID, types.ErrorCode, []task.ActivationFrame) {
		fallbacks++
	})
	if err := s.runTask(timeoutTask); err != nil {
		t.Fatalf("run timeout task: %v", err)
	}

	timeoutArgs, errCode := store.PropertyValue(0, "timeout_args")
	if errCode != types.E_NONE {
		t.Fatalf("read timeout_args: %v", errCode)
	}
	if timeoutArgs.Type() != types.TYPE_LIST || len(timeoutArgs.Elements()) != 3 {
		t.Fatalf("handle_task_timeout args = %s, want three-element list", timeoutArgs.String())
	}
	if got := timeoutArgs.Elements()[0]; got.Type() != types.TYPE_STR || got.Str() != "ticks" {
		t.Errorf("resource argument = %s, want ticks", got.String())
	}
	if fallbacks != 0 {
		t.Fatalf("fallback traceback sends = %d, want 0 after truthy timeout handler", fallbacks)
	}
}
