package engine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// toastFinallyFixture is a Toast checkpoint holding one task suspended inside
// a finally body. Toast keeps why the finally is running, and the value that
// goes with it, as two operands of the frame (execute.cc Finally_Reason,
// unwind_stack). See testdata/toast-suspended-finally.md for the producers.
type toastFinallyFixture struct {
	name     string
	file     string
	hash     string
	property string
	// The frame holding the finally, counted down from the task's top frame,
	// and the stack slot of its reason operand; the value is the next slot.
	frameBelowTop int
	reasonSlot    int
	reason        int64
	value         string
	// The test object's properties, and their values in the checkpoint and
	// after the task has ended.
	properties []string
	saved      string
	resumed    string
	state      task.TaskState
}

var toastFinallyFixtures = []toastFinallyFixture{
	{
		name: "fall through", file: "toast-suspended-finally-fall-through.db",
		hash:     "4b6a3a1564b4e230fa6659b3a88a2d52f52a974d204c57cdf3c983af35309c39",
		property: "conformance_finally_fall_through",
		reason:   0, value: "0",
		properties: []string{"before", "after", "finished"},
		saved:      "{1, 0, 0}", resumed: "{1, 1, 1}", state: task.TaskCompleted,
	},
	{
		// The finally body's verb call is what suspended, so the finally is
		// in the caller's frame; it was entered by `return 5`.
		name: "return, suspended in a called verb", file: "toast-suspended-finally-called-verb.db",
		hash:          "679bc6b2f520fe46aa5b8d979e3326243b6035b6692966285f7c86e90f160bf2",
		property:      "conformance_finally_called_verb",
		frameBelowTop: 1,
		reason:        3, value: "5",
		properties: []string{"before", "after", "result", "finished"},
		saved:      "{1, 0, 0, 0}", resumed: "{1, 42, 5, 1}", state: task.TaskCompleted,
	},
	{
		// Below the reason are the enclosing try's codes, handler label and
		// catch marker. The value is {code, message, value, stack}.
		name: "raise", file: "toast-suspended-finally-raise.db",
		hash:       "9044f35900904ba2da2c07766c6fd999a1145c83700a3098990b971bafce4a27",
		property:   "conformance_finally_raise",
		reasonSlot: 3,
		reason:     1, value: "{E_PERM, \"finally raise\", 7, ",
		properties: []string{"before", "after", "skipped", "result", "finished"},
		saved:      "{1, 0, 0, 0, 0}", resumed: "{1, 1, 0, {E_PERM, \"finally raise\", 7}, 1}", state: task.TaskCompleted,
	},
	{
		// Toast holds no error for this reason: it kept the error outside the
		// task for #0:handle_uncaught_error, and suspending dropped it. The
		// task ends, through its caller's finally body ("outer"), without the
		// handler being called; "finally control" is from a task that did not
		// suspend.
		name: "uncaught", file: "toast-suspended-finally-uncaught.db",
		hash:     "247d0350bb0ca36dcd2f70b83ae6b148d133e4619fe741b14c65d46e80023c73",
		property: "conformance_finally_uncaught",
		reason:   2, value: "0",
		properties: []string{"before", "after", "outer", "finished", "control", "handled"},
		saved:      "{1, 0, 0, 0, 1, {\"finally control\"}}", resumed: "{1, 1, 1, 0, 1, {\"finally control\"}}", state: task.TaskKilled,
	},
	{
		name: "return", file: "toast-suspended-finally-return.db",
		hash:     "fd12b742469e8fe607e7219f43b0a1c1501aebdd0018dc79051ca3c155abcff5",
		property: "conformance_finally_return",
		reason:   3, value: "5",
		properties: []string{"before", "after", "skipped", "result", "finished"},
		saved:      "{1, 0, 0, 0, 0}", resumed: "{1, 1, 0, 5, 1}", state: task.TaskCompleted,
	},
	{
		// Below the reason are the list loop's container and cursor. The
		// value is {stack depth to unwind to, pc to jump to}.
		name: "break", file: "toast-suspended-finally-break.db",
		hash:       "d466f7040b8d55a8fefa79da5051407c68e55a626b096243f867bfd88b8cfaed",
		property:   "conformance_finally_break",
		reasonSlot: 2,
		reason:     5, value: "{0, 77}",
		properties: []string{"before", "after", "count", "finished"},
		saved:      "{1, 0, 1, 0}", resumed: "{1, 1, 1, 1}", state: task.TaskCompleted,
	},
	{
		// Below the reason are the range loop's next value and limit, which
		// a continue keeps on the stack.
		name: "continue", file: "toast-suspended-finally-continue.db",
		hash:       "40439f212e59b770e95cce163e40546f0cbfbe2c7208d9d485826541807056f2",
		property:   "conformance_finally_continue",
		reasonSlot: 2,
		reason:     5, value: "{2, 2}",
		properties: []string{"before", "after", "count", "finished"},
		saved:      "{1, 0, 1, 0}", resumed: "{1, 1, 4, 1}", state: task.TaskCompleted,
	},
}

// A Toast task suspended inside a finally body is restored, written back with
// the operands Toast saved, and then continues what brought it into the
// finally: falling through, returning, raising to a handler, ending with no
// handler, or leaving a loop.
func TestToastSuspendedFinallyImportContinuesTheReason(t *testing.T) {
	for _, fixture := range toastFinallyFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			database := loadNativeImportFixture(t, "testdata/"+fixture.file, fixture.hash)
			if len(database.SuspendedTasks) != 1 {
				t.Fatalf("native suspended tasks=%d, want 1", len(database.SuspendedTasks))
			}
			saved := database.SuspendedTasks[0]
			toast := saved.Snapshot.VM.Frames
			holder := len(toast) - 1 - fixture.frameBelowTop
			if holder < 0 || len(toast[holder].Stack) < fixture.reasonSlot+2 {
				t.Fatalf("fixture frames=%d, finally frame stack=%v", len(toast), toast[max(holder, 0)].Stack)
			}
			reason, value := toast[holder].Stack[fixture.reasonSlot], toast[holder].Stack[fixture.reasonSlot+1]
			if !reason.Equal(types.NewInt(fixture.reason)) || !strings.HasPrefix(value.String(), fixture.value) {
				t.Fatalf("fixture finally reason/value=%s/%s, want %d/%s", reason, value, fixture.reason, fixture.value)
			}
			store, err := database.NewStoreFromDatabase()
			if err != nil {
				t.Fatal(err)
			}
			object, code := store.DirectTxn().PropertyValue(1, fixture.property)
			if code != types.E_NONE || object.Type() != types.TYPE_OBJ {
				t.Fatalf("native fixture object=%s error=%s", object, code)
			}
			read := func() string {
				t.Helper()
				values := make([]types.Value, len(fixture.properties))
				for index, name := range fixture.properties {
					value, code := store.DirectTxn().PropertyValue(object.Obj(), name)
					if code != types.E_NONE {
						t.Fatalf("%s error=%s", name, code)
					}
					values[index] = value
				}
				return types.NewList(values).String()
			}
			if got := read(); got != fixture.saved {
				t.Fatalf("checkpoint %v=%s, want %s", fixture.properties, got, fixture.saved)
			}
			rt := newTestRuntimeWithBuiltins(t, store)
			defer rt.Stop()
			rt.Session().LoadServerOptionsFromStore(store)
			rt.Session().LoadProtectedBuiltinsFromStore(store)
			saved.Snapshot.StartTime = time.Now().Add(-time.Second)
			if err := rt.LoadSuspendedTasks(database.SuspendedTasks); err != nil {
				t.Fatal(err)
			}
			restored := rt.GetTask(saved.Snapshot.ID)
			if restored == nil {
				t.Fatalf("native suspended task %d was not restored", saved.Snapshot.ID)
			}

			// The restored task checkpoints as the frames Toast wrote: same
			// pc and the same operands, the reason and its value among them.
			_, suspended := rt.TaskSnapshots()
			if len(suspended) != 1 || len(suspended[0].VM.Frames) != len(toast) {
				t.Fatalf("checkpoint of the restored task holds %d tasks", len(suspended))
			}
			marker := func(kind int) int {
				if kind == 7 || kind == 8 {
					return kind
				}
				return 0
			}
			for index, frame := range suspended[0].VM.Frames {
				want := toast[index]
				if frame.Native == nil {
					t.Fatalf("frame %d was not written in Toast's form", index)
				}
				if frame.IP != want.IP || frame.Native.ErrorPC != want.Native.ErrorPC || frame.BuiltinPC != want.BuiltinPC || len(frame.Stack) != len(want.Stack) {
					t.Fatalf("frame %d written with pc/error pc/builtin pc/stack=%d/%d/%d/%v, Toast wrote %d/%d/%d/%v", index, frame.IP, frame.Native.ErrorPC, frame.BuiltinPC, frame.Stack, want.IP, want.Native.ErrorPC, want.BuiltinPC, want.Stack)
				}
				for slot := range frame.Stack {
					if !frame.Stack[slot].Equal(want.Stack[slot]) || marker(frame.Native.OperandTypes[slot]) != marker(want.Native.OperandTypes[slot]) {
						t.Fatalf("frame %d written with stack %v, Toast wrote %v", index, frame.Stack, want.Stack)
					}
				}
			}

			if state := settleNativeImportedContinuation(t, rt, restored); state != fixture.state {
				t.Fatalf("task ended in state %v, want %v", state, fixture.state)
			}
			if got := read(); got != fixture.resumed {
				t.Fatalf("resumed %v=%s, want %s", fixture.properties, got, fixture.resumed)
			}
		})
	}
}

// Toast holds an uncaught error outside the task while its finally bodies
// run, and drops it when the task suspends. So an uncaught error whose
// finally body suspends ends the task without #0:handle_uncaught_error, which
// the same error does reach when nothing suspends; and the suspended task
// checkpoints as Toast's FIN_UNCAUGHT, reason 2 with value 0.
func TestUncaughtErrorSuspendedInFinallyEndsUnreported(t *testing.T) {
	store := newToastExportStore(t, map[string]string{
		"handle_uncaught_error": "#0.log = {@#0.log, args[2]};\nreturn 1;",
	})
	rt := newTestRuntimeWithBuiltins(t, store)
	ticks, seconds := backgroundTaskLimits(rt.session)
	control := NewTaskFull(7101, 2, compileTestProgram(t, rt.registry, "try\nraise(E_PERM, \"control\");\nfinally\n#0.n = #0.n + 100;\nendtry"), ticks, seconds)
	rt.populateContextDependencies(control.Context)
	control.IsForked = true
	control.ForkCreator = rt
	rt.QueueTask(control)
	if state := settleNativeImportedContinuation(t, rt, control); state != task.TaskKilled {
		t.Fatalf("control task ended in state %v, want %v", state, task.TaskKilled)
	}
	suspended := pauseBarnTask(t, rt, "try\nraise(E_PERM, \"suspended\");\nfinally\n#0.n = #0.n + 1;\nsuspend(100);\n#0.n = #0.n + 1;\nendtry\n#0.n = #0.n + 10;")
	rt.Stop()
	if len(suspended) != 1 || len(suspended[0].VM.Frames) != 1 {
		t.Fatalf("suspended snapshots = %d, want one task of one frame", len(suspended))
	}
	frame := suspended[0].VM.Frames[0]
	if frame.Native == nil || len(frame.Stack) != 2 || !frame.Stack[0].Equal(types.NewInt(2)) || !frame.Stack[1].Equal(types.NewInt(0)) {
		t.Fatalf("frame written in Toast's form = %v with stack %v, want reason 2 and value 0", frame.Native != nil, frame.Stack)
	}
	path := filepath.Join(t.TempDir(), "barn.db")
	if err := dbformat.WriteCheckpoint(path, store, nil, suspended, nil); err != nil {
		t.Fatal(err)
	}
	reloaded, err := dbformat.LoadDatabase(path + ".new")
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.SuspendedTasks) != 1 {
		t.Fatalf("reloaded suspended tasks = %d, want 1", len(reloaded.SuspendedTasks))
	}
	resumedStore, err := reloaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	resumed := newTestRuntimeWithBuiltins(t, resumedStore)
	defer resumed.Stop()
	reloaded.SuspendedTasks[0].Snapshot.StartTime = time.Now().Add(-time.Second)
	if err := resumed.LoadSuspendedTasks(reloaded.SuspendedTasks); err != nil {
		t.Fatal(err)
	}
	restored := resumed.GetTask(reloaded.SuspendedTasks[0].Snapshot.ID)
	if restored == nil {
		t.Fatal("suspended task was not restored")
	}
	if state := settleNativeImportedContinuation(t, resumed, restored); state != task.TaskKilled {
		t.Fatalf("task ended in state %v, want %v", state, task.TaskKilled)
	}
	for property, want := range map[string]string{"log": "{\"control\"}", "n": "102"} {
		if got, code := resumedStore.DirectTxn().PropertyValue(0, property); code != types.E_NONE || got.String() != want {
			t.Fatalf("#0.%s = %s (%s), want %s", property, got, code, want)
		}
	}
}
