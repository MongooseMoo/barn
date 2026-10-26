package format

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// An observation harness for issue 516, not assertions that defects must remain.
// It never opens a fixture for writing or runs a server.
func TestFormatAudit516Evidence(t *testing.T) {
	raw, err := os.ReadFile("testdata/toastcore.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("fixture toastcore bytes=%d sha256=%x", len(raw), sha256.Sum256(raw))
	text := string(raw)
	firstDot := strings.Index(text, "\n.\n")
	if firstDot < 0 {
		t.Fatal("first queued-task source terminator not found")
	}
	truncated := raw[:firstDot+1]
	guard := &audit516EOFReader{source: bytes.NewReader(truncated)}
	_, err = parseDatabaseWithWaifIdentities(bufio.NewReader(guard), nil)
	t.Logf("truncate before first dot: bytes=%d sha256=%x EOF_reads=%d result=%v", len(truncated), sha256.Sum256(truncated), guard.eofs, err)

	for _, mutation := range []struct{ name, before, after string }{
		{"players-negative", "**\n6\n", "**\n-1\n"},
		{"finalizations-negative", "0 values pending finalization", "-1 values pending finalization"},
		{"queued-negative", "4 queued tasks", "-1 queued tasks"},
		{"suspended-negative", "0 suspended tasks", "-1 suspended tasks"},
		{"version-prefix", "Format Version 17 **", "Format Version 170 **"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			if !strings.Contains(text, mutation.before) {
				t.Fatal("fixture mutation anchor missing")
			}
			mutated := strings.Replace(text, mutation.before, mutation.after, 1)
			defer func() {
				if value := recover(); value != nil {
					t.Logf("panic=%v", value)
				}
			}()
			database, err := parseDatabaseWithWaifIdentities(bufio.NewReader(strings.NewReader(mutated)), nil)
			if err != nil {
				t.Logf("error=%v", err)
			} else {
				t.Logf("accepted version=%d objects=%d", database.Version, len(database.Objects))
			}
		})
	}

	// Follow the store-backed producer in dump_persistence_test.go, starting
	// with a tracked database and retaining the anonymous object via a property.
	database, err := LoadDatabaseReadOnly("../../Test_fresh2.db")
	if err != nil {
		t.Fatal(err)
	}
	s, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	anon, code := s.DirectTxn().CreateObject([]types.ObjID{0}, 3, true)
	if code != types.E_NONE {
		t.Fatal(code)
	}
	if code := s.DirectTxn().DefineProperty(0, "audit_saved_anon", store.NewProperty(types.NewAnon(anon), 3, 0, false, true)); code != types.E_NONE {
		t.Fatal(code)
	}
	verb := store.NewVerb([]string{"audit_program"}, 3, 0, store.VerbArgs{This: "none", Prep: "none", That: "none"}, nil)
	if _, code := s.AddVerb(anon, verb); code != types.E_NONE {
		t.Fatal(code)
	}
	if code := s.DirectTxn().SetVerbCodeByIndex(anon, 0, []string{"return 1;"}); code != types.E_NONE {
		t.Fatal(code)
	}
	waif := types.NewWaif(0, 3)
	s.RegisterWaif(0, waif)
	if code := s.DirectTxn().DefineProperty(0, "audit_saved_waif", store.NewProperty(waif, 3, 0, false, true)); code != types.E_NONE {
		t.Fatal(code)
	}
	var output bytes.Buffer
	snapshot := s.Snapshot()
	if err := NewWriter(&output, snapshot).WriteDatabase(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := parseDatabaseWithWaifIdentities(bufio.NewReader(bytes.NewReader(output.Bytes())), nil)
	if err != nil {
		t.Fatal(err)
	}
	loadedStore, err := reloaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	for _, obj := range reloaded.AnonymousObjs {
		source, present := obj.VerbCodeByIndex(0)
		t.Logf("anonymous id=%d verbs=%d verb_index_exists=%v source=%q", obj.ID(), obj.VerbCount(), present, source)
	}
	for _, obj := range loadedStore.Snapshot().AnonymousObjects {
		t.Logf("anonymous snapshot id=%d HasProgram=%v source=%q", obj.ID, obj.VerbList[0].HasProgram, obj.VerbList[0].Code)
	}
	t.Logf("producer source=%q emitted_program=%v", snapshot.AnonymousObjects[0].VerbList[0].Code, strings.Contains(output.String(), "#8:0\nreturn 1;\n.\n"))
	t.Logf("writer anonymous program=%v; waif registry before=%d loaded_definitions=%d retained_definitions=%d registry_after=%d", snapshot.AnonymousObjects[0].VerbList[0].HasProgram, s.WaifCount(), reloaded.loadedWaifCount, len(reloaded.savedWaifs), loadedStore.WaifCount())
}

type audit516EOFReader struct {
	source *bytes.Reader
	eofs   int
}

func (r *audit516EOFReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	if errors.Is(err, io.EOF) {
		r.eofs++
		if r.eofs > 8 {
			return 0, errors.New("audit stopped after eight ignored EOF results")
		}
	}
	return n, err
}
