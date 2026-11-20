package engine

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// Toast evaluates a range loop's lower bound, then its upper bound, then runs
// OP_FOR_RANGE (code_gen.cc STMT_RANGE), so a task paused in a verb called
// from the upper bound holds the lower bound as an ordinary operand. See
// testdata/toast-suspended-range-bound.md for the producer.
func TestToastSuspendedRangeBoundImportRunsTheLoop(t *testing.T) {
	database := loadNativeImportFixture(t, "testdata/toast-suspended-range-bound.db", "8454828352c2833042c79789516d9e0f8ca32c26e07cebef40a534991bc4baff")
	if len(database.SuspendedTasks) != 2 {
		t.Fatalf("native suspended tasks=%d, want 2", len(database.SuspendedTasks))
	}
	// Each task is the fork body, the looping verb paused at its this:limit()
	// call, and limit paused in suspend(). The looping verbs hold what Toast
	// had evaluated before the call: flat its lower bound 2; nested the outer
	// loop's next value 2 and limit 2, then the inner lower bound 10.
	wantOperands := map[string]string{"flat": "{2}", "nested": "{2, 2, 10}"}
	for _, saved := range database.SuspendedTasks {
		frames := saved.Snapshot.VM.Frames
		if len(frames) != 3 || frames[1].Native == nil {
			t.Fatalf("task %d does not hold three Toast frames", saved.Snapshot.ID)
		}
		want, known := wantOperands[frames[1].Verb]
		if got := types.NewList(frames[1].Stack).String(); !known || got != want {
			t.Fatalf("fixture frame %s holds operands %s, want %s", frames[1].Verb, got, want)
		}
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	object, code := store.DirectTxn().PropertyValue(1, "conformance_suspended_range_bound_checkpoint")
	if code != types.E_NONE || object.Type() != types.TYPE_OBJ {
		t.Fatalf("native fixture object=%s error=%s", object, code)
	}
	state := func() string {
		t.Helper()
		var values []types.Value
		for _, name := range []string{"flat_paused", "flat_resumed", "seen", "flat_finished", "nested_paused", "nested_resumed", "pairs", "nested_finished"} {
			value, code := store.DirectTxn().PropertyValue(object.Obj(), name)
			if code != types.E_NONE {
				t.Fatalf("%s error=%s", name, code)
			}
			values = append(values, value)
		}
		return types.NewList(values).String()
	}
	if got, want := state(), "{1, 0, {}, 0, 1, 0, {}, 0}"; got != want {
		t.Fatalf("checkpoint state=%s, want %s", got, want)
	}
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	rt.Session().LoadServerOptionsFromStore(store)
	rt.Session().LoadProtectedBuiltinsFromStore(store)
	for _, saved := range database.SuspendedTasks {
		saved.Snapshot.StartTime = time.Now().Add(-time.Second)
	}
	if err := rt.LoadSuspendedTasks(database.SuspendedTasks); err != nil {
		t.Fatal(err)
	}
	// Barn writes the imported tasks back as Toast wrote them: every frame at
	// Toast's pc with Toast's operands.
	_, exported := rt.TaskSnapshots()
	if len(exported) != len(database.SuspendedTasks) {
		t.Fatalf("exported %d tasks, want %d", len(exported), len(database.SuspendedTasks))
	}
	for _, saved := range database.SuspendedTasks {
		for _, again := range exported {
			if again.ID != saved.Snapshot.ID {
				continue
			}
			if again.VM.NativeRootVector != saved.Snapshot.VM.NativeRootVector || len(again.VM.Frames) != len(saved.Snapshot.VM.Frames) {
				t.Fatalf("task %d exported vector %d with %d frames", again.ID, again.VM.NativeRootVector, len(again.VM.Frames))
			}
			for index, frame := range again.VM.Frames {
				want := saved.Snapshot.VM.Frames[index]
				if frame.Native == nil {
					t.Fatalf("task %d frame %d (%s) was not exported in Toast's form", again.ID, index, want.Verb)
				}
				if got, wanted := types.NewList(frame.Stack).String(), types.NewList(want.Stack).String(); frame.IP != want.IP || frame.Native.ErrorPC != want.Native.ErrorPC || got != wanted {
					t.Fatalf("task %d frame %d (%s) exported pc/error pc/operands %d/%d/%s, Toast wrote %d/%d/%s", again.ID, index, want.Verb, frame.IP, frame.Native.ErrorPC, got, want.IP, want.Native.ErrorPC, wanted)
				}
			}
		}
	}
	for _, saved := range database.SuspendedTasks {
		restored := rt.GetTask(saved.Snapshot.ID)
		if restored == nil {
			t.Fatalf("native suspended task %d was not restored", saved.Snapshot.ID)
		}
		driveNativeImportedContinuation(t, rt, restored)
	}
	if got, want := state(), "{1, 1, {2, 3, 4}, 1, 1, 1, {{1, 10}, {1, 11}, {2, 20}, {2, 21}}, 1}"; got != want {
		t.Fatalf("resumed state=%s, want %s", got, want)
	}
}
