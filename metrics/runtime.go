package metrics

import (
	"context"
	"expvar"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// Phase is a fixed vocabulary: no task IDs, verbs, SQL or arguments become
// expvar keys. Intervals can overlap; these are wall durations, not CPU time.
type Phase uint8

const (
	Slice Phase = iota
	VMAttempt
	RetryWaste
	InputAdmission
	BackgroundAdmission
	Readmission
	CommitGate
	SQLiteHandleWait
	SQLiteWork
	AsyncWork
	ResumeQueue
	ResumeToVM
	phaseCount
)

var phaseNames = [...]string{
	"slice", "vm_attempt", "retry_waste", "input_admission", "background_admission", "readmission",
	"commit_gate", "sqlite_handle_wait", "sqlite_work", "async_work",
	"resume_queue", "resume_to_vm",
}

var durationBounds = [...]time.Duration{
	time.Microsecond, 10 * time.Microsecond, 100 * time.Microsecond,
	time.Millisecond, 10 * time.Millisecond, 100 * time.Millisecond,
	time.Second, 10 * time.Second,
}

type phaseCounter struct {
	count, total, maximum, ticks atomic.Int64
	buckets                      [len(durationBounds) + 1]atomic.Int64
}

// PhaseStats is a cumulative process-wide observation. Buckets are disjoint;
// the last is unbounded. Concurrent scrapes are approximate, not atomic views.
type PhaseStats struct {
	Count   int64   `json:"count"`
	TotalNS int64   `json:"total_ns"`
	MaxNS   int64   `json:"max_ns"`
	Ticks   int64   `json:"ticks"`
	UpperNS []int64 `json:"upper_ns"`
	Buckets []int64 `json:"buckets"`
}

var runtimePhases [phaseCount]phaseCounter
var detailedRuntime atomic.Bool

var (
	AsyncStarted   = expvar.NewInt("barn.async_started")
	AsyncCompleted = expvar.NewInt("barn.async_completed")
	AsyncRejected  = expvar.NewInt("barn.async_completion_rejected")
)

func init() {
	detailedRuntime.Store(os.Getenv("BARN_RUNTIME_DIAGNOSTICS") == "1")
	expvar.Publish("barn.runtime_phases", expvar.Func(func() any {
		out := make(map[string]PhaseStats, phaseCount)
		for p := Phase(0); p < phaseCount; p++ {
			out[p.String()] = RuntimePhaseStats(p)
		}
		return out
	}))
}

func (p Phase) String() string { return phaseNames[p] }

// ParsePhase accepts only the bounded phase vocabulary.
func ParsePhase(name string) (Phase, bool) {
	for p := Phase(0); p < phaseCount; p++ {
		if p.String() == name {
			return p, true
		}
	}
	return 0, false
}

// RuntimePhaseStats returns totals without retaining individual observations.
func RuntimePhaseStats(p Phase) PhaseStats { return runtimePhases[p].snapshot() }

func (c *phaseCounter) snapshot() PhaseStats {
	s := PhaseStats{Count: c.count.Load(), TotalNS: c.total.Load(), MaxNS: c.maximum.Load(), Ticks: c.ticks.Load(),
		UpperNS: make([]int64, len(durationBounds)), Buckets: make([]int64, len(c.buckets))}
	for i, bound := range durationBounds {
		s.UpperNS[i] = int64(bound)
	}
	for i := range c.buckets {
		s.Buckets[i] = c.buckets[i].Load()
	}
	return s
}

func (c *phaseCounter) observe(elapsed time.Duration, ticks int64) {
	elapsed, ticks = max(elapsed, 0), max(ticks, 0)
	c.total.Add(int64(elapsed))
	c.ticks.Add(ticks)
	for prior := c.maximum.Load(); int64(elapsed) > prior; prior = c.maximum.Load() {
		if c.maximum.CompareAndSwap(prior, int64(elapsed)) {
			break
		}
	}
	bucket := len(durationBounds)
	for i, bound := range durationBounds {
		if elapsed <= bound {
			bucket = i
			break
		}
	}
	c.buckets[bucket].Add(1)
	c.count.Add(1)
}

// Identity labels opt-in local logs only. It contains no runtime values.
type Identity struct {
	TaskID int64
	This   int64
	Verb   string
}

// ObserveRuntime records every interval, including zero-length intervals.
// Detailed records additionally require BARN_RUNTIME_DIAGNOSTICS=1 and debug
// logging. Their overhead belongs to diagnostic runs, not performance claims.
func ObserveRuntime(p Phase, id Identity, elapsed time.Duration, ticks int64) {
	runtimePhases[p].observe(elapsed, ticks)
	if detailedRuntime.Load() && slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		slog.LogAttrs(context.Background(), slog.LevelDebug, "runtime phase",
			slog.String("phase", p.String()), slog.Int64("task_id", id.TaskID),
			slog.Int64("this", id.This), slog.String("verb", id.Verb),
			slog.Int64("elapsed_ns", int64(max(elapsed, 0))), slog.Int64("ticks", max(ticks, 0)))
	}
}

// RuntimeEvent records the external lifecycle without retaining its payload.
func RuntimeEvent(stage string, id Identity, generation uint64, accepted bool) {
	if detailedRuntime.Load() && slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		slog.LogAttrs(context.Background(), slog.LevelDebug, "threaded lifecycle",
			slog.String("stage", stage), slog.Int64("task_id", id.TaskID),
			slog.Int64("this", id.This), slog.String("verb", id.Verb),
			slog.Uint64("generation", generation), slog.Bool("accepted", accepted))
	}
}

// TerminalException identifies a finalized exception before a database handler
// can replace the current task or issue history. Call only after retry,
// cancellation and suspension decisions. Payloads and rendered errors are never
// accepted here. Like phase records, this requires diagnostic opt-in and debug.
func TerminalException(id Identity, code types.ErrorCode, reason types.ExceptionReason, intrinsicEval, committed bool) {
	if detailedRuntime.Load() && slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		slog.LogAttrs(context.Background(), slog.LevelDebug, "terminal task exception",
			slog.Int64("task_id", id.TaskID), slog.Int64("this", id.This), slog.String("verb", id.Verb),
			slog.String("error", code.String()), slog.String("reason", reason.String()),
			slog.Bool("intrinsic_eval", intrinsicEval), slog.Bool("committed", committed))
	}
}
