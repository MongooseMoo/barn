package store

import (
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/sourcekey"
	"github.com/MongooseMoo/barn/types"
)

// A verb's CodeKey identifies its source content: it must always equal a fresh
// hash of the verb's current Code, through every path that writes verb source.
// A stale key would hand the compiler a cached program for the old source.

func verbViewForTest(t *testing.T, store *Store, objID types.ObjID, name string) VerbView {
	t.Helper()
	view, err := store.DirectTxn().FindVerbOnObject(objID, name)
	if err != nil {
		t.Fatalf("FindVerbOnObject(#%d, %q): %v", objID, name, err)
	}
	return view
}

func assertKeyMatchesCode(t *testing.T, view VerbView, context string) {
	t.Helper()
	if !view.CodeKey.IsSet() {
		t.Fatalf("%s: CodeKey is unset", context)
	}
	if want := sourcekey.Of(view.Code); view.CodeKey != want {
		t.Fatalf("%s: CodeKey %v does not match hash of Code %q", context, view.CodeKey, view.Code)
	}
}

func storeWithVerbForTest(t *testing.T, code []string) *Store {
	t.Helper()
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root: %v", err)
	}
	verb := NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, code)
	if _, errCode := store.AddVerb(0, verb); errCode != types.E_NONE {
		t.Fatalf("AddVerb: %v", errCode)
	}
	return store
}

func TestNewVerbCarriesItsSourceKey(t *testing.T) {
	store := storeWithVerbForTest(t, []string{"return 1;"})
	assertKeyMatchesCode(t, verbViewForTest(t, store, 0, "look"), "NewVerb")
}

func TestVerbSetCodeRefreshesKey(t *testing.T) {
	verb := NewVerb("look", []string{"look"}, 0, VerbRead, VerbArgs{}, nil)
	before := verb.View().CodeKey
	verb.SetCode([]string{"return 1;"})
	after := verb.View()
	assertKeyMatchesCode(t, after, "Verb.SetCode")
	if after.CodeKey == before {
		t.Fatalf("Verb.SetCode left the key unchanged")
	}
}

func TestObjectBuilderSetVerbCodeByIndexRefreshesKey(t *testing.T) {
	builder := NewObjectBuilder(1)
	builder.AppendVerb(NewVerb("look", []string{"look"}, 0, VerbRead, VerbArgs{}, nil))
	if !builder.SetVerbCodeByIndex(0, []string{"return 7;"}) {
		t.Fatalf("SetVerbCodeByIndex returned false")
	}
	store := NewStore()
	if err := store.Add(builder.Build()); err != nil {
		t.Fatalf("Add built object: %v", err)
	}
	view := verbViewForTest(t, store, 1, "look")
	assertKeyMatchesCode(t, view, "ObjectBuilder.SetVerbCodeByIndex")
	if len(view.Code) != 1 || view.Code[0] != "return 7;" {
		t.Fatalf("builder code = %q", view.Code)
	}
}

func TestStoreSetVerbCodeRefreshesKey(t *testing.T) {
	store := storeWithVerbForTest(t, []string{"return 1;"})
	before := verbViewForTest(t, store, 0, "look").CodeKey
	if errCode := store.DirectTxn().SetVerbCode(0, "look", []string{"return 2;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCode: %v", errCode)
	}
	after := verbViewForTest(t, store, 0, "look")
	assertKeyMatchesCode(t, after, "Store.SetVerbCode")
	if after.CodeKey == before {
		t.Fatalf("Store.SetVerbCode left the key unchanged")
	}
}

func TestStoreSetVerbCodeByIndexRefreshesKey(t *testing.T) {
	store := storeWithVerbForTest(t, []string{"return 1;"})
	before := verbViewForTest(t, store, 0, "look").CodeKey
	if errCode := store.DirectTxn().SetVerbCodeByIndex(0, 0, []string{"return 3;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCodeByIndex: %v", errCode)
	}
	after := verbViewForTest(t, store, 0, "look")
	assertKeyMatchesCode(t, after, "Store.SetVerbCodeByIndex")
	if after.CodeKey == before {
		t.Fatalf("Store.SetVerbCodeByIndex left the key unchanged")
	}
}

func TestTxnSetVerbCodeRefreshesKeyBeforeAndAfterCommit(t *testing.T) {
	store := storeWithVerbForTest(t, []string{"return 1;"})
	before := verbViewForTest(t, store, 0, "look").CodeKey

	tx := store.BeginReadOnly(0)
	defer tx.Release()
	if errCode := tx.SetVerbCode(0, "look", []string{"return 4;"}); errCode != types.E_NONE {
		t.Fatalf("txn SetVerbCode: %v", errCode)
	}
	staged, err := tx.FindVerbOnObject(0, "look")
	if err != nil {
		t.Fatalf("txn FindVerbOnObject: %v", err)
	}
	assertKeyMatchesCode(t, staged, "StoreTxn.SetVerbCode (staged)")
	if staged.CodeKey == before {
		t.Fatalf("staged verb write left the key unchanged")
	}
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit: %v", errCode)
	}

	committed := verbViewForTest(t, store, 0, "look")
	assertKeyMatchesCode(t, committed, "StoreTxn.SetVerbCode (committed)")
	if committed.CodeKey != staged.CodeKey {
		t.Fatalf("committed key %v != staged key %v", committed.CodeKey, staged.CodeKey)
	}
}

func TestTxnSetVerbCodeByIndexRefreshesKey(t *testing.T) {
	store := storeWithVerbForTest(t, []string{"return 1;"})
	tx := store.BeginReadOnly(0)
	defer tx.Release()
	if errCode := tx.SetVerbCodeByIndex(0, 0, []string{"return 5;"}); errCode != types.E_NONE {
		t.Fatalf("txn SetVerbCodeByIndex: %v", errCode)
	}
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit: %v", errCode)
	}
	assertKeyMatchesCode(t, verbViewForTest(t, store, 0, "look"), "StoreTxn.SetVerbCodeByIndex")
}

// A read-only txn that stages a verb write rebuilds its private object image
// from the live one (store_txn.go verb-write overlay). The rebuilt image must
// carry the staged source's key, not the live verb's.
func TestTxnImageRebuildCarriesStagedVerbKey(t *testing.T) {
	store := storeWithVerbForTest(t, []string{"return 1;"})
	tx := store.BeginReadOnly(0)
	defer tx.Release()
	if errCode := tx.SetVerbCode(0, "look", []string{"return 6;"}); errCode != types.E_NONE {
		t.Fatalf("txn SetVerbCode: %v", errCode)
	}
	// Force a rebuild of the txn's object image by reading a scalar that
	// re-derives it, then re-read the verb.
	if errCode := store.DirectTxn().SetObjectName(0, "renamed"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName: %v", errCode)
	}
	view, err := tx.FindVerbOnObject(0, "look")
	if err != nil {
		t.Fatalf("txn FindVerbOnObject: %v", err)
	}
	assertKeyMatchesCode(t, view, "txn image rebuild")
}

// Verb reads run concurrently with verb-code writes on the real hot path (every
// verb call takes a VerbView). The key must be published with the code it
// describes, never torn from a concurrent writer.
func TestConcurrentVerbReadsSeeConsistentCodeAndKey(t *testing.T) {
	store := storeWithVerbForTest(t, []string{"return 1;"})

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for reader := 0; reader < 8; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				tx := store.BeginReadOnly(0)
				view, _, err := tx.FindCallableVerb(0, "look")
				tx.Release()
				if err != nil {
					continue
				}
				if !view.CodeKey.IsSet() {
					t.Errorf("concurrent read saw an unset CodeKey")
					return
				}
				if want := sourcekey.Of(view.Code); view.CodeKey != want {
					t.Errorf("concurrent read saw key %v for code %q", view.CodeKey, view.Code)
					return
				}
			}
		}()
	}
	for i := 0; i < 200; i++ {
		lines := []string{"return " + string(rune('a'+i%26)) + ";"}
		if errCode := store.DirectTxn().SetVerbCode(0, "look", lines); errCode != types.E_NONE {
			t.Fatalf("SetVerbCode: %v", errCode)
		}
	}
	close(stop)
	wg.Wait()
}
