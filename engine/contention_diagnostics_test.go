package engine

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

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

// fillTimeline records slices of attempts on a fresh timeline, slice i
// starting i ms after the first.
func fillTimeline(slices, attempts int) *taskTimeline {
	tl := &taskTimeline{tl: TaskTimeline{TaskID: 1, Verb: "loop"}}
	base := time.Unix(1000, 0)
	for i := range slices {
		start := base.Add(time.Duration(i) * time.Millisecond)
		tl.beginSlice(start, 0)
		for range attempts {
			tl.beginAttempt(start)
			tl.endAttempt("lost", time.Microsecond, 5, nil, nil)
		}
		tl.endSlice()
	}
	return tl
}

// A long-lived task keeps only its latest maxTimelineSlices slices, oldest
// first, and counts the rest.
func TestTaskTimelineKeepsLatestSlices(t *testing.T) {
	const slices = maxTimelineSlices + 5
	snap := fillTimeline(slices, 1).snapshot("running", false)
	if len(snap.Slices) != maxTimelineSlices || snap.DroppedSlices != 5 {
		t.Fatalf("kept %d slices, dropped %d; want %d and 5", len(snap.Slices), snap.DroppedSlices, maxTimelineSlices)
	}
	for i, sl := range snap.Slices {
		if want := int64(i+5) * int64(time.Millisecond); sl.AtNS != want || len(sl.Attempts) != 1 {
			t.Fatalf("slice %d at %d with %d attempts, want at %d with 1", i, sl.AtNS, len(sl.Attempts), want)
		}
	}
}

// The slow-task log form of a full timeline holds the totals and the first
// and last few attempts, not every attempt.
func TestTaskTimelineSummaryIsBounded(t *testing.T) {
	snap := fillTimeline(maxTimelineSlices+10, maxTimelineAttempts+2).snapshot("finished", true)
	full := FormatTaskTimeline(snap)
	summary := FormatTaskTimelineSummary(snap, timelineLogAttempts)
	if got := strings.Count(summary, "[lost "); got != 2*timelineLogAttempts {
		t.Errorf("summary shows %d attempts, want %d:\n%s", got, 2*timelineLogAttempts, summary)
	}
	attempts := maxTimelineSlices * maxTimelineAttempts
	for _, want := range []string{fmt.Sprintf("attempts=%d", attempts), fmt.Sprintf("lost=%d", attempts),
		fmt.Sprintf("... %d attempts omitted ...", attempts-2*timelineLogAttempts), "earlier dropped=10"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary lacks %q:\n%s", want, summary)
		}
	}
	if len(summary) >= len(full)/4 {
		t.Errorf("summary is %d bytes, full timeline %d", len(summary), len(full))
	}
}

// Switching tracking off through the runtime releases live tasks' timelines.
func TestSetDiagnosticsTrackingOffReleasesTimelines(t *testing.T) {
	store := newBoundaryTestStore(t)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
	defer s.Stop()
	s.SetDiagnosticsTracking(true)
	task := NewTask(3203, 0, 1000, 1)
	s.taskManager.RegisterTask(task)
	defer s.taskManager.RemoveTask(task.ID)
	if s.timelineFor(task) == nil {
		t.Fatal("no timeline with tracking on")
	}
	s.SetDiagnosticsTracking(false)
	if _, ok := task.Timeline(); ok || store.ConflictCensusTracking() {
		t.Errorf("timeline kept (%v) or tracking still on (%v) after switching off", ok, store.ConflictCensusTracking())
	}
}

// The reports the debug endpoints and the shutdown summary serve
// (CommitGateReport for /debug/commit-gate, TaskTimelines for
// /debug/task-timelines, the store's census for /debug/conflict-census, and
// their text forms) are read while tasks commit through the shared gate,
// conflict on one property and take the exclusive gate at server_log's
// boundary, with tracking off and on. Run with -race.
func TestDiagnosticsReportsReadDuringTasks(t *testing.T) {
	for _, tracking := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[tracking], func(t *testing.T) {
			store := newBoundaryTestStore(t)
			s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 4)
			defer s.Stop()
			s.SetDiagnosticsTracking(tracking)
			ticks, seconds := foregroundTaskLimits(newTestRegistry())
			program := compileTestProgram(t, s.registry, `
#0.write_value = #0.write_value + 1;
if (#0.write_value % 3 == 0)
  server_log("boundary");
endif
#0.read_value = #0.read_value + 1;
`)
			stop := make(chan struct{})
			var readers sync.WaitGroup
			for range 3 {
				readers.Add(1)
				go func() {
					defer readers.Done()
					for {
						select {
						case <-stop:
							return
						default:
						}
						gate := s.CommitGateReport()
						live, slow := s.TaskTimelines()
						census := store.ConflictCensus()
						_ = FormatCommitGate(gate, 10)
						_ = FormatConflictCensus(census, 1, 10, tracking)
						for _, tl := range append(live, slow...) {
							_ = FormatTaskTimeline(tl)
							_ = FormatTaskTimelineSummary(tl, timelineLogAttempts)
						}
					}
				}()
			}
			var tasks sync.WaitGroup
			const workers, perWorker = 4, 15
			for w := range workers {
				tasks.Add(1)
				go func() {
					defer tasks.Done()
					for i := range perWorker {
						task := NewTaskFull(int64(4000+w*perWorker+i), 0, program, ticks, seconds)
						task.Context.IsWizard = true
						task.Context.Log = slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
						task.This, task.VerbLoc, task.VerbName = 0, 0, "bump"
						if err := s.runTask(task); err != nil {
							t.Errorf("runTask: %v", err)
							return
						}
					}
				}()
			}
			tasks.Wait()
			close(stop)
			readers.Wait()
			if v, ec := store.DirectTxn().PropertyValue(0, "read_value"); ec != types.E_NONE || v.Int() != workers*perWorker {
				t.Errorf("read_value = %v (%v), want %d", v, ec, workers*perWorker)
			}
			holds := s.GateHolds()
			if len(holds) == 0 {
				t.Error("no exclusive gate holds recorded")
			}
			t.Logf("lost commits %d, gate holders %d", store.ConflictCensus().LostTxns, len(holds))
		})
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
