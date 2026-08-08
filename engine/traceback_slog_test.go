package engine

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// captureRecords installs a JSON logger as the default and returns the records
// it collected. The default logger is restored when the test ends.
func captureRecords(t *testing.T, emit func()) []map[string]any {
	t.Helper()

	var buf bytes.Buffer
	prior := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prior) })

	emit()

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not valid JSON: %v (%q)", err, line)
		}
		records = append(records, rec)
	}
	return records
}

// sampleStack is a two-deep call stack: #2:look called #10:look_self.
func sampleStack() []task.ActivationFrame {
	return []task.ActivationFrame{
		{
			This: 2, VerbLoc: 2, Verb: "look", StoredVerb: "look",
			Player: 2, Programmer: 2, LineNumber: 12, SourceLine: "this:look_self();",
		},
		{
			This: 10, VerbLoc: 10, Verb: "look_self", StoredVerb: "look_self",
			Player: 2, Programmer: 2, LineNumber: 3, SourceLine: `x = 1 + "a";`,
		},
	}
}

// A traceback must be one record. Split across lines, concurrent tasks interleave
// and the stack cannot be reassembled from the log afterwards.
func TestLogTracebackEmitsSingleRecord(t *testing.T) {
	s := &Runtime{}
	tk := &task.Task{ID: 42, This: 10, VerbName: "look_self", CallStack: sampleStack()}

	records := captureRecords(t, func() { s.logTraceback(tk, types.E_TYPE, tk.CallStack) })

	if len(records) != 1 {
		t.Fatalf("want exactly 1 record for a traceback, got %d", len(records))
	}
	rec := records[0]
	if rec["level"] != "ERROR" || rec["msg"] != "uncaught exception" {
		t.Errorf("unexpected level/msg: %#v", rec)
	}
	if rec["task_id"] != float64(42) || rec["verb"] != "look_self" || rec["this"] != float64(10) {
		t.Errorf("record does not identify the task: %#v", rec)
	}
	if rec["error"] != "E_TYPE" {
		t.Errorf("error = %v, want E_TYPE", rec["error"])
	}
}

// The logged text must be the same traceback the player sees, so the two can be
// compared directly when a report says "the player saw X".
func TestLoggedTracebackMatchesPlayerFormatting(t *testing.T) {
	s := &Runtime{}
	stack := sampleStack()
	tk := &task.Task{ID: 1, This: 10, VerbName: "look_self", CallStack: stack}

	records := captureRecords(t, func() { s.logTraceback(tk, types.E_TYPE, tk.CallStack) })

	want := task.FormatTracebackString(stack, types.E_TYPE)
	if got := records[0]["traceback"]; got != want {
		t.Errorf("traceback attr does not match player formatting:\n got: %q\nwant: %q", got, want)
	}
}

// The frames array is the machine-readable half: an agent reconstructs the call
// chain from it without parsing rendered text.
func TestTracebackFramesCarryStructuredStack(t *testing.T) {
	s := &Runtime{}
	tk := &task.Task{ID: 1, This: 10, VerbName: "look_self", CallStack: sampleStack()}

	records := captureRecords(t, func() { s.logTraceback(tk, types.E_TYPE, tk.CallStack) })

	frames, ok := records[0]["frames"].([]any)
	if !ok || len(frames) != 2 {
		t.Fatalf("want 2 structured frames, got %#v", records[0]["frames"])
	}

	// Most recent frame first, matching the rendered traceback's order.
	top, ok := frames[0].(map[string]any)
	if !ok {
		t.Fatalf("frame is not an object: %#v", frames[0])
	}
	if top["verb"] != "look_self" || top["verbloc"] != float64(10) || top["line"] != float64(3) {
		t.Errorf("top frame is wrong: %#v", top)
	}
	if top["source"] != `x = 1 + "a";` {
		t.Errorf("top frame lost its source line: %#v", top["source"])
	}

	caller := frames[1].(map[string]any)
	if caller["verb"] != "look" || caller["line"] != float64(12) {
		t.Errorf("caller frame is wrong: %#v", caller)
	}
}

// Printed tracebacks name a frame by its stored name spec, and the eval'd-code
// activation as "Input to EVAL". The structured frames must agree.
func TestTracebackFramesUseStoredVerbNames(t *testing.T) {
	s := &Runtime{}
	stack := []task.ActivationFrame{
		{This: 2, VerbLoc: 2, Verb: "eval", StoredVerb: "eval*-d", LineNumber: 1},
		{This: 2, VerbLoc: 2, Verb: "", LineNumber: 1, IsEvalFrame: true},
	}
	tk := &task.Task{ID: 1, CallStack: stack}

	records := captureRecords(t, func() { s.logTraceback(tk, types.E_TYPE, tk.CallStack) })

	frames := records[0]["frames"].([]any)
	if got := frames[0].(map[string]any)["verb"]; got != "Input to EVAL" {
		t.Errorf("eval frame verb = %v, want \"Input to EVAL\"", got)
	}
	if got := frames[1].(map[string]any)["verb"]; got != "eval*-d" {
		t.Errorf("verb = %v, want the stored name spec \"eval*-d\"", got)
	}
}

// E_VERBNF is the normal outcome for an optional hook verb; logging it would
// bury real failures in noise.
func TestCallVerbTracebackSkipsVerbNotFound(t *testing.T) {
	s := &Runtime{}

	records := captureRecords(t, func() {
		s.logCallVerbTraceback(2, "user_disconnected", types.E_VERBNF, sampleStack(), 2)
	})

	if len(records) != 0 {
		t.Errorf("E_VERBNF should not be logged, got %#v", records)
	}
}

func TestCallVerbTracebackLogsRealErrors(t *testing.T) {
	s := &Runtime{}

	records := captureRecords(t, func() {
		s.logCallVerbTraceback(10, "look_self", types.E_TYPE, sampleStack(), 2)
	})

	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	rec := records[0]
	if rec["msg"] != "verb call exception" || rec["verb"] != "look_self" || rec["player"] != float64(2) {
		t.Errorf("unexpected record: %#v", rec)
	}
	if _, ok := rec["frames"].([]any); !ok {
		t.Errorf("record is missing structured frames: %#v", rec)
	}
}
