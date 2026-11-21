package engine

import (
	"reflect"
	"testing"
)

// The eval activation Toast wrote keeps its builtin's name in Barn: the loaded
// task's activation names it eval, or call_function under
// call_function("eval", ...), as Toast's callers() did before the checkpoint,
// and Barn's checkpoint of the loaded task writes Toast's record for it again,
// so Toast resumed from Barn's file names it the same.
func TestToastSuspendedEvalKeepsFrameName(t *testing.T) {
	for _, fixture := range []struct{ path, hash, name string }{
		{"testdata/toast-suspended-eval.db", "d2d4529ee4fbf988006881ceb142447b2c08cd69253a19e18be16e45fdd54419", "eval"},
		{"testdata/toast-suspended-call-function-eval.db", "ad1bd19d6162be3e0fb0de43700e8bc5c5e5c9d5f9d13189db4ed20b7e256259", "call_function"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			database := loadNativeImportFixture(t, fixture.path, fixture.hash)
			toast := loadNativeImportFixture(t, fixture.path, fixture.hash).SuspendedTasks[0].Snapshot.VM
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
			if top := stack[len(stack)-1]; !top.IsEvalFrame || top.BuiltinCaller != fixture.name || top.BuiltinPC != 2 {
				t.Fatalf("loaded eval activation names builtin %q at pc %d, want %s at 2", top.BuiltinCaller, top.BuiltinPC, fixture.name)
			}
			_, suspended := rt.TaskSnapshots()
			if len(suspended) != 1 || len(suspended[0].VM.Frames) != len(toast.Frames) {
				t.Fatalf("checkpoint holds %d tasks, want the loaded one with %d frames", len(suspended), len(toast.Frames))
			}
			top, want := suspended[0].VM.Frames[len(toast.Frames)-1], toast.Frames[len(toast.Frames)-1]
			if top.Native == nil || top.BuiltinPC != want.BuiltinPC || !reflect.DeepEqual(top.Native.Builtin, want.Native.Builtin) {
				t.Fatalf("Barn writes the eval activation as builtin pc %d %+v, want Toast's %d %+v", top.BuiltinPC, top.Native, want.BuiltinPC, want.Native.Builtin)
			}
		})
	}
}
