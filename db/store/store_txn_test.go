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
