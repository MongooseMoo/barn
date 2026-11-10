package engine

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// With diagnostics tracking on, a task whose read goes stale before its first
// irreversible builtin and is re-run records: a timeline of one slice with a
// lost-at-boundary attempt naming the stale key and a committed attempt that
// held the gate for server_log; a loss path on #0.read_value at the boundary
// opened by server_log, with its read site, on attempt 1; the verb's
// goodput with one retry and its boundaries; the task's retries; and the
// server_log calls by site. The report formats all of it.
func TestContentionDiagnosticsRecordBoundaryLoss(t *testing.T) {
	store := newBoundaryTestStore(t)
	store.SetConflictCensusTracking(true)
	descriptor, _ := bumpReadValueLiveOnce(t, store)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1, descriptor)
	defer s.Stop()

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := NewTaskFull(3201, 0, compileTestProgram(t, s.registry, `
before = #0.read_value;
bump_read_value_live_once();
server_log("irreversible-once");
#0.write_value = before + 10;
return before;
`), ticks, seconds)
	queued.Context.IsWizard = true
	queued.Context.Log = slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	queued.This, queued.VerbLoc, queued.VerbName = 0, 0, "diag"

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}

	tl, ok := queued.Timeline()
	if !ok {
		t.Fatal("no timeline recorded with tracking on")
	}
	t.Logf("%s", FormatTaskTimeline(tl))
	if len(tl.Slices) != 1 || len(tl.Slices[0].Attempts) != 2 || tl.Retries != 1 {
		t.Fatalf("timeline = %+v, want one slice of two attempts and one retry", tl)
	}
	first, second := tl.Slices[0].Attempts[0], tl.Slices[0].Attempts[1]
	if first.Outcome != "lost-at-boundary" || first.Conflict != "#0.read_value" || first.Ticks <= 0 {
		t.Errorf("first attempt = %+v, want lost-at-boundary on #0.read_value", first)
	}
	if second.Outcome != "commit" || !strings.Contains(second.Gate, "server_log()") {
		t.Errorf("second attempt = %+v, want a commit holding the gate for server_log", second)
	}

	census := store.ConflictCensus()
	var path *dbstore.ConflictPath
	for i := range census.Paths {
		if p := &census.Paths[i]; p.Kind == dbstore.ConflictProperty && p.Name == "read_value" {
			path = p
		}
	}
	if path == nil {
		t.Fatalf("no loss path on #0.read_value: %+v", census.Paths)
	}
	// The program text starts with a newline, so the read is on line 2 and
	// server_log on line 4.
	if path.Loser != (dbstore.ConflictLabel{Obj: 0, Verb: "diag"}) || path.ReadSite.Line != 2 ||
		path.LostAt.Boundary != 1 || path.MaxAttempt != 1 ||
		len(path.BoundarySites) != 1 || path.BoundarySites[0].Builtin != "server_log" || path.BoundarySites[0].Site.Line != 4 {
		t.Errorf("path = %+v, want #0:diag reading at line 2, lost at the server_log boundary (line 4) on attempt 1", *path)
	}
	var verb *dbstore.VerbGoodput
	for i := range census.Verbs {
		if census.Verbs[i].Verb == (dbstore.ConflictLabel{Obj: 0, Verb: "diag"}) {
			verb = &census.Verbs[i]
		}
	}
	if verb == nil || verb.Slices != 1 || verb.Retries() != 1 || verb.WastedTicks <= 0 || verb.Boundaries != 2 ||
		verb.Tasks != 1 || verb.MaxRetriesPerTask != 1 {
		t.Errorf("verb goodput = %+v, want 1 slice, 1 retry, wasted ticks, 2 boundaries, 1 task with 1 retry", verb)
	}
	calls := uint64(0)
	for _, c := range census.BuiltinCalls {
		if c.Builtin == "server_log" {
			calls += c.Calls
		}
	}
	if calls != 2 {
		t.Errorf("server_log calls = %d (%+v), want 2 (one per attempt)", calls, census.BuiltinCalls)
	}
	report := strings.Join(FormatConflictCensus(census, 0, 10, true), "\n")
	for _, want := range []string{"path #0.read_value loser=#0:diag", "lost_at[final=0 suspend=0 boundary=1", "server_log()@", "verb #0:diag"} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
	gate := strings.Join(FormatCommitGate(s.CommitGateReport(), 10), "\n")
	if !strings.Contains(gate, "builtin=server_log") {
		t.Errorf("gate report lacks the builtin:\n%s", gate)
	}
	if live, _ := s.TaskTimelines(); len(live) != 0 {
		t.Errorf("finished task still listed live: %+v", live)
	}
}

// With tracking off no timeline is recorded.
func TestTaskTimelineOffWithoutTracking(t *testing.T) {
	store := newBoundaryTestStore(t)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
	defer s.Stop()
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := NewTaskFull(3202, 0, compileTestProgram(t, s.registry, `#0.write_value = 3;`), ticks, seconds)
	queued.Context.IsWizard = true
	queued.Context.Log = slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if _, ok := queued.Timeline(); ok {
		t.Error("timeline recorded with tracking off")
	}
	if v, ec := store.DirectTxn().PropertyValue(0, "write_value"); ec != types.E_NONE || v.Int() != 3 {
		t.Errorf("write_value = %v (%v), want 3", v, ec)
	}
}
