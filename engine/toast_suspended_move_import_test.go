package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Toast records move()'s continuation (builtin move, the pc do_move continues
// at when the hook returns, and its what/where/position) on the hook verb's
// activation. See testdata/toast-suspended-move.md for the producers.
type toastSuspendedMoveFixture struct {
	hook, hash string
	pc         int
	// The frame's pc line in the checkpoint: pc, builtin pc, error pc.
	pcLine string
	// The hooks that had run when the checkpoint was written, and whether
	// move() had already changed the object's location.
	logged []string
	moved  bool
	// Every hook entry and the suspended hook's resumption, in order.
	resumed []string
}

var toastSuspendedMoveFixtures = []toastSuspendedMoveFixture{
	{"accept", "430c730bfc4c722b2eb26e8b98a5f85683e7bcabfa72aa2e71c7851cc80f59ea", 2, "22 2 20",
		[]string{"accept"}, false,
		[]string{"accept", "accept resumed", "exitfunc", "enterfunc"}},
	{"exitfunc", "80fa40bafb261708520efa6647b2241af1cf6b8bcbf3ac247badf3de938ea205", 3, "22 3 20",
		[]string{"accept", "exitfunc"}, true,
		[]string{"accept", "exitfunc", "exitfunc resumed", "enterfunc"}},
	{"enterfunc", "4e3a934cefa184bec8c86968993d2ba2b20a539590a0a041aca2bd5af9675d17", 4, "22 4 20",
		[]string{"accept", "exitfunc", "enterfunc"}, true,
		[]string{"accept", "exitfunc", "enterfunc", "enterfunc resumed"}},
}

const toastSuspendedMoveData = "bf_move data: what = 11, where = 13, position = 0"

func (fixture toastSuspendedMoveFixture) load(t *testing.T) *dbformat.Database {
	t.Helper()
	database := loadNativeImportFixture(t, "testdata/toast-suspended-move-"+fixture.hook+".db", fixture.hash)
	if len(database.SuspendedTasks) != 1 {
		t.Fatalf("native suspended tasks=%d, want 1", len(database.SuspendedTasks))
	}
	// The frames are the fork body, start (paused at its move() call) and the
	// hook verb, paused in suspend().
	frames := database.SuspendedTasks[0].Snapshot.VM.Frames
	top := frames[len(frames)-1]
	if len(frames) != 3 || top.Verb != fixture.hook || top.Native == nil || top.Native.Builtin == nil ||
		top.Native.Builtin.Name != "move" || top.BuiltinPC != fixture.pc || top.Native.Builtin.State != toastSuspendedMoveData {
		t.Fatalf("fixture does not hold a %s frame with Toast's move continuation at pc %d: %d frames", fixture.hook, fixture.pc, len(frames))
	}
	return database
}

// state reads the producer's record of the move from store.
func (fixture toastSuspendedMoveFixture) state(t *testing.T, store *dbstore.Store) string {
	t.Helper()
	object, code := store.DirectTxn().PropertyValue(1, "conformance_suspended_move_"+fixture.hook)
	if code != types.E_NONE || object.Type() != types.TYPE_OBJ {
		t.Fatalf("native fixture object=%s error=%s", object, code)
	}
	read := func(name string) types.Value {
		value, code := store.DirectTxn().PropertyValue(object.Obj(), name)
		if code != types.E_NONE {
			t.Fatalf("%s error=%s", name, code)
		}
		return value
	}
	location, code := store.DirectTxn().Location(read("thing").Obj())
	if code != types.E_NONE {
		t.Fatalf("location error=%s", code)
	}
	return fmt.Sprintf("log=%s stack=%s moved=%t result=%s finished=%s",
		read("log"), read("stack"), location == read("dest").Obj(), read("result"), read("finished"))
}

func toastSuspendedMoveState(log, stack []string, moved bool, result string, finished int) string {
	list := func(entries []string) types.Value {
		values := make([]types.Value, len(entries))
		for index, entry := range entries {
			values[index] = types.NewStr(entry)
		}
		return types.NewList(values)
	}
	return fmt.Sprintf("log=%s stack=%s moved=%t result=%s finished=%d", list(log), list(stack), moved, result, finished)
}

// resume loads database's suspended task into a runtime over store and runs it
// to completion. The resumed hook must see move() between itself and start in
// callers(), the hooks after it must run, and move() must return 0 to start,
// which stores {0}.
func (fixture toastSuspendedMoveFixture) resume(t *testing.T, database *dbformat.Database, store *dbstore.Store) {
	t.Helper()
	if got, want := fixture.state(t, store), toastSuspendedMoveState(fixture.logged, nil, fixture.moved, "0", 0); got != want {
		t.Fatalf("checkpoint %s, want %s", got, want)
	}
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	rt.Session().LoadServerOptionsFromStore(store)
	rt.Session().LoadProtectedBuiltinsFromStore(store)
	saved := database.SuspendedTasks[0]
	saved.Snapshot.StartTime = time.Now().Add(-time.Second)
	if err := rt.LoadSuspendedTasks(database.SuspendedTasks); err != nil {
		t.Fatal(err)
	}
	restored := rt.GetTask(saved.Snapshot.ID)
	if restored == nil {
		t.Fatalf("native suspended task %d was not restored", saved.Snapshot.ID)
	}
	driveNativeImportedContinuation(t, rt, restored)
	if got, want := fixture.state(t, store), toastSuspendedMoveState(fixture.resumed, []string{"move", "start", ""}, true, "{0}", 1); got != want {
		t.Fatalf("resumed %s, want %s", got, want)
	}
}

func TestToastSuspendedMoveImportFinishesMove(t *testing.T) {
	for _, fixture := range toastSuspendedMoveFixtures {
		t.Run(fixture.hook, func(t *testing.T) {
			database := fixture.load(t)
			store, err := database.NewStoreFromDatabase()
			if err != nil {
				t.Fatal(err)
			}
			fixture.resume(t, database, store)
		})
	}
}

// Barn writes a task suspended in a move() hook as Toast does: importing each
// Toast checkpoint and writing it out again gives the hook frame Toast's pc
// line, builtin name and move data, and Barn finishes the move from that file.
func TestToastSuspendedMoveExportsToastContinuation(t *testing.T) {
	for _, fixture := range toastSuspendedMoveFixtures {
		t.Run(fixture.hook, func(t *testing.T) {
			toast := fixture.load(t).SuspendedTasks[0].Snapshot.VM
			database := fixture.load(t)
			store, err := database.NewStoreFromDatabase()
			if err != nil {
				t.Fatal(err)
			}
			rt := newTestRuntimeWithBuiltins(t, store)
			rt.Session().LoadServerOptionsFromStore(store)
			rt.Session().LoadProtectedBuiltinsFromStore(store)
			if err := rt.LoadSuspendedTasks(database.SuspendedTasks); err != nil {
				rt.Stop()
				t.Fatal(err)
			}
			_, suspended := rt.TaskSnapshots()
			rt.Stop()
			if len(suspended) != 1 || len(suspended[0].VM.Frames) != len(toast.Frames) {
				t.Fatalf("checkpoint holds %d tasks, want the imported one with %d frames", len(suspended), len(toast.Frames))
			}
			for index, frame := range suspended[0].VM.Frames {
				want := toast.Frames[index]
				if frame.Native == nil {
					t.Fatalf("frame %d was not written in Toast's form", index)
				}
				if frame.IP != want.IP || frame.BuiltinPC != want.BuiltinPC || frame.Native.ErrorPC != want.Native.ErrorPC ||
					!reflect.DeepEqual(frame.Native.Builtin, want.Native.Builtin) {
					t.Fatalf("frame %d pc/builtin pc/error pc/builtin = %d/%d/%d/%+v, want Toast's %d/%d/%d/%+v", index,
						frame.IP, frame.BuiltinPC, frame.Native.ErrorPC, frame.Native.Builtin,
						want.IP, want.BuiltinPC, want.Native.ErrorPC, want.Native.Builtin)
				}
				if len(frame.Stack) != len(want.Stack) {
					t.Fatalf("frame %d stack %v, want Toast's %v", index, frame.Stack, want.Stack)
				}
				// The file holds each operand's type code; the export marks only
				// Toast's control markers (7 catch, 8 finally), and these frames
				// hold none.
				for slot := range frame.Stack {
					if kind := want.Native.OperandTypes[slot]; !frame.Stack[slot].Equal(want.Stack[slot]) || kind == 7 || kind == 8 || frame.Native.OperandTypes[slot] != 0 {
						t.Fatalf("frame %d stack %v types %v, want Toast's %v types %v", index, frame.Stack, frame.Native.OperandTypes, want.Stack, want.Native.OperandTypes)
					}
				}
			}

			path := filepath.Join(t.TempDir(), "barn.db")
			if err := dbformat.WriteCheckpoint(path, store, nil, suspended, nil); err != nil {
				t.Fatal(err)
			}
			written, err := os.ReadFile(path + ".new")
			if err != nil {
				t.Fatal(err)
			}
			// The hook frame's last three lines, as in the Toast fixture.
			if tail := "\n" + fixture.pcLine + "\nmove\n" + toastSuspendedMoveData + "\n"; strings.Count(string(written), tail) != 1 {
				t.Fatalf("written checkpoint does not hold Toast's hook frame ending %q", tail)
			}
			reloaded, err := dbformat.LoadDatabase(path + ".new")
			if err != nil {
				t.Fatal(err)
			}
			if len(reloaded.SuspendedTasks) != 1 {
				t.Fatalf("reloaded suspended tasks = %d, want 1", len(reloaded.SuspendedTasks))
			}
			reloadedStore, err := reloaded.NewStoreFromDatabase()
			if err != nil {
				t.Fatal(err)
			}
			fixture.resume(t, reloaded, reloadedStore)
		})
	}
}

// Under call_function("move", ...) Toast records builtin call_function on the
// hook verb's activation, naming move, with move's data inside it. See
// testdata/toast-suspended-move.md for the producer. Barn finishes the move
// from that file as from a direct move()'s.
func TestToastSuspendedCallFunctionMoveImportFinishesMove(t *testing.T) {
	database := loadNativeImportFixture(t, "testdata/toast-suspended-call-function-move-accept.db", "363d56392a024d99522536a5dbf710426af450d0cc8f835b9f0050c2bdbff5d1")
	if len(database.SuspendedTasks) != 1 {
		t.Fatalf("native suspended tasks=%d, want 1", len(database.SuspendedTasks))
	}
	frames := database.SuspendedTasks[0].Snapshot.VM.Frames
	want := &task.NativeBuiltinSnapshot{Name: "call_function", State: "bf_call_function data: fname = move",
		Inner: &task.NativeBuiltinSnapshot{Name: "move", State: toastSuspendedMoveData}}
	if top := frames[len(frames)-1]; len(frames) != 3 || top.Verb != "accept" || top.Native == nil ||
		top.BuiltinPC != 2 || !reflect.DeepEqual(top.Native.Builtin, want) {
		t.Fatalf("fixture does not hold an accept frame with Toast's call_function move continuation at pc 2: %d frames", len(frames))
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	// stack holds the names callers() gave in accept before it suspended.
	state := func() string {
		object, _ := store.DirectTxn().PropertyValue(1, "conformance_suspended_call_function_move_accept")
		read := func(name string) types.Value {
			value, code := store.DirectTxn().PropertyValue(object.Obj(), name)
			if code != types.E_NONE {
				t.Fatalf("%s error=%s", name, code)
			}
			return value
		}
		location, _ := store.DirectTxn().Location(read("thing").Obj())
		return fmt.Sprintf("log=%s stack=%s moved=%t result=%s finished=%s",
			read("log"), read("stack"), location == read("dest").Obj(), read("result"), read("finished"))
	}
	stack := []string{"call_function", "start", ""}
	if got, want := state(), toastSuspendedMoveState([]string{"accept"}, stack, false, "0", 0); got != want {
		t.Fatalf("checkpoint %s, want %s", got, want)
	}
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	rt.Session().LoadServerOptionsFromStore(store)
	rt.Session().LoadProtectedBuiltinsFromStore(store)
	saved := database.SuspendedTasks[0]
	saved.Snapshot.StartTime = time.Now().Add(-time.Second)
	if err := rt.LoadSuspendedTasks(database.SuspendedTasks); err != nil {
		t.Fatal(err)
	}
	restored := rt.GetTask(saved.Snapshot.ID)
	if restored == nil {
		t.Fatalf("native suspended task %d was not restored", saved.Snapshot.ID)
	}
	driveNativeImportedContinuation(t, rt, restored)
	resumed := []string{"accept", "accept resumed", "exitfunc", "enterfunc"}
	if got, want := state(), toastSuspendedMoveState(resumed, stack, true, "{0}", 1); got != want {
		t.Fatalf("resumed %s, want %s", got, want)
	}
}

// The accept frame Toast wrote under call_function("move", ...) keeps its
// builtin's name in Barn: the loaded task's activation names it call_function,
// as Toast's callers() did before the checkpoint, and Barn's checkpoint of the
// loaded task writes Toast's call_function record again (execute.cc
// bf_call_function_write), so Toast resumed from Barn's file names it the same.
func TestToastSuspendedCallFunctionMoveKeepsFrameName(t *testing.T) {
	const fixture = "testdata/toast-suspended-call-function-move-accept.db"
	const hash = "363d56392a024d99522536a5dbf710426af450d0cc8f835b9f0050c2bdbff5d1"
	database := loadNativeImportFixture(t, fixture, hash)
	toast := loadNativeImportFixture(t, fixture, hash).SuspendedTasks[0].Snapshot.VM
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	rt.Session().LoadServerOptionsFromStore(store)
	rt.Session().LoadProtectedBuiltinsFromStore(store)
	if err := rt.LoadSuspendedTasks(database.SuspendedTasks); err != nil {
		t.Fatal(err)
	}
	restored := rt.GetTask(database.SuspendedTasks[0].Snapshot.ID)
	if restored == nil {
		t.Fatal("native suspended task was not restored")
	}
	stack := restored.GetCallStack()
	if top := stack[len(stack)-1]; top.Verb != "accept" || top.BuiltinCaller != "call_function" || top.BuiltinPC != 2 {
		t.Fatalf("loaded accept activation %s names builtin %q at pc %d, want call_function at 2", top.Verb, top.BuiltinCaller, top.BuiltinPC)
	}
	_, suspended := rt.TaskSnapshots()
	if len(suspended) != 1 || len(suspended[0].VM.Frames) != len(toast.Frames) {
		t.Fatalf("checkpoint holds %d tasks, want the loaded one with %d frames", len(suspended), len(toast.Frames))
	}
	top, want := suspended[0].VM.Frames[len(toast.Frames)-1], toast.Frames[len(toast.Frames)-1]
	if top.Native == nil || top.BuiltinPC != want.BuiltinPC || !reflect.DeepEqual(top.Native.Builtin, want.Native.Builtin) {
		t.Fatalf("Barn writes the accept frame as builtin pc %d %+v, want Toast's %d %+v", top.BuiltinPC, top.Native, want.BuiltinPC, want.Native.Builtin)
	}
	path := filepath.Join(t.TempDir(), "barn.db")
	if err := dbformat.WriteCheckpoint(path, store, nil, suspended, nil); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path + ".new")
	if err != nil {
		t.Fatal(err)
	}
	if tail := "\ncall_function\nbf_call_function data: fname = move\n" + toastSuspendedMoveData + "\n"; strings.Count(string(written), tail) != 1 {
		t.Fatalf("written checkpoint does not hold Toast's call_function accept frame ending %q", tail)
	}
}

// A task Barn started, suspended two verb calls above each move() hook, so the
// hook's frame is a middle frame: the checkpoint holds every frame in Toast's
// form with move's continuation on the hook frame, and the task finishes the
// move from the reloaded file.
func TestBarnMoveHookMiddleFrameCheckpointsAsToastContinuation(t *testing.T) {
	for stage, hook := range []string{"accept", "exitfunc", "enterfunc"} {
		t.Run(hook, func(t *testing.T) {
			verbs := map[string]string{
				"start": "r = move(#3, #5);\n#0.log = {@#0.log, r};",
				"deep":  "for x in ({1})\ntry\nr = #0:pause();\nfinally\n#0.n = #0.n + r;\nendtry\nendfor\nreturn r;",
				"pause": "suspend(100);\nreturn 1;",
			}
			store := newToastExportStore(t, verbs)
			for id := types.ObjID(3); id <= 5; id++ {
				addServerVerbTestObject(t, store, id, 0)
			}
			if code := store.DirectTxn().MoveObject(3, 4, 0); code != types.E_NONE {
				t.Fatalf("place #3 in #4: %s", code)
			}
			for name, object := range map[string]types.ObjID{"accept": 5, "exitfunc": 4, "enterfunc": 5} {
				source := "#0.log = {@#0.log, verb};\nreturn 1;"
				if name == hook {
					source = "#0.log = {@#0.log, verb};\nr = #0:deep();\n#0.log = {@#0.log, verb + \" resumed\"};\nreturn r;"
				}
				store.AddVerb(object, dbstore.NewVerb([]string{name}, 2,
					dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
					dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
					strings.Split(source, "\n")))
			}
			rt := newTestRuntimeWithBuiltins(t, store)
			suspended := pauseBarnTask(t, rt, "#0:start();")
			rt.Stop()
			// The frames are the program, start (paused at move()), the hook,
			// deep and pause.
			if len(suspended) != 1 || len(suspended[0].VM.Frames) != 5 {
				t.Fatalf("checkpoint holds %d tasks, want one of 5 frames", len(suspended))
			}
			for index, frame := range suspended[0].VM.Frames {
				if frame.Native == nil {
					t.Fatalf("frame %d was not written in Toast's form", index)
				}
				want := (*task.NativeBuiltinSnapshot)(nil)
				if index == 2 {
					want = &task.NativeBuiltinSnapshot{Name: "move", State: "bf_move data: what = 3, where = 5, position = 0"}
				}
				if !reflect.DeepEqual(frame.Native.Builtin, want) || (want != nil && (frame.Verb != hook || frame.BuiltinPC != stage+2)) {
					t.Fatalf("frame %d (%s) builtin pc %d state %+v, want %+v", index, frame.Verb, frame.BuiltinPC, frame.Native.Builtin, want)
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
			resumedStore, err := reloaded.NewStoreFromDatabase()
			if err != nil {
				t.Fatal(err)
			}
			resumed := newTestRuntimeWithBuiltins(t, resumedStore)
			defer resumed.Stop()
			saved := reloaded.SuspendedTasks[0]
			saved.Snapshot.StartTime = time.Now().Add(-time.Second)
			if err := resumed.LoadSuspendedTasks(reloaded.SuspendedTasks); err != nil {
				t.Fatal(err)
			}
			restored := resumed.GetTask(saved.Snapshot.ID)
			if restored == nil {
				t.Fatal("Toast-form task was not restored")
			}
			driveNativeImportedContinuation(t, resumed, restored)
			entries := []string{}
			for _, name := range []string{"accept", "exitfunc", "enterfunc"} {
				entries = append(entries, fmt.Sprintf("%q", name))
				if name == hook {
					entries = append(entries, fmt.Sprintf("%q", name+" resumed"))
				}
			}
			log, _ := resumedStore.DirectTxn().PropertyValue(0, "log")
			count, _ := resumedStore.DirectTxn().PropertyValue(0, "n")
			location, _ := resumedStore.DirectTxn().Location(3)
			if want := "{" + strings.Join(entries, ", ") + ", 0}"; log.String() != want || count.String() != "1" || location != 5 {
				t.Fatalf("#0.log = %s, #0.n = %s, #3 in #%d; want %s, 1, #5", log, count, location, want)
			}
		})
	}
}
