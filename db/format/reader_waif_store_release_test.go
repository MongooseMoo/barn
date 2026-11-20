package format

import (
	"bufio"
	"bytes"
	"runtime"
	"testing"
	"weak"

	"github.com/MongooseMoo/barn/db/store"
)

// A loaded store that holds one of its registered WAIFs must still be
// collectable once the caller drops it. FuzzReadDatabase builds several stores
// per input; when each one was pinned forever, the fuzz worker's live heap grew
// to the guest's OOM limit (issue #717).
func TestLoadedStoreHoldingWaifIsCollectable(t *testing.T) {
	raw := taskRoundTripRaw(t) // #0.shared holds a WAIF.
	released := loadStoreWeakly(t, raw)
	for i := 0; i < 10 && released.Value() != nil; i++ {
		runtime.GC()
	}
	if released.Value() != nil {
		t.Fatal("store built by NewStoreFromDatabase is still reachable after its last reference was dropped")
	}
}

func loadStoreWeakly(t *testing.T, raw []byte) weak.Pointer[store.Store] {
	t.Helper()
	database, err := parseDatabaseWithWaifIdentities(bufio.NewReader(bytes.NewReader(raw)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(database.savedWaifs) == 0 {
		t.Fatal("input must load a WAIF")
	}
	s, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	return weak.Make(s)
}
