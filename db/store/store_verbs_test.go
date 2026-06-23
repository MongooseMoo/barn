package store

import (
	"testing"

	"barn/types"
)

func TestVerbMutationsStampVerbVersion(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if _, errCode := store.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	added := verbVersionForTest(t, store, 0, "look")
	if added == 0 {
		t.Fatalf("added verb version = 0, want stamped version")
	}
	if errCode := store.SetVerbCode(0, "look", []string{"return 1;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCode failed: %v", errCode)
	}
	code := verbVersionForTest(t, store, 0, "look")
	if code <= added {
		t.Fatalf("version after SetVerbCode = %d, want > %d", code, added)
	}
	if errCode := store.SetVerbArgs(0, "look", VerbArgs{This: "this", Prep: "none", That: "none"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbArgs failed: %v", errCode)
	}
	args := verbVersionForTest(t, store, 0, "look")
	if args <= code {
		t.Fatalf("version after SetVerbArgs = %d, want > %d", args, code)
	}
	if errCode := store.SetVerbInfo(0, "look", 0, VerbRead|VerbExecute, []string{"inspect"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbInfo failed: %v", errCode)
	}
	info := verbVersionForTest(t, store, 0, "inspect")
	if info <= args {
		t.Fatalf("version after SetVerbInfo = %d, want > %d", info, args)
	}
	if errCode := store.SetVerbCodeByIndex(0, 0, []string{"return 2;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCodeByIndex failed: %v", errCode)
	}
	byIndex := verbVersionForTest(t, store, 0, "inspect")
	if byIndex <= info {
		t.Fatalf("version after SetVerbCodeByIndex = %d, want > %d", byIndex, info)
	}
}

func verbVersionForTest(t *testing.T, store *Store, objID types.ObjID, name string) uint64 {
	t.Helper()

	store.mu.RLock()
	defer store.mu.RUnlock()

	verb, _, err := store.findVerbLocked(objID, name)
	if err != nil {
		t.Fatalf("findVerbLocked(%q) failed: %v", name, err)
	}
	return verb.version
}
