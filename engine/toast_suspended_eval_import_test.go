package engine

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// Toast records eval()'s return continuation (builtin eval, pc 2) on the
// activation eval() pushed, not on its caller. See
// testdata/toast-suspended-eval.md for the producer.
func TestToastSuspendedEvalImportReturnsEvalResult(t *testing.T) {
	database := loadNativeImportFixture(t, "testdata/toast-suspended-eval.db", "d2d4529ee4fbf988006881ceb142447b2c08cd69253a19e18be16e45fdd54419")
	if len(database.SuspendedTasks) != 1 {
		t.Fatalf("native suspended tasks=%d, want 1", len(database.SuspendedTasks))
	}
	saved := database.SuspendedTasks[0]
	// The frames are the fork body, start (paused at its eval() call) and the
	// eval activation, paused in suspend().
	frames := saved.Snapshot.VM.Frames
	top := frames[len(frames)-1]
	if len(frames) != 3 || top.Native == nil || top.Native.Builtin == nil || top.Native.Builtin.Name != "eval" || top.BuiltinPC != 2 {
		t.Fatalf("fixture does not hold an eval frame with Toast's eval continuation: %d frames", len(frames))
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	object, code := store.DirectTxn().PropertyValue(1, "conformance_suspended_eval_checkpoint")
	if code != types.E_NONE || object.Type() != types.TYPE_OBJ {
		t.Fatalf("native fixture object=%s error=%s", object, code)
	}
	read := func(name string) types.Value {
		t.Helper()
		value, code := store.DirectTxn().PropertyValue(object.Obj(), name)
		if code != types.E_NONE {
			t.Fatalf("%s error=%s", name, code)
		}
		return value
	}
	zero, one := types.NewInt(0), types.NewInt(1)
	if !read("before").Equal(one) || !read("after").Equal(zero) || !read("result").Equal(zero) || !read("finished").Equal(zero) {
		t.Fatalf("checkpoint before/after/result/finished=%s/%s/%s/%s, want 1/0/0/0", read("before"), read("after"), read("result"), read("finished"))
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
	driveNativeImportedContinuation(t, rt, restored)
	want := types.NewList([]types.Value{one, types.NewInt(73)})
	if !read("before").Equal(one) || !read("after").Equal(one) || !read("result").Equal(want) || !read("finished").Equal(one) {
		t.Fatalf("resumed before/after/result/finished=%s/%s/%s/%s, want 1/1/{1, 73}/1", read("before"), read("after"), read("result"), read("finished"))
	}
}
