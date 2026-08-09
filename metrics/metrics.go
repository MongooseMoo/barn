// Package metrics publishes Barn's runtime counters through expvar.
//
// The counters answer "what has this server been doing" at a glance — how many
// tasks ran, how many died, how often the database was checkpointed and how long
// it took. They are served as JSON at /debug/vars alongside pprof, so both a
// human and a script can read them without attaching a profiler.
//
// This package imports only the standard library, so any Barn package may depend
// on it without risking an import cycle.
package metrics

import "expvar"

var (
	// TasksStarted counts every task the execution runtime has created.
	TasksStarted = expvar.NewInt("barn.tasks_started")
	// TasksKilled counts tasks that died — killed, or aborted by an error.
	TasksKilled = expvar.NewInt("barn.tasks_killed")

	// UncaughtExceptions counts MOO errors that escaped to the top of a task.
	UncaughtExceptions = expvar.NewInt("barn.uncaught_exceptions")
	// PanicsRecovered counts Go panics caught before they could kill the server.
	// A nonzero value here is always a bug in Barn.
	PanicsRecovered = expvar.NewInt("barn.panics_recovered")

	// Checkpoints counts completed database checkpoints.
	Checkpoints = expvar.NewInt("barn.checkpoints")
	// CheckpointLastMs is how long the most recent checkpoint took.
	CheckpointLastMs = expvar.NewInt("barn.checkpoint_last_ms")

	// GCSweeps counts anonymous-object collection passes.
	GCSweeps = expvar.NewInt("barn.gc_sweeps")
	// GCSweepLastMs is how long the most recent sweep took.
	GCSweepLastMs = expvar.NewInt("barn.gc_sweep_last_ms")
)

// PublishGauge exposes a value that is read on demand rather than counted, such
// as the number of tasks currently alive. Re-publishing a name is ignored: expvar
// panics on a duplicate, and a metric is not worth crashing a server over.
func PublishGauge(name string, read func() int64) {
	if expvar.Get(name) != nil {
		return
	}
	expvar.Publish(name, expvar.Func(func() any { return read() }))
}
