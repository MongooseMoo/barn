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

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestReadDatabaseRejectsNegativeSectionCounts(t *testing.T) {
	raw, err := os.ReadFile("testdata/toastcore.db")
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range []struct{ before, after, errorText string }{
		{"**\n6\n", "**\n-1\n", "invalid player count"},
		{"0 values pending finalization", "-1 values pending finalization", "invalid pending finalization count"},
		{"4 queued tasks", "-1 queued tasks", "invalid queued task count"},
		{"0 suspended tasks", "-1 suspended tasks", "invalid suspended task count"},
		{"0 interrupted tasks", "-1 interrupted tasks", "invalid interrupted task count"},
		{"0 active connections", "-1 active connections", "invalid active connection count"},
	} {
		t.Run(section.errorText, func(t *testing.T) {
			if !bytes.Contains(raw, []byte(section.before)) {
				t.Fatal("fixture mutation anchor missing")
			}
			input := bytes.Replace(raw, []byte(section.before), []byte(section.after), 1)
			_, err := parseDatabaseWithWaifIdentities(bufio.NewReader(bytes.NewReader(input)), nil)
			if err == nil || !strings.Contains(err.Error(), section.errorText) {
				t.Fatalf("mutated fixture: %v, want %s", err, section.errorText)
			}
			t.Log(err)
		})
	}
	for _, path := range []string{"testdata/toastcore.db", "../../Test_fresh2.db", "testdata/Broken1.db"} {
		if _, err := LoadDatabaseReadOnly(path); err != nil {
			t.Fatalf("unchanged fixture %s: %v", path, err)
		}
	}
}

func TestReadRecordCountsRejectNegativeAndTruncatedHugeCounts(t *testing.T) {
	db := &Database{Version: 17}
	for _, tc := range []struct {
		name, suffix string
		read         func(*bufio.Reader) error
	}{
		{"players-v4", "\n", db.readPlayersV4},
		{"players-v17", "\n", db.readPlayersV17},
		{"finalizations", " values pending finalization\n", db.readFinalizations},
		{"queued", " queued tasks\n", db.readQueuedTasks},
		{"suspended", " suspended tasks\n", db.readSuspendedTasks},
		{"interrupted", " interrupted tasks\n", db.readInterruptedTasks},
		{"connections", " active connections with listeners\n", db.readActiveConnections},
		{"environment", " variables\n", func(r *bufio.Reader) error { _, err := db.readRtEnv(r); return err }},
		{"ordered-environment", " variables\n", func(r *bufio.Reader) error { _, _, err := db.readOrderedRtEnv(r); return err }},
		{"anonymous", "\n", db.readAnonymousObjects},
	} {
		for _, count := range []int{-1, math.MaxInt} {
			t.Run(tc.name+"/"+strconv.Itoa(count), func(t *testing.T) {
				err := tc.read(bufio.NewReader(strings.NewReader(strconv.Itoa(count) + tc.suffix)))
				if err == nil || (count < 0 && !strings.Contains(err.Error(), "invalid")) {
					t.Fatalf("invalid or truncated count: %v", err)
				}
			})
		}
	}
}

func TestReadVMCountsRejectOverflowAndUntrustedLengths(t *testing.T) {
	var raw bytes.Buffer
	w := NewWriter(&raw, store.Snapshot{})
	var frame task.VMFrameSnapshot
	frame.Program.Source = []string{"return 1;"}
	saved := task.Snapshot{ID: 1, VM: &task.VMSnapshot{MaxStackDepth: 50, Frames: []task.VMFrameSnapshot{frame}}}
	if err := w.writeSuspendedTask(saved); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{-1, math.MaxInt, math.MaxInt - 1} {
		for _, marker := range []string{"0 -1 0 50\n", "0 variables\n", "0 rt_stack slots in use\n"} {
			t.Run(fmt.Sprintf("%s/%d", strings.TrimSpace(marker), count), func(t *testing.T) {
				text := raw.String()
				at := strings.Index(text, marker)
				if at < 0 {
					t.Fatal("writer marker missing")
				}
				input := text[:at] + strconv.Itoa(count) + marker[1:]
				_, err := (&Database{Version: 17}).readSuspendedTask(bufio.NewReader(strings.NewReader(input)))
				if err == nil {
					t.Fatal("invalid or truncated VM count accepted")
				}
			})
		}
		input := fmt.Sprintf("1 interrupted reading task\n0\n0\n%d -1 0 50\n", count)
		if _, err := (&Database{Version: 17}).readInterruptedTask(bufio.NewReader(strings.NewReader(input))); err == nil {
			t.Fatalf("invalid/truncated interrupted top %d accepted", count)
		}
	}
}

func TestReadObjectPropertyCountsRejectInvalidLengths(t *testing.T) {
	var raw bytes.Buffer
	w := NewWriter(&raw, store.Snapshot{})
	if err := w.writeObject(&store.SnapshotObject{ID: 0, LastMove: types.NewInt(0)}); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(raw.String(), "\n")
	for _, offset := range []int{4, 3, 2} { // verb count, propdefs, total properties
		for _, count := range []int{-1, math.MaxInt} {
			input := strings.Join(lines[:len(lines)-offset], "\n") + "\n" + strconv.Itoa(count) + "\n"
			if _, err := (&Database{Version: 17}).readObject(bufio.NewReader(strings.NewReader(input))); err == nil {
				t.Fatalf("invalid/truncated object count at offset %d = %d accepted", offset, count)
			}
		}
	}
	old, err := os.ReadFile("../../Test_fresh2.db")
	if err != nil {
		t.Fatal(err)
	}
	text := string(old)
	const marker = "handle_task_timeout\r\n3\r\n172\r\n-1\r\n6\r\n"
	at := strings.Index(text, marker)
	if at < 0 {
		t.Fatal("v4 fixture property-definition anchor missing")
	}
	for _, count := range []int{-1, math.MaxInt} {
		input := text[:at] + strings.TrimSuffix(marker, "6\r\n") + strconv.Itoa(count) + "\r\n"
		if _, err := parseDatabaseWithWaifIdentities(bufio.NewReader(strings.NewReader(input)), nil); err == nil {
			t.Fatalf("v4 invalid/truncated propdefs %d accepted", count)
		}
	}
}

func TestReadRecordsGrowBeyondInitialCapacity(t *testing.T) {
	const count = maxTrustedCount + 1
	var raw bytes.Buffer
	players := make([]types.ObjID, count)
	for i := range players {
		players[i] = types.ObjID(i)
	}
	w := NewWriter(&raw, store.Snapshot{Players: players})
	if err := w.writePlayers(); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	for _, read := range []func(*Database, *bufio.Reader) error{(*Database).readPlayersV4, (*Database).readPlayersV17} {
		db := &Database{}
		if err := read(db, bufio.NewReader(bytes.NewReader(raw.Bytes()))); err != nil || len(db.Players) != count || db.Players[count-1] != count-1 {
			t.Fatalf("large players: len=%d err=%v", len(db.Players), err)
		}
	}
	raw.Reset()
	names := make([]string, count)
	values := make(map[string]types.Value, count)
	for i := range names {
		names[i] = "v" + strconv.Itoa(i)
		values[names[i]] = types.NewInt(int64(i))
	}
	if err := w.writeRtEnv(names, values); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	gotNames, gotValues, err := (&Database{Version: 17}).readOrderedRtEnv(bufio.NewReader(bytes.NewReader(raw.Bytes())))
	if err != nil || len(gotNames) != count || len(gotValues) != count {
		t.Fatalf("large environment: names=%d values=%d err=%v", len(gotNames), len(gotValues), err)
	}
	for i := range names {
		if gotNames[i] != names[i] || gotValues[i].Int() != int64(i) {
			t.Fatalf("environment order changed at %d", i)
		}
	}
}
