package format

import (
	"bufio"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/db/store"
)

// anonymousSection is a v17 anonymous-objects section holding one object
// saved under id.
func anonymousSection(id int64) string {
	return "1\n#" + strconv.FormatInt(id, 10) + "\nanon\n0\n0\n1\n-1\n10\n0\n4\n0\n1\n-1\n4\n0\n0\n0\n0\n0\n"
}

func TestReadAnonymousObjectsRejectsIdsWithoutIdentity(t *testing.T) {
	for _, id := range []int64{-92, -1, math.MaxInt64 - int64(store.AnonymousIdentityBase) + 1, math.MaxInt64} {
		t.Run(strconv.FormatInt(id, 10), func(t *testing.T) {
			db := &Database{Version: 17}
			err := db.readAnonymousObjects(bufio.NewReader(strings.NewReader(anonymousSection(id))))
			if err == nil || !strings.Contains(err.Error(), "invalid id") {
				t.Fatalf("readAnonymousObjects(#%d) = %v, want invalid id error", id, err)
			}
			if len(db.AnonymousObjs) != 0 {
				t.Fatalf("kept %d anonymous objects", len(db.AnonymousObjs))
			}
		})
	}
	for _, id := range []int64{0, 7, math.MaxInt64 - int64(store.AnonymousIdentityBase)} {
		t.Run("valid/"+strconv.FormatInt(id, 10), func(t *testing.T) {
			db := &Database{Version: 17}
			if err := db.readAnonymousObjects(bufio.NewReader(strings.NewReader(anonymousSection(id)))); err != nil {
				t.Fatal(err)
			}
			if len(db.AnonymousObjs) != 1 {
				t.Fatalf("got %d anonymous objects, want 1", len(db.AnonymousObjs))
			}
			if _, err := db.NewStoreFromDatabase(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
