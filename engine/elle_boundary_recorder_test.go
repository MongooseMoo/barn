package engine

import (
	"path/filepath"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/types"
)

func TestElleRecorderSeparatesEffectSuspendForkAndExceptionPublications(t *testing.T) {
	store := newBoundaryTestStore(t)
	recorder := newElleRecorder()
	var observe builtins.BuiltinFunc = recorder.observe
	var effect builtins.BuiltinFunc = func(*builtins.Execution, []types.Value) types.Result { return types.Ok(types.None) }
	slot := testBuiltinSlot("elle_effect", 0, 0, nil, &effect)
	slot.Effect = builtins.Irreversible
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 2,
		testBuiltinSlot("elle_observe", 3, 3, []int64{2, 2, -1}, &observe), slot)
	defer rt.Stop()
	const id = 416
	elleExpectedExceptions[id] = true
	t.Cleanup(func() { delete(elleExpectedExceptions, id) })
	tk := elleTask(t, rt, id, `
#0.write_value=7; elle_observe("w","x",7); elle_effect();
#0.write_value=9; elle_observe("w","x",9); suspend(0);
#0.write_value=11; elle_observe("w","x",11);
fork (0) #0.write_value=13; elle_observe("w","x",13); endfork
raise(E_DIV);`)
	elleDrain(t, rt, []*Task{tk})
	counts, err := recorder.write(filepath.Join(t.TempDir(), "history.edn"))
	if err != nil {
		t.Fatal(err)
	}
	if counts["ok"] != 4 || counts["w/x"] != 4 || counts["fail"] != 0 || counts["info"] != 0 || counts["discarded_attempts"] != 0 {
		t.Fatalf("publication outcomes=%v", counts)
	}
	want := map[int64]bool{7: false, 9: false, 11: false, 13: false}
	for _, tx := range recorder.txns {
		if !tx.committed || len(tx.ops) != 1 {
			t.Fatalf("publication combines boundaries or is uncommitted: %+v", tx)
		}
		value := tx.ops[0][2].(int64)
		if seen, ok := want[value]; !ok || seen {
			t.Fatalf("unexpected or duplicate publication: %v", tx.ops)
		}
		want[value] = true
		if tx.start >= tx.end {
			t.Fatalf("publication completion precedes invocation: %+v", tx)
		}
	}
}

func TestElleRecorderRetryAfterSuspendPreservesEarlierPublication(t *testing.T) {
	store := newBoundaryTestStore(t)
	recorder := newElleRecorder()
	var observe builtins.BuiltinFunc = recorder.observe
	bump, calls := bumpReadValueLiveOnce(t, store)
	var effect builtins.BuiltinFunc = func(*builtins.Execution, []types.Value) types.Result { return types.Ok(types.None) }
	slot := testBuiltinSlot("elle_effect", 0, 0, nil, &effect)
	slot.Effect = builtins.Irreversible
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 2,
		testBuiltinSlot("elle_observe", 3, 3, []int64{2, 2, -1}, &observe), bump, slot)
	defer rt.Stop()
	tk := elleTask(t, rt, 417, `#0.write_value=7; elle_observe("w","x",7); suspend(0); v=#0.read_value; elle_observe("r","y",v); bump_read_value_live_once(); #0.write_value=9; elle_observe("w","x",9); elle_effect(); #0.write_value=11; elle_observe("w","x",11);`)
	elleDrain(t, rt, []*Task{tk})
	counts, err := recorder.write(filepath.Join(t.TempDir(), "history.edn"))
	if err != nil {
		t.Fatal(err)
	}
	if *calls != 2 || counts["discarded_attempts"] != 1 || counts["ok"] != 3 || counts["r/y"] != 1 || counts["w/x"] != 3 || counts["fail"] != 0 || counts["info"] != 0 {
		t.Fatalf("calls=%d publication outcomes=%v", *calls, counts)
	}
	for _, tx := range recorder.txns {
		if tx.committed && len(tx.ops) > 0 && tx.ops[0][0] == "r" && tx.ops[0][2] != int64(1) {
			t.Fatalf("discarded snapshot survived in committed history: %v", tx.ops)
		}
	}
}

func TestElleRecorderFailureAfterIrreversiblePublicationKeepsBothOutcomes(t *testing.T) {
	if !isolationprobe.Enabled {
		t.Skip("commit failure outcome hook requires -tags elle")
	}
	store := newBoundaryTestStore(t)
	for _, id := range []types.ObjID{1, 2} {
		if err := store.Add(dbstore.NewObject(id, 0)); err != nil {
			t.Fatal(err)
		}
	}
	recorder := newElleRecorder()
	var observe builtins.BuiltinFunc = recorder.observe
	var effect builtins.BuiltinFunc = func(*builtins.Execution, []types.Value) types.Result { return types.Ok(types.None) }
	slot := testBuiltinSlot("elle_effect", 0, 0, nil, &effect)
	slot.Effect = builtins.Irreversible
	var sabotage builtins.BuiltinFunc = func(ctx *builtins.Execution, _ []types.Value) types.Result {
		if ec := ctx.StoreTxn.MoveObject(1, 2, 0); ec != types.E_NONE {
			return types.Err(ec)
		}
		if err := store.Recycle(2); err != nil {
			t.Error(err)
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.None)
	}
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 2,
		testBuiltinSlot("elle_observe", 3, 3, []int64{2, 2, -1}, &observe), slot,
		testBuiltinSlot("elle_sabotage", 0, 0, nil, &sabotage))
	defer rt.Stop()
	tk := elleTask(t, rt, 418, `#0.write_value=7; elle_observe("w","x",7); elle_effect(); #0.write_value=9; elle_observe("w","x",9); elle_sabotage();`)
	rt.QueueTask(tk)
	rt.ProcessReadyTasks()
	counts, err := recorder.write(filepath.Join(t.TempDir(), "history.edn"))
	if err != nil {
		t.Fatal(err)
	}
	if tk.Result.Error != types.E_INVIND || counts["ok"] != 1 || counts["fail"] != 1 || counts["info"] != 0 || counts["w/x"] != 1 || counts["discarded_attempts"] != 0 {
		t.Fatalf("result=%+v publication outcomes=%v", tk.Result, counts)
	}
	live, ec := store.DirectTxn().PropertyValue(0, "write_value")
	if ec != types.E_NONE || live.Int() != 7 {
		t.Fatalf("earlier publication lost or failed write leaked: value=%v error=%v", live, ec)
	}
}
