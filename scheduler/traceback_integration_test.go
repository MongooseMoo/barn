package scheduler

import (
	"strings"
	"testing"

	dbstore "barn/db/store"
)

// A verb that raises an uncaught error must produce exactly one log record
// carrying the whole traceback. This drives the real scheduler — a compiled verb
// raising a real E_TYPE — rather than a fabricated call stack, so it covers the
// actual call site in runTask rather than the formatting in isolation.
func TestUncaughtExceptionInRealTaskLogsOneRecord(t *testing.T) {
	resetServerVerbTaskManager(t)
	t.Cleanup(func() { resetServerVerbTaskManager(t) })

	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	store.AddVerb(0, dbstore.NewVerb("boom", []string{"boom"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{`x = 1 + "a";`}))

	s := NewScheduler(store)

	records := captureRecords(t, func() {
		if _, err := s.RunServerVerbTask(0, "boom", nil, 2); err != nil {
			t.Fatalf("run verb task: %v", err)
		}
	})

	var found map[string]any
	for _, rec := range records {
		if rec["msg"] == "uncaught exception" {
			if found != nil {
				t.Fatal("the traceback was logged as more than one record")
			}
			found = rec
		}
	}
	if found == nil {
		t.Fatalf("no uncaught-exception record was logged; got %#v", records)
	}

	if found["error"] != "E_TYPE" {
		t.Errorf("error = %v, want E_TYPE", found["error"])
	}
	if found["verb"] != "boom" {
		t.Errorf("verb = %v, want boom", found["verb"])
	}

	// The frames must locate the failure: the raising verb, and the line of source
	// that raised. That is the "why" an operator is reading the log for.
	frames, ok := found["frames"].([]any)
	if !ok || len(frames) == 0 {
		t.Fatalf("record carries no structured frames: %#v", found["frames"])
	}
	top, ok := frames[0].(map[string]any)
	if !ok {
		t.Fatalf("frame is not an object: %#v", frames[0])
	}
	if top["verb"] != "boom" {
		t.Errorf("top frame verb = %v, want boom", top["verb"])
	}
	if top["source"] != `x = 1 + "a";` {
		t.Errorf("top frame source = %v, want the raising line", top["source"])
	}
	if top["line"] != float64(1) {
		t.Errorf("top frame line = %v, want 1", top["line"])
	}

	// And the rendered text must be present and complete, so a human reads the
	// same traceback the player would have seen.
	tb, _ := found["traceback"].(string)
	if !strings.Contains(tb, "(End of traceback)") || !strings.Contains(tb, "Type mismatch") {
		t.Errorf("traceback text is missing or truncated: %q", tb)
	}
}
