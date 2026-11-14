package engine

import (
	"path/filepath"
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
// fork bodies.
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

// A task paused inside a for loop keeps loop state Toast holds differently
// (the container and a cursor); the checkpoint leaves it out rather than
// write a frame Toast would misread.
func TestTaskInRefusedShapeIsLeftOutOfCheckpoint(t *testing.T) {
	store := newToastExportStore(t, nil)
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	ticks, seconds := backgroundTaskLimits(rt.session)
	started := NewTaskFull(7200, 2, compileTestProgram(t, rt.registry, "for x in ({1, 2})\nsuspend(100);\nendfor"), ticks, seconds)
	rt.populateContextDependencies(started.Context)
	started.IsForked = true
	started.ForkCreator = rt
	rt.QueueTask(started)
	rt.ProcessReadyTasks()
	if state := started.GetState(); state != task.TaskSuspended {
		t.Fatalf("task state = %v, want suspended", state)
	}
	if _, suspended := rt.TaskSnapshots(); len(suspended) != 0 {
		t.Fatalf("checkpoint kept %d tasks, want the for-loop task left out", len(suspended))
	}
}
