package format

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
)

// FuzzReadDatabase exercises the raw database bytes read at server startup:
// LoadDatabase -> loadDatabase -> parseDatabaseWithWaifIdentities (reader.go).
// No recovery hides panics. Go's fuzz worker timeout detects stalled inputs;
// count allocations must stay incremental in the production reader.
func FuzzReadDatabase(f *testing.F) {
	for _, path := range []string{
		"../../Test_fresh2.db", // Small real v4 database.
		"testdata/Broken1.db", "testdata/Broken2.db", "testdata/Broken3.db",
		"testdata/Broken4.db", "testdata/Broken5.db", // Toast v5 repair fixtures.
		"testdata/Anon6.db", // Small real v17 database, including finalization.
	} {
		seed, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(seed)
	}
	// Exercise every v17 section count at its boundary, including truncated
	// huge counts, without requiring mutations to discover the decimal spelling.
	var empty bytes.Buffer
	if err := NewWriter(&empty, store.NewStore().Snapshot()).WriteDatabase(); err != nil {
		f.Fatal(err)
	}
	f.Add(bytes.Clone(empty.Bytes()))
	lines := strings.SplitAfter(empty.String(), "\n")
	for i := 1; i < len(lines)-1; i++ {
		for _, count := range []int{-1, math.MaxInt} {
			mutated := append([]string(nil), lines...)
			mutated[i] = strconv.Itoa(count) + strings.TrimPrefix(lines[i], "0")
			f.Add([]byte(strings.Join(mutated, "")))
		}
	}
	// Include a complete saved VM frame so mutations reach the restored
	// activation decoder, not only the empty task section headers.
	var suspended bytes.Buffer
	writer := NewWriter(&suspended, store.NewStore().Snapshot())
	writer.SetTaskSnapshots(nil, []task.Snapshot{{
		ID: 1, StartTime: time.Unix(1, 0), State: task.TaskSuspended,
		VM: &task.VMSnapshot{MaxStackDepth: 50, Frames: []task.VMFrameSnapshot{{
			Program: bytecode.Program{Source: []string{"return 1;"}},
		}}},
	}})
	if err := writer.WriteDatabase(); err != nil {
		f.Fatal(err)
	}
	seed, err := parseDatabaseWithWaifIdentities(bufio.NewReader(bytes.NewReader(suspended.Bytes())), nil)
	if err != nil || len(seed.SuspendedTasks) != 1 {
		f.Fatalf("suspended seed: %v", err)
	}
	f.Add(bytes.Clone(suspended.Bytes()))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, raw []byte) {
		database, err := parseDatabaseWithWaifIdentities(bufio.NewReader(bytes.NewReader(raw)), nil)
		if err != nil {
			return
		}
		original, err := database.NewStoreFromDatabase()
		if err != nil {
			t.Fatalf("accepted database cannot construct store: %v", err)
		}
		// The first dump normalizes each object's property slot count to its
		// ancestry (reader_helpers.go slotNames, writer_object.go
		// writeProperties), where Toast keeps the file's raw count. So the
		// invariant is idempotence: a second round trip matches the first.
		first, err := dumpAndReload(original)
		if err != nil {
			t.Fatal(err)
		}
		second, err := dumpAndReload(first)
		if err != nil {
			t.Fatal(err)
		}
		if err := CompareRoundTripStores(first, second); err != nil {
			t.Fatal(err)
		}
	})
}

// dumpAndReload writes s as a database and loads the result into a new store.
func dumpAndReload(s *store.Store) (*store.Store, error) {
	var dump bytes.Buffer
	if err := NewWriter(&dump, s.Snapshot()).WriteDatabase(); err != nil {
		return nil, fmt.Errorf("write database: %w", err)
	}
	reloaded, err := parseDatabaseWithWaifIdentities(bufio.NewReader(&dump), nil)
	if err != nil {
		return nil, fmt.Errorf("read written database: %w", err)
	}
	restored, err := reloaded.NewStoreFromDatabase()
	if err != nil {
		return nil, fmt.Errorf("construct reloaded store: %w", err)
	}
	return restored, nil
}
