package store

import (
	"testing"

	"barn/types"
)

func TestReadOnlyTransactionSeesStableSnapshot(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.SetObjectName(0, "before"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName before failed: %v", errCode)
	}
	if errCode := store.DefineProperty(0, NewProperty("scratch", types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	if _, errCode := store.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 1;"})); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}
	child, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject child failed: %v", errCode)
	}

	readTS := store.ReadTimestamp()
	tx := store.BeginReadOnly(0)
	if tx.ReadTimestamp() != readTS {
		t.Fatalf("txn timestamp = %d, want %d", tx.ReadTimestamp(), readTS)
	}

	if errCode := store.SetObjectName(0, "after"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName after failed: %v", errCode)
	}
	if errCode := store.SetPropertyValue(0, "scratch", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue after failed: %v", errCode)
	}
	if errCode := store.SetVerbCode(0, "look", []string{"return 2;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCode after failed: %v", errCode)
	}
	if _, errCode := store.CreateObject([]types.ObjID{0}, 0, false); errCode != types.E_NONE {
		t.Fatalf("CreateObject second child failed: %v", errCode)
	}

	if got, errCode := tx.ObjectName(0); errCode != types.E_NONE || got != "before" {
		t.Fatalf("txn ObjectName = %q err=%v, want before", got, errCode)
	}
	prop, errCode := tx.FindProperty(0, "scratch")
	if errCode != types.E_NONE {
		t.Fatalf("txn FindProperty failed: %v", errCode)
	}
	if got := prop.Value.(types.IntValue).Val; got != 1 {
		t.Fatalf("txn property value = %d, want 1", got)
	}
	verb, _, err := tx.FindVerb(0, "look")
	if err != nil {
		t.Fatalf("txn FindVerb failed: %v", err)
	}
	if len(verb.Code) != 1 || verb.Code[0] != "return 1;" {
		t.Fatalf("txn verb code = %#v, want original code", verb.Code)
	}
	children, errCode := tx.Children(0)
	if errCode != types.E_NONE {
		t.Fatalf("txn Children failed: %v", errCode)
	}
	if len(children) != 1 || children[0] != child {
		t.Fatalf("txn children = %#v, want only child #%d", children, child)
	}

	if got, errCode := store.ObjectName(0); errCode != types.E_NONE || got != "after" {
		t.Fatalf("live ObjectName = %q err=%v, want after", got, errCode)
	}
	if nextTS := store.ReadTimestamp(); nextTS <= readTS {
		t.Fatalf("store timestamp after writes = %d, want > %d", nextTS, readTS)
	}
}

func TestReadOnlyTransactionClonesReturnedContainers(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	child, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject child failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	children, errCode := tx.Children(0)
	if errCode != types.E_NONE {
		t.Fatalf("txn Children failed: %v", errCode)
	}
	children[0] = 99

	again, errCode := tx.Children(0)
	if errCode != types.E_NONE {
		t.Fatalf("txn Children second read failed: %v", errCode)
	}
	if len(again) != 1 || again[0] != child {
		t.Fatalf("mutating returned children changed txn snapshot: %#v", again)
	}
}

func TestReadOnlyTransactionLoadsObjectsLazily(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if _, errCode := store.CreateObject([]types.ObjID{0}, 0, false); errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if got := len(tx.objects); got != 0 {
		t.Fatalf("BeginReadOnly cached %d objects, want 0", got)
	}
	if _, errCode := tx.ObjectName(0); errCode != types.E_NONE {
		t.Fatalf("ObjectName failed: %v", errCode)
	}
	if got := len(tx.objects); got != 1 {
		t.Fatalf("after one object read cached %d objects, want 1", got)
	}
	if _, errCode := tx.Children(0); errCode != types.E_NONE {
		t.Fatalf("Children failed: %v", errCode)
	}
	if got := len(tx.objects); got != 1 {
		t.Fatalf("relationship read cached %d objects, want still 1", got)
	}
}

func TestTransactionDisjointPropertyWritesBothCommit(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, NewProperty("a", types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty a failed: %v", errCode)
	}
	if errCode := store.DefineProperty(0, NewProperty("b", types.NewInt(10), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty b failed: %v", errCode)
	}

	txA := store.BeginReadOnly(0)
	txB := store.BeginReadOnly(0)
	if errCode := txA.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("txA SetPropertyValue failed: %v", errCode)
	}
	if errCode := txB.SetPropertyValue(0, "b", types.NewInt(20)); errCode != types.E_NONE {
		t.Fatalf("txB SetPropertyValue failed: %v", errCode)
	}

	if errCode := txA.Commit(); errCode != types.E_NONE {
		t.Fatalf("txA Commit failed: %v", errCode)
	}
	if errCode := txB.Commit(); errCode != types.E_NONE {
		t.Fatalf("txB Commit failed: %v", errCode)
	}

	a, errCode := store.PropertyValue(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue a failed: %v", errCode)
	}
	if got := a.(types.IntValue).Val; got != 2 {
		t.Fatalf("a = %d, want 2", got)
	}
	b, errCode := store.PropertyValue(0, "b")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue b failed: %v", errCode)
	}
	if got := b.(types.IntValue).Val; got != 20 {
		t.Fatalf("b = %d, want 20", got)
	}
}

func TestTransactionSamePropertyWriteConflicts(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, NewProperty("a", types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	first := store.BeginReadOnly(0)
	second := store.BeginReadOnly(0)
	if errCode := first.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("first SetPropertyValue failed: %v", errCode)
	}
	if errCode := second.SetPropertyValue(0, "a", types.NewInt(3)); errCode != types.E_NONE {
		t.Fatalf("second SetPropertyValue failed: %v", errCode)
	}

	if errCode := first.Commit(); errCode != types.E_NONE {
		t.Fatalf("first Commit failed: %v", errCode)
	}
	if errCode := second.Commit(); errCode != types.E_INVARG {
		t.Fatalf("second Commit = %v, want E_INVARG conflict", errCode)
	}

	a, errCode := store.PropertyValue(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue failed: %v", errCode)
	}
	if got := a.(types.IntValue).Val; got != 2 {
		t.Fatalf("a = %d, want first writer value 2", got)
	}
}

func TestTransactionCommitPreservesHistoricalReads(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, NewProperty("a", types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	reader := store.BeginReadOnly(0)
	writer := store.BeginReadOnly(0)
	if errCode := writer.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("writer SetPropertyValue failed: %v", errCode)
	}
	if errCode := writer.Commit(); errCode != types.E_NONE {
		t.Fatalf("writer Commit failed: %v", errCode)
	}

	prop, errCode := reader.FindProperty(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("reader FindProperty failed: %v", errCode)
	}
	if got := prop.Value.(types.IntValue).Val; got != 1 {
		t.Fatalf("reader property value after commit = %d, want historical value 1", got)
	}

	live, errCode := store.PropertyValue(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("live PropertyValue failed: %v", errCode)
	}
	if got := live.(types.IntValue).Val; got != 2 {
		t.Fatalf("live property value after commit = %d, want 2", got)
	}
}
