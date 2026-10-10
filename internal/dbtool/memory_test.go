package dbtool

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbformat "github.com/MongooseMoo/barn/db/format"
)

func TestMemoryReportPrintsHeapAndSlotCounts(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.db")
	writeDatabaseFixture(t, source)
	profile := filepath.Join(t.TempDir(), "heap.pprof")
	var out bytes.Buffer

	if err := MemoryReport(&out, source, profile); err != nil {
		t.Fatalf("MemoryReport: %v", err)
	}
	for _, want := range []string{"objects:", "property_slots:", "clear_slots:", "heap_alloc:", "heap_objects:", "heap_inuse:", "sys:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
	if !strings.Contains(out.String(), "objects:          1\n") {
		t.Errorf("fixture holds one object:\n%s", out.String())
	}
	info, err := os.Stat(profile)
	if err != nil {
		t.Fatalf("heap profile: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("heap profile is empty")
	}
}

func TestMemoryReportReturnsLoadFailure(t *testing.T) {
	load := func(string) (*dbformat.Database, error) {
		return nil, errors.New("unreadable")
	}
	var out bytes.Buffer

	err := memoryReport(&out, "missing.db", "", load)
	if err == nil || !strings.Contains(err.Error(), "load database: unreadable") {
		t.Fatalf("memoryReport error = %v, want load failure", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output after failed load = %q, want empty", out.String())
	}
}
