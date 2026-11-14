package engine

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// toastExportShape is a task Barn starts itself, paused in one of the frame
// shapes the real Mongoose checkpoint holds: a builtin or verb call inside
// while loops, try/except and try/finally bodies, catch expressions, and
// fork bodies; and inside list, map, range and string for loops.
type toastExportShape struct {
	name    string
	verbs   map[string]string
	program string
	want    string
}

var toastExportShapes = []toastExportShape{
	{
		name:    "main vector builtin",
		program: "#0.log = {@#0.log, 1};\nsuspend(100);\n#0.log = {@#0.log, 2};",
		want:    "{1, 2}",
	},
	{
		name: "operand held below a verb call, builtin in a while loop",
		verbs: map[string]string{
			"outer": "x = 100;\nr = x + this:inner();\n#0.log = {@#0.log, r};",
			"inner": "while (1)\nsuspend(100);\nreturn 73;\nendwhile",
		},
		program: "#0:outer();",
		want:    "{173}",
	},
	{
		name: "verb call inside try/except inside a try/finally body",
		verbs: map[string]string{
			"outer": "try\ntry\nr = this:inner();\n#0.log = {@#0.log, r};\nexcept (ANY)\n#0.log = {@#0.log, \"except\"};\nendtry\nfinally\n#0.log = {@#0.log, \"finally\"};\nendtry",
			"inner": "suspend(100);\nreturn \"inner\";",
		},
		program: "#0:outer();",
		want:    "{\"inner\", \"finally\"}",
	},
	{
		name:    "catch expression in a while condition inside a try/finally body",
		program: "n = 0;\ntry\nwhile ((n < 1) && (typeof(v = `suspend(100) ! ANY') == INT))\nn = n + 1;\nendwhile\nfinally\n#0.log = {@#0.log, n};\nendtry",
		want:    "{1}",
	},
	{
		name:    "builtin in a while loop in a fork body",
		program: "{interval} = {100};\nfork (0)\nwhile (#0.n < 1)\n#0.n = #0.n + 1;\nsuspend(interval);\nendwhile\n#0.log = {@#0.log, #0.n};\nendfork",
		want:    "{1}",
	},
	{
		name: "verb call that is a whole fork body",
		verbs: map[string]string{
			"inner": "suspend(100);\n#0.log = {@#0.log, \"served\"};",
		},
		program: "fork (0)\n#0:inner();\nendfork",
		want:    "{\"served\"}",
	},
	{
		name: "verb call inside a list loop",
		verbs: map[string]string{
			"pause": "if (#0.n == 0)\n#0.n = 1;\nsuspend(100);\nendif\nreturn 1;",
		},
		program: "for x in ({10, 20})\n#0.log = {@#0.log, x + #0:pause()};\nendfor",
		want:    "{11, 21}",
	},
	{
		name: "verb call inside a self-appending list",
		verbs: map[string]string{
			"pause": "suspend(100);\nreturn 1;",
		},
		program: "r = {5};\nr = {@r, #0:pause(), 7};\n#0.log = r;",
		want:    "{5, 1, 7}",
	},
	{
		name:    "builtin at the last element of a list loop",
		program: "for x in ({1, 2})\nif (x == 2)\nsuspend(100);\nendif\n#0.log = {@#0.log, x};\nendfor",
		want:    "{1, 2}",
	},
	{
		name:    "builtin inside a map loop binding keys",
		program: "for v, k in ([\"a\" -> 1, \"b\" -> 2, \"c\" -> 3])\nif (k == \"a\")\nsuspend(100);\nendif\n#0.log = {@#0.log, {k, v}};\nendfor",
		want:    "{{\"a\", 1}, {\"b\", 2}, {\"c\", 3}}",
	},
	{
		name:    "builtin at the last element of a map loop",
		program: "for v in ([\"a\" -> 1, \"b\" -> 2])\nif (v == 2)\nsuspend(100);\nendif\n#0.log = {@#0.log, v};\nendfor",
		want:    "{1, 2}",
	},
	{
		name:    "builtin inside a range loop",
		program: "for i in [1..3]\nif (i == 2)\nsuspend(100);\nendif\n#0.log = {@#0.log, i};\nendfor",
		want:    "{1, 2, 3}",
	},
	{
		name:    "builtin inside a string loop",
		program: "for c in (\"ab\")\nif (c == \"a\")\nsuspend(100);\nendif\n#0.log = {@#0.log, c};\nendfor",
		want:    "{\"a\", \"b\"}",
	},
	{
		name: "verb call in a list loop inside a map loop in a fork body",
		verbs: map[string]string{
			"pause": "if (#0.n == 0)\n#0.n = 1;\nsuspend(100);\nendif\nreturn 0;",
		},
		program: "fork (0)\nfor v, k in ([\"p\" -> {1, 2}])\nfor x in (v)\n#0:pause();\n#0.log = {@#0.log, {k, x}};\nendfor\nendfor\nendfork",
		want:    "{{\"p\", 1}, {\"p\", 2}}",
	},
	{
		name:    "fork body inside a collection loop",
		program: "for x in ({7})\nfork (0)\nsuspend(100);\n#0.log = {@#0.log, x};\nendfork\nendfor",
		want:    "{7}",
	},
}

func newToastExportStore(t *testing.T, verbs map[string]string) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	for name, value := range map[string]types.Value{"log": types.NewList(nil), "n": types.NewInt(0)} {
		if code := store.DirectTxn().DefineProperty(0, name, dbstore.NewProperty(value, 2, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
			t.Fatalf("define #0.%s: %s", name, code)
		}
	}
	for name, source := range verbs {
		store.AddVerb(0, dbstore.NewVerb([]string{name}, 2,
			dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
			dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
			strings.Split(source, "\n")))
	}
	return store
}

// pauseBarnTask runs program as a background task until some task it started
// is suspended, and returns the checkpoint's task snapshots.
func pauseBarnTask(t *testing.T, rt *Runtime, program string) []task.Snapshot {
	t.Helper()
	ticks, seconds := backgroundTaskLimits(rt.session)
	started := NewTaskFull(7100, 2, compileTestProgram(t, rt.registry, program), ticks, seconds)
	rt.populateContextDependencies(started.Context)
	started.IsForked = true
	started.ForkCreator = rt
	rt.QueueTask(started)
	for range 10 {
		rt.ProcessReadyTasks()
		if _, suspended := rt.TaskSnapshots(); len(suspended) != 0 {
			return suspended
		}
	}
	t.Fatal("program never paused")
	return nil
}

// A Barn-started task paused in each real frame shape is written in Toast's
// form: every frame carries Toast's pc and operand stack, the file reloads
// those frames unchanged, Barn's own import accepts them, and the task then
// finishes exactly as it would have without the checkpoint.
func TestBarnSuspendedTaskShapesCheckpointAsToastFrames(t *testing.T) {
	for _, shape := range toastExportShapes {
		t.Run(shape.name, func(t *testing.T) {
			store := newToastExportStore(t, shape.verbs)
			rt := newTestRuntimeWithBuiltins(t, store)
			suspended := pauseBarnTask(t, rt, shape.program)
			original := rt.GetTask(suspended[0].ID).PersistenceSnapshot()
			rt.Stop()
			if len(suspended) != 1 {
				t.Fatalf("suspended snapshots = %d, want 1", len(suspended))
			}
			exported := suspended[0].VM
			for index, frame := range exported.Frames {
				if frame.Native == nil {
					t.Fatalf("frame %d was not written in Toast's form", index)
				}
				if index > 0 && exported.NativeRootVector < -1 {
					t.Fatalf("invalid root vector %d", exported.NativeRootVector)
				}
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
			saved := reloaded.SuspendedTasks[0].Snapshot
			if saved.VM.NativeRootVector != exported.NativeRootVector || saved.VM.NativeFunctionID != 0 {
				t.Fatalf("reloaded VM header vector/func = %d/%d, want %d/0", saved.VM.NativeRootVector, saved.VM.NativeFunctionID, exported.NativeRootVector)
			}
			for index, frame := range saved.VM.Frames {
				want := exported.Frames[index]
				if frame.Native == nil || frame.IP != want.IP || len(frame.Stack) != len(want.Stack) {
					t.Fatalf("reloaded frame %d: native=%v ip=%d stack=%d, want ip=%d stack=%d", index, frame.Native != nil, frame.IP, len(frame.Stack), want.IP, len(want.Stack))
				}
				// The file holds each operand's type code; the export marks
				// only Toast's control markers (7 catch, 8 finally).
				marker := func(kind int) bool { return kind == 7 || kind == 8 }
				for slot, value := range frame.Stack {
					if !value.Equal(want.Stack[slot]) || marker(frame.Native.OperandTypes[slot]) != marker(want.Native.OperandTypes[slot]) || (marker(frame.Native.OperandTypes[slot]) && frame.Native.OperandTypes[slot] != want.Native.OperandTypes[slot]) {
						t.Fatalf("reloaded frame %d slot %d = %s (type %d), want %s (type %d)", index, slot, value, frame.Native.OperandTypes[slot], want.Stack[slot], want.Native.OperandTypes[slot])
					}
				}
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
			restored := resumed.GetTask(saved.ID)
			if restored == nil {
				t.Fatal("Toast-form task was not restored")
			}
			// The Toast-form frames, imported, export to the same Toast pc,
			// error pc, operands and root vector again.
			_, again := resumed.TaskSnapshots()
			if len(again) != 1 || again[0].VM.NativeRootVector != exported.NativeRootVector {
				t.Fatalf("re-export = %d tasks", len(again))
			}
			for index, frame := range again[0].VM.Frames {
				want := exported.Frames[index]
				if frame.IP != want.IP || frame.Native.ErrorPC != want.Native.ErrorPC || len(frame.Stack) != len(want.Stack) {
					t.Fatalf("re-exported frame %d pc/error pc/stack = %d/%d/%v, want %d/%d/%v", index, frame.IP, frame.Native.ErrorPC, frame.Stack, want.IP, want.Native.ErrorPC, want.Stack)
				}
				for slot := range frame.Stack {
					if !frame.Stack[slot].Equal(want.Stack[slot]) || frame.Native.OperandTypes[slot] != want.Native.OperandTypes[slot] {
						t.Fatalf("re-exported frame %d stack %v, want %v", index, frame.Stack, want.Stack)
					}
				}
			}
			// A main-vector frame converts back to the Barn frame it was: same
			// code, position, operands and named variables. A fork frame comes
			// back compiled as Toast's fork vector, so it is checked by resuming.
			for index, frame := range restored.PersistenceSnapshot().VM.Frames {
				before := original.VM.Frames[index]
				if before.Program.Origin.Fork != 0 {
					continue
				}
				if string(frame.Program.Code) != string(before.Program.Code) || frame.IP != before.IP {
					t.Fatalf("frame %d came back at ip %d of different code, want ip %d", index, frame.IP, before.IP)
				}
				if len(frame.Stack) != len(before.Stack) {
					t.Fatalf("frame %d stack = %v, want %v", index, frame.Stack, before.Stack)
				}
				for slot := range frame.Stack {
					if !frame.Stack[slot].Equal(before.Stack[slot]) {
						t.Fatalf("frame %d stack = %v, want %v", index, frame.Stack, before.Stack)
					}
				}
				for slot, name := range before.Program.VarNames {
					// A variable Toast holds unbound at the call (OP_PUSH_CLEAR)
					// is not in the exported frame and comes back unbound.
					if !slices.ContainsFunc(exported.Frames[index].Program.VarNames, func(kept string) bool { return strings.EqualFold(kept, name) }) {
						if !frame.Locals[slot].IsUnbound() {
							t.Fatalf("frame %d variable %s = %s, want unbound", index, name, frame.Locals[slot])
						}
						continue
					}
					if !frame.Locals[slot].Equal(before.Locals[slot]) {
						t.Fatalf("frame %d variable %s = %s, want %s", index, name, frame.Locals[slot], before.Locals[slot])
					}
				}
			}
			driveNativeImportedContinuation(t, resumed, restored)
			log, code := resumedStore.DirectTxn().PropertyValue(0, "log")
			if code != types.E_NONE || log.String() != shape.want {
				t.Fatalf("#0.log = %s (%s), want %s", log, code, shape.want)
			}
		})
	}
}

const (
	toastFormProgram = "#0.log = {@#0.log, \"toast\"};\nsuspend(100);\n#0.log = {@#0.log, \"toast resumed\"};"
	// Paused inside a finally clause: Toast holds the pending reason there,
	// which Barn does not write in Toast's form.
	barnFormProgram = "try\n#0.other = {@#0.other, \"body\"};\nfinally\nsuspend(100);\n#0.other = {@#0.other, \"finally resumed\"};\nendtry"
)

// pauseMixedTasks starts one task in a shape Toast can hold and one it
// cannot, lets both suspend, and returns the checkpoint's snapshots.
func pauseMixedTasks(t *testing.T, rt *Runtime) []task.Snapshot {
	t.Helper()
	ticks, seconds := backgroundTaskLimits(rt.session)
	for id, program := range map[int64]string{7201: toastFormProgram, 7202: barnFormProgram} {
		started := NewTaskFull(id, 2, compileTestProgram(t, rt.registry, program), ticks, seconds)
		rt.populateContextDependencies(started.Context)
		started.IsForked = true
		started.ForkCreator = rt
		rt.QueueTask(started)
	}
	for range 4 {
		rt.ProcessReadyTasks()
	}
	_, suspended := rt.TaskSnapshots()
	if len(suspended) != 2 {
		t.Fatalf("checkpoint holds %d tasks, want both", len(suspended))
	}
	for _, snapshot := range suspended {
		native := snapshot.VM.Frames[0].Native != nil
		if native != (snapshot.ID == 7201) {
			t.Fatalf("task %d written in Toast's form = %v", snapshot.ID, native)
		}
	}
	return suspended
}

// A live checkpoint never leaves a task out: one Toast cannot hold is kept
// whole in Barn's frame form beside the Toast-form ones, and Barn reloads and
// finishes all of them.
func TestCheckpointKeepsBarnFormTaskBesideToastFormTasks(t *testing.T) {
	store := newToastExportStore(t, nil)
	if code := store.DirectTxn().DefineProperty(0, "other", dbstore.NewProperty(types.NewList(nil), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
		t.Fatalf("define #0.other: %s", code)
	}
	rt := newTestRuntimeWithBuiltins(t, store)
	suspended := pauseMixedTasks(t, rt)
	rt.Stop()
	path := filepath.Join(t.TempDir(), "barn.db")
	if err := dbformat.WriteCheckpoint(path, store, nil, suspended, nil); err != nil {
		t.Fatal(err)
	}
	reloaded, err := dbformat.LoadDatabase(path + ".new")
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.SuspendedTasks) != 2 {
		t.Fatalf("reloaded %d tasks, want 2", len(reloaded.SuspendedTasks))
	}
	resumedStore, err := reloaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	resumed := newTestRuntimeWithBuiltins(t, resumedStore)
	defer resumed.Stop()
	for _, saved := range reloaded.SuspendedTasks {
		native := saved.Snapshot.VM.Frames[0].Native != nil
		if native != (saved.Snapshot.ID == 7201) {
			t.Fatalf("reloaded task %d read as Toast's form = %v", saved.Snapshot.ID, native)
		}
		saved.Snapshot.StartTime = time.Now().Add(-time.Second)
	}
	if err := resumed.LoadSuspendedTasks(reloaded.SuspendedTasks); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{7201, 7202} {
		restored := resumed.GetTask(id)
		if restored == nil {
			t.Fatalf("task %d was not restored", id)
		}
		driveNativeImportedContinuation(t, resumed, restored)
	}
	for property, want := range map[string]string{"log": "{\"toast\", \"toast resumed\"}", "other": "{\"body\", \"finally resumed\"}"} {
		if got, code := resumedStore.DirectTxn().PropertyValue(0, property); code != types.E_NONE || got.String() != want {
			t.Fatalf("#0.%s = %s (%s), want %s", property, got, code, want)
		}
	}
}

// Toast compiles the last push of a variable before a store of it, or before
// the end of the verb, in one basic block as OP_PUSH_CLEAR, which unbinds it
// (code_gen.cc BYTECODE_REDUCE_REF). At the suspend below Toast holds `a`
// unbound (its push into `b` is its last use) and `b` bound (pushed after the
// suspend), so the exported frame names b and not a.
func TestExportedFrameUnbindsVariablesToastPushCleared(t *testing.T) {
	store := newToastExportStore(t, nil)
	rt := newTestRuntimeWithBuiltins(t, store)
	suspended := pauseBarnTask(t, rt, "a = {1};\nb = {a};\nsuspend(100);\n#0.log = b;")
	rt.Stop()
	if len(suspended) != 1 || len(suspended[0].VM.Frames) != 1 || suspended[0].VM.Frames[0].Native == nil {
		t.Fatal("task was not exported in Toast's form")
	}
	frame := suspended[0].VM.Frames[0]
	named := map[string]bool{}
	for _, name := range frame.Program.VarNames {
		named[strings.ToLower(name)] = true
	}
	if named["a"] || !named["b"] {
		t.Fatalf("exported variables %v, want b without a", frame.Program.VarNames)
	}
}

// A jump into the call's basic block between a cleared push and the call
// makes the cleared set depend on the path; the task stays in Barn's form.
func TestExportRefusesPathDependentPushClear(t *testing.T) {
	store := newToastExportStore(t, nil)
	rt := newTestRuntimeWithBuiltins(t, store)
	// `n` is pushed (into m) and stored again in the while condition's block,
	// whose loop top lies between: on the first pass the condition's call sees
	// n cleared, on later passes bound.
	suspended := pauseBarnTask(t, rt, "n = 1;\nm = n;\nwhile ((n = suspend(100)) == 0)\nendwhile\n#0.log = {m, n};")
	rt.Stop()
	if len(suspended) != 1 {
		t.Fatalf("suspended = %d, want 1", len(suspended))
	}
	if suspended[0].VM.Frames[0].Native != nil {
		t.Fatal("path-dependent frame was written in Toast's form")
	}
}

// The option for a file to hand back to Toast leaves Barn-form tasks out and
// names each one; without it (the live checkpoint) every task is kept.
func TestToastLoadableCheckpointOmitsBarnFormTasks(t *testing.T) {
	store := newToastExportStore(t, nil)
	if code := store.DirectTxn().DefineProperty(0, "other", dbstore.NewProperty(types.NewList(nil), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
		t.Fatalf("define #0.other: %s", code)
	}
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	suspended := pauseMixedTasks(t, rt)
	path := filepath.Join(t.TempDir(), "toast.db")
	checkpoint, err := dbformat.BeginCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint.OmitBarnFrameTasks()
	if err := checkpoint.Capture(store, nil, suspended, nil); err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.Serialize(); err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.Publish(); err != nil {
		t.Fatal(err)
	}
	omitted := checkpoint.Omitted()
	if err := checkpoint.Close(); err != nil {
		t.Fatal(err)
	}
	if len(omitted) != 1 || omitted[0] != 7202 {
		t.Fatalf("omitted = %v, want [7202]", omitted)
	}
	reloaded, err := dbformat.LoadDatabase(path + ".new")
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.SuspendedTasks) != 1 || reloaded.SuspendedTasks[0].Snapshot.ID != 7201 || reloaded.SuspendedTasks[0].Snapshot.VM.Frames[0].Native == nil {
		t.Fatalf("Toast-loadable checkpoint holds %d tasks, want only the Toast-form task 7201", len(reloaded.SuspendedTasks))
	}
}
