package engine

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// A task's timeline: where a slow or stuck task's time went, slice by slice and
// attempt by attempt. It is recorded only while diagnostics tracking is on (the
// store's conflict census tracking: CONFLICT_CENSUS = 1, or the harness's
// BARN_MONGOOSE_STALE_READS), so with tracking off the cost is one atomic load
// per slice. With it on, each task that runs a slice holds one bounded record:
// at most maxTimelineSlices slices (the first timelineKeepFirst and the latest
// ones; the rest are counted in DroppedSlices) of at most maxTimelineAttempts
// attempts each. A finished task whose timeline spans at least
// slowTimelineThreshold is kept in a ring of the latest maxSlowTimelines for
// /debug/task-timelines, and logged once ("slow task timeline", INFO) when it
// is a player's command (a background loop that suspends between slices
// outlives the threshold by design).

const (
	maxTimelineSlices     = 64
	timelineKeepFirst     = 4
	maxTimelineAttempts   = 8
	maxSlowTimelines      = 32
	maxLiveTimelines      = 64
	slowTimelineThreshold = time.Second
)

// TaskTimeline is a copy of one task's timeline. Start is the start of its first
// recorded slice; Elapsed runs from Start to the end of the latest slice (to
// the copy, for a task still running a slice). State is the task's state when
// the copy was taken or when it finished.
type TaskTimeline struct {
	TaskID        int64
	This          types.ObjID
	VerbLoc       types.ObjID
	Verb          string
	State         string
	Finished      bool
	Start         time.Time
	ElapsedNS     int64
	Retries       int
	Slices        []TimelineSlice
	DroppedSlices int
}

// TimelineSlice is one slice: AtNS after the timeline's start it began, after
// AdmissionWaitNS waiting for background admission and QueueNS from an
// external completion (SQLite, exec, ...) to the slice's start; WallNS is its
// length (0 while running). Attempts holds its attempts, the first
// maxTimelineAttempts of them.
type TimelineSlice struct {
	AtNS            int64
	AdmissionWaitNS int64
	QueueNS         int64
	WallNS          int64
	Attempts        []TimelineAttempt
	DroppedAttempts int
}

// TimelineAttempt is one attempt of a slice: AtNS after the slice began, WallNS
// in the VM, Ticks executed. Outcome is one of commit, suspend, exception,
// lost (validation lost at the commit, re-run), lost-at-boundary (stale at the
// irreversible-effect boundary, re-run), lost-final (validation lost, not
// re-runnable), error (the attempt ended any other way: a failed commit, a
// kill, a launch error) or running. Conflict names the first stale key of
// a lost attempt and Conflicts how many there were. Gate names the exclusive
// commit gate hold the attempt took (cause and builtin), ExclusiveWaitNS its
// acquire wait; SharedWaitNS sums the attempt's shared gate waits at commit
// and SharedBlockedNS the part spent queued behind an exclusive holder or
// request.
type TimelineAttempt struct {
	AtNS            int64
	WallNS          int64
	Ticks           int64
	Outcome         string
	Conflict        string
	Conflicts       int
	Gate            string
	ExclusiveWaitNS int64
	SharedWaitNS    int64
	SharedBlockedNS int64
}

// taskTimeline is the live record. mu guards everything; the task's goroutine
// writes it and /debug/task-timelines and the harness copy it.
type taskTimeline struct {
	mu sync.Mutex
	tl TaskTimeline
	// sliceStart is the current slice's start, for its WallNS.
	sliceStart time.Time
	// pendingAdmission is the background admission wait before the slice
	// about to begin.
	pendingAdmission time.Duration
}

// noteAdmissionWait records the background admission wait before the next
// slice.
func (tl *taskTimeline) noteAdmissionWait(wait time.Duration) {
	if tl == nil {
		return
	}
	tl.mu.Lock()
	tl.pendingAdmission = wait
	tl.mu.Unlock()
}

// attemptWaits accumulates an attempt's commit gate waits; the observer can
// run on another goroutine than the one reading it at the attempt's end.
type attemptWaits struct {
	shared, blocked atomic.Int64
}

func (w *attemptWaits) take() (shared, blocked int64) {
	return w.shared.Swap(0), w.blocked.Swap(0)
}

// DiagnosticsTracking reports whether timelines (and the census's tracked
// tables) are being recorded: the store's conflict census tracking.
func (s *Runtime) DiagnosticsTracking() bool {
	return s.store != nil && s.store.ConflictCensusTracking()
}

// timelineFor returns t's timeline, creating it, while tracking is on, or nil.
func (s *Runtime) timelineFor(t *Task) *taskTimeline {
	if s.store == nil || !s.store.ConflictCensusTracking() {
		return nil
	}
	if tl := t.timeline.Load(); tl != nil {
		return tl
	}
	tl := &taskTimeline{tl: TaskTimeline{TaskID: t.ID, This: t.This, VerbLoc: t.VerbLoc, Verb: t.VerbName}}
	if !t.timeline.CompareAndSwap(nil, tl) {
		return t.timeline.Load()
	}
	return tl
}

// beginSlice opens a slice starting at started, queue after the external
// completion that readied it.
func (tl *taskTimeline) beginSlice(started time.Time, queue time.Duration) {
	if tl == nil {
		return
	}
	tl.mu.Lock()
	defer tl.mu.Unlock()
	admissionWait := tl.pendingAdmission
	tl.pendingAdmission = 0
	if tl.tl.Start.IsZero() {
		tl.tl.Start = started
	}
	tl.sliceStart = started
	if len(tl.tl.Slices) >= maxTimelineSlices {
		tl.tl.Slices = append(tl.tl.Slices[:timelineKeepFirst], tl.tl.Slices[timelineKeepFirst+1:]...)
		tl.tl.DroppedSlices++
	}
	tl.tl.Slices = append(tl.tl.Slices, TimelineSlice{
		AtNS:            int64(started.Sub(tl.tl.Start)),
		AdmissionWaitNS: int64(admissionWait),
		QueueNS:         int64(queue),
	})
}

// beginAttempt opens an attempt of the current slice at now.
func (tl *taskTimeline) beginAttempt(now time.Time) {
	if tl == nil {
		return
	}
	tl.mu.Lock()
	defer tl.mu.Unlock()
	sl := tl.currentLocked()
	if sl == nil {
		return
	}
	if len(sl.Attempts) >= maxTimelineAttempts {
		sl.DroppedAttempts++
		return
	}
	sl.Attempts = append(sl.Attempts, TimelineAttempt{AtNS: int64(now.Sub(tl.sliceStart)), Outcome: "running"})
}

func (tl *taskTimeline) currentLocked() *TimelineSlice {
	if len(tl.tl.Slices) == 0 {
		return nil
	}
	return &tl.tl.Slices[len(tl.tl.Slices)-1]
}

// noteGate records the exclusive gate hold the current attempt took.
func (tl *taskTimeline) noteGate(holder GateHolder, wait time.Duration) {
	if tl == nil {
		return
	}
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if a := tl.attemptLocked(); a != nil {
		a.Gate = holder.label()
		a.ExclusiveWaitNS += int64(wait)
	}
}

func (tl *taskTimeline) attemptLocked() *TimelineAttempt {
	sl := tl.currentLocked()
	if sl == nil || len(sl.Attempts) == 0 || sl.DroppedAttempts > 0 {
		return nil
	}
	return &sl.Attempts[len(sl.Attempts)-1]
}

// endAttempt closes the current attempt with its outcome, VM wall time and
// ticks, the reads its transaction found stale, and its shared gate waits.
func (tl *taskTimeline) endAttempt(outcome string, wall time.Duration, ticks int64, txn *dbstore.StoreTxn, waits *attemptWaits) {
	if tl == nil {
		return
	}
	var shared, blocked int64
	if waits != nil {
		shared, blocked = waits.take()
	}
	var conflicts []dbstore.ReadConflict
	if strings.HasPrefix(outcome, "lost") {
		conflicts = txn.Conflicts()
	}
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if outcome == "lost" || outcome == "lost-at-boundary" {
		tl.tl.Retries++
	}
	a := tl.attemptLocked()
	if a == nil {
		return
	}
	a.Outcome = outcome
	a.WallNS = int64(wall)
	a.Ticks = ticks
	a.SharedWaitNS += shared
	a.SharedBlockedNS += blocked
	a.Conflicts = len(conflicts)
	if len(conflicts) > 0 {
		a.Conflict = ConflictKeyString(conflicts[0])
	}
}

// endSlice closes the current slice now.
func (tl *taskTimeline) endSlice() {
	if tl == nil {
		return
	}
	now := time.Now()
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if sl := tl.currentLocked(); sl != nil {
		sl.WallNS = int64(now.Sub(tl.sliceStart))
		for i := range sl.Attempts {
			if sl.Attempts[i].Outcome == "running" {
				sl.Attempts[i].Outcome = "error"
			}
		}
	}
	tl.tl.ElapsedNS = int64(now.Sub(tl.tl.Start))
}

// snapshot copies the timeline with the given state.
func (tl *taskTimeline) snapshot(state string, finished bool) TaskTimeline {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	out := tl.tl
	out.State, out.Finished = state, finished || out.Finished
	if !finished && !tl.tl.Finished && len(out.Slices) > 0 && out.Slices[len(out.Slices)-1].WallNS == 0 {
		out.ElapsedNS = int64(time.Since(out.Start))
	}
	out.Slices = make([]TimelineSlice, len(tl.tl.Slices))
	for i, sl := range tl.tl.Slices {
		sl.Attempts = append([]TimelineAttempt(nil), sl.Attempts...)
		out.Slices[i] = sl
	}
	return out
}

// Timeline returns a copy of the task's timeline, and false when none was
// recorded (diagnostics tracking off, or no slice ran yet).
func (t *Task) Timeline() (TaskTimeline, bool) {
	tl := t.timeline.Load()
	if tl == nil {
		return TaskTimeline{}, false
	}
	return tl.snapshot(t.GetState().String(), false), true
}

// slowTimelines is the ring of the latest finished slow timelines.
type slowTimelines struct {
	mu   sync.Mutex
	ring []TaskTimeline
	next int
}

func (r *slowTimelines) add(tl TaskTimeline) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.ring) < maxSlowTimelines {
		r.ring = append(r.ring, tl)
		return
	}
	r.ring[r.next] = tl
	r.next = (r.next + 1) % maxSlowTimelines
}

func (r *slowTimelines) list() []TaskTimeline {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]TaskTimeline(nil), r.ring...)
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// finishTimeline closes a finished task's timeline, logging and keeping it when
// it spans slowTimelineThreshold or more.
func (s *Runtime) finishTimeline(t *Task, state string) {
	tl := t.timeline.Load()
	if tl == nil {
		return
	}
	tl.mu.Lock()
	tl.tl.Finished = true
	tl.mu.Unlock()
	snap := tl.snapshot(state, true)
	if s.store != nil {
		s.store.NoteTaskRetries(dbstore.ConflictLabel{Obj: snap.VerbLoc, Verb: snap.Verb}, snap.Retries)
	}
	if time.Duration(snap.ElapsedNS) < slowTimelineThreshold {
		return
	}
	s.slowTimelines.add(snap)
	if !t.FromCommand {
		// A background loop that suspends between slices outlives the
		// threshold by design; only a player's command is logged.
		return
	}
	slog.LogAttrs(s.ctx, slog.LevelInfo, "slow task timeline",
		slog.Int64("task_id", snap.TaskID), slog.Int64("this", int64(snap.This)),
		slog.String("verb", snap.Verb), slog.String("state", state),
		slog.Duration("elapsed", time.Duration(snap.ElapsedNS)),
		slog.Int("slices", len(snap.Slices)+snap.DroppedSlices), slog.Int("retries", snap.Retries),
		slog.String("timeline", FormatTaskTimeline(snap)))
}

// TaskTimelines returns the timelines of up to maxLiveTimelines unfinished
// tasks, oldest first, and the latest finished slow ones.
func (s *Runtime) TaskTimelines() (live, slow []TaskTimeline) {
	for _, t := range s.taskManager.Snapshot() {
		if t == nil {
			continue
		}
		if tl, ok := t.Timeline(); ok && !tl.Finished {
			live = append(live, tl)
		}
	}
	sort.Slice(live, func(i, j int) bool { return live[i].Start.Before(live[j].Start) })
	if len(live) > maxLiveTimelines {
		live = live[:maxLiveTimelines]
	}
	return live, s.slowTimelines.list()
}

// ConflictKeyString names the key a stale read was on: #obj.prop, #obj.prop[]
// for a keyed map read, #obj:verb, or the kind and object.
func ConflictKeyString(c dbstore.ReadConflict) string {
	return conflictKeyName(c.Kind, c.ObjID, c.Name)
}

func conflictKeyName(kind dbstore.ConflictKind, obj types.ObjID, name string) string {
	switch kind {
	case dbstore.ConflictProperty:
		return fmt.Sprintf("#%d.%s", obj, name)
	case dbstore.ConflictMapKey:
		return fmt.Sprintf("#%d.%s[]", obj, name)
	case dbstore.ConflictVerb:
		return fmt.Sprintf("#%d:%s", obj, name)
	case dbstore.ConflictVerbShape:
		return "(any verb or ancestry)"
	case dbstore.ConflictWaif:
		return "waif " + name
	case dbstore.ConflictTask:
		return "task " + name
	default:
		return fmt.Sprintf("%s #%d", kind, obj)
	}
}

// FormatTaskTimeline renders a timeline on one line per slice, for the slow
// task log record and the harness.
func FormatTaskTimeline(tl TaskTimeline) string {
	var b strings.Builder
	ms := func(ns int64) string { return time.Duration(ns).Round(10 * time.Microsecond).String() }
	fmt.Fprintf(&b, "task %d #%d:%s (verb on #%d) state=%s elapsed=%s retries=%d slices=%d",
		tl.TaskID, tl.This, tl.Verb, tl.VerbLoc, tl.State, ms(tl.ElapsedNS), tl.Retries, len(tl.Slices)+tl.DroppedSlices)
	for i, sl := range tl.Slices {
		if tl.DroppedSlices > 0 && i == timelineKeepFirst {
			fmt.Fprintf(&b, "\n  ... %d slices dropped ...", tl.DroppedSlices)
		}
		fmt.Fprintf(&b, "\n  slice at=%s admission_wait=%s queue=%s wall=%s:", ms(sl.AtNS), ms(sl.AdmissionWaitNS), ms(sl.QueueNS), ms(sl.WallNS))
		for _, a := range sl.Attempts {
			fmt.Fprintf(&b, " [%s at=%s vm=%s ticks=%d", a.Outcome, ms(a.AtNS), ms(a.WallNS), a.Ticks)
			if a.Conflict != "" {
				fmt.Fprintf(&b, " stale=%s", a.Conflict)
				if a.Conflicts > 1 {
					fmt.Fprintf(&b, "+%d", a.Conflicts-1)
				}
			}
			if a.Gate != "" {
				fmt.Fprintf(&b, " gate=%q excl_wait=%s", a.Gate, ms(a.ExclusiveWaitNS))
			}
			if a.SharedWaitNS > 0 {
				fmt.Fprintf(&b, " shared_wait=%s behind_excl=%s", ms(a.SharedWaitNS), ms(a.SharedBlockedNS))
			}
			b.WriteByte(']')
		}
		if sl.DroppedAttempts > 0 {
			fmt.Fprintf(&b, " +%d attempts", sl.DroppedAttempts)
		}
	}
	return b.String()
}
