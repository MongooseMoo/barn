package format

import (
	"os"
	"testing"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/sourcekey"
	"github.com/MongooseMoo/barn/types"
)

// A verb read back through the loader must arrive with its content key already
// computed: the loader fills verb source in a second pass (ObjectBuilder.
// SetVerbCodeByIndex / Verb.SetCode), and a verb that reached the store without
// a key would make every call to it rehash its whole source.
func TestLoadedVerbsCarryTheirSourceKey(t *testing.T) {
	s := store.NewStore()
	objID, errCode := s.CreateObject(nil, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject: %v", errCode)
	}
	mkVerb := func(name string) store.Verb {
		return store.NewVerb(name, []string{name}, 0, store.VerbPerms(0),
			store.VerbArgs{This: "none", Prep: "none", That: "none"}, nil)
	}
	if _, errCode = s.AddVerb(objID, mkVerb("first")); errCode != types.E_NONE {
		t.Fatalf("AddVerb(first): %v", errCode)
	}
	if _, errCode = s.AddVerb(objID, mkVerb("second")); errCode != types.E_NONE {
		t.Fatalf("AddVerb(second): %v", errCode)
	}
	if errCode = s.SetVerbCodeByIndex(objID, 0, []string{"return 1;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCodeByIndex(first): %v", errCode)
	}
	if errCode = s.SetVerbCodeByIndex(objID, 1, []string{"return 2;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCodeByIndex(second): %v", errCode)
	}

	tmpFile, err := os.CreateTemp(t.TempDir(), "verb-codekey-*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer tmpFile.Close()
	if err := NewWriter(tmpFile, s.Snapshot()).WriteDatabase(); err != nil {
		t.Fatalf("WriteDatabase: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	loaded, err := LoadDatabase(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadDatabase: %v", err)
	}
	loadedStore, err := loaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatalf("construct store: %v", err)
	}
	obj := loadedStore.Snapshot().Objects[objID]
	if obj == nil {
		t.Fatalf("reloaded object #%d missing", objID)
	}
	if len(obj.VerbList) != 2 {
		t.Fatalf("reloaded verb count = %d, want 2", len(obj.VerbList))
	}
	for _, view := range obj.VerbList {
		if !view.CodeKey.IsSet() {
			t.Fatalf("reloaded verb %q has an unset CodeKey", view.Name)
		}
		if want := sourcekey.Of(view.Code); view.CodeKey != want {
			t.Fatalf("reloaded verb %q: CodeKey does not match its Code %q", view.Name, view.Code)
		}
	}
	if obj.VerbList[0].CodeKey == obj.VerbList[1].CodeKey {
		t.Fatalf("verbs with different source share a CodeKey")
	}
}
