package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A record produced by the scheduler for an uncaught MOO error. Kept verbatim
// from a real run so this test fails if the log schema drifts away from what the
// reader expects.
const tracebackRecord = `{"time":"2026-07-12T00:37:44.1-06:00","level":"ERROR","msg":"verb call exception","this":0,"verb":"user_connected","player":15579,"error":"E_TYPE","error_msg":"Type mismatch","traceback":"#0:user_connected (this == #0), line 1:  Type mismatch\n(End of traceback)","frames":[{"verbloc":0,"verb":"user_connected","this":0,"player":15579,"programmer":2,"line":1,"source":"x = 1 + \"a\";"}]}`

const infoRecord = `{"time":"2026-07-12T00:37:40.0-06:00","level":"INFO","msg":"listening","port":9504}`

func writeLog(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "latest.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	return path
}

func TestReadRecordsFiltersByLevel(t *testing.T) {
	path := writeLog(t, infoRecord, tracebackRecord)

	records, err := readRecords(path, levels["ERROR"])
	if err != nil {
		t.Fatalf("readRecords: %v", err)
	}
	if len(records) != 1 || records[0].Msg != "verb call exception" {
		t.Fatalf("want only the error record, got %#v", records)
	}

	all, err := readRecords(path, levels["INFO"])
	if err != nil {
		t.Fatalf("readRecords: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want both records at INFO, got %d", len(all))
	}
}

// The traceback and the source line are the reason to read the log at all; they
// must survive the round trip from the server's record to the reader's output.
func TestErrorRecordCarriesTracebackAndSource(t *testing.T) {
	path := writeLog(t, tracebackRecord)

	records, err := readRecords(path, levels["ERROR"])
	if err != nil {
		t.Fatalf("readRecords: %v", err)
	}
	rec := records[0]

	if !strings.Contains(rec.Traceback, "(End of traceback)") {
		t.Errorf("traceback not parsed: %q", rec.Traceback)
	}
	if len(rec.Frames) != 1 {
		t.Fatalf("want 1 frame, got %d", len(rec.Frames))
	}
	if rec.Frames[0].Source != `x = 1 + "a";` {
		t.Errorf("frame source = %q, want the raising line", rec.Frames[0].Source)
	}
	if rec.Frames[0].Verb != "user_connected" || rec.Frames[0].Line != 1 {
		t.Errorf("frame does not locate the failure: %#v", rec.Frames[0])
	}
	// Fields the reader formats are consumed; the rest stay queryable as attrs.
	if rec.Extra["error"] != "E_TYPE" {
		t.Errorf("error attr lost: %#v", rec.Extra)
	}
	if _, leaked := rec.Extra["traceback"]; leaked {
		t.Error("traceback should be lifted out of the attrs, not printed inline")
	}
}

// A server killed mid-write leaves a partial final line. Reading the log is how
// you find out why it died, so a truncated line must not abort the read.
func TestTruncatedFinalLineIsSkipped(t *testing.T) {
	path := writeLog(t, tracebackRecord, `{"level":"ERROR","msg":"cut off ha`)

	records, err := readRecords(path, levels["ERROR"])
	if err != nil {
		t.Fatalf("a truncated line must not fail the read: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want the one intact record, got %d", len(records))
	}
}

// Go stacks are long; the scanner's default line limit would drop exactly the
// records that matter most.
func TestLongGoStackRecordIsRead(t *testing.T) {
	huge := strings.Repeat("goroutine 1 [running]: github.com/MongooseMoo/barn/vm.(*VM).step(...)\\n", 3000)
	panicRecord := `{"level":"ERROR","msg":"panic in task","panic":"index out of range","go_stack":"` + huge + `"}`
	path := writeLog(t, panicRecord)

	records, err := readRecords(path, levels["ERROR"])
	if err != nil {
		t.Fatalf("readRecords: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("a long panic record was dropped: got %d records", len(records))
	}
	if !strings.Contains(records[0].GoStack, "github.com/MongooseMoo/barn/vm") {
		t.Error("go_stack did not survive the read")
	}
}
