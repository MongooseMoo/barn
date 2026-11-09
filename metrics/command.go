package metrics

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

var commandSequence atomic.Int64

// Command follows one received input through its root tasks and committed output.
// It retains metadata only, never the command line or response payload. It is
// process-local diagnostic state, deliberately absent from checkpoints.
type Command struct {
	ID, ConnectionID, Player                int64
	received                                time.Time
	mu                                      sync.Mutex
	tasks                                   map[int64]bool
	processed, finished, committed, written bool
	kind                                    string
	timer                                   *time.Timer
	context                                 func() []slog.Attr
}

// NewCommand uses the existing runtime diagnostic opt-in. Disabled collection
// does not allocate per-command timers or retain command state.
func NewCommand(connectionID, player int64, received time.Time, contextAttrs func() []slog.Attr) *Command {
	if !detailedRuntime.Load() || !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		return nil
	}
	c := &Command{ID: commandSequence.Add(1), ConnectionID: connectionID, Player: player,
		received: received, kind: "input", tasks: make(map[int64]bool), context: contextAttrs}
	c.event("received")
	c.timer = time.AfterFunc(100*time.Millisecond, c.slow)
	return c
}

func (c *Command) event(stage string, extra ...slog.Attr) {
	attrs := []slog.Attr{slog.Int64("command_id", c.ID), slog.Int64("conn_id", c.ConnectionID),
		slog.Int64("player", c.Player), slog.String("stage", stage), slog.String("kind", c.kind),
		slog.Int64("elapsed_ns", int64(time.Since(c.received)))}
	slog.LogAttrs(context.Background(), slog.LevelDebug, "player command", append(attrs, extra...)...)
}

func (c *Command) slow() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finished || c.written {
		return
	}
	attrs := []slog.Attr{}
	if c.context != nil {
		attrs = c.context()
	}
	c.event("pending", attrs...)
}

func (c *Command) Dequeued() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.event("dequeued")
}

func (c *Command) Classify(kind string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.kind = kind
	c.event("classified")
}

func (c *Command) TaskStarted(id int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tasks[id] = true
	c.event("task_started", slog.Int64("task_id", id))
}

func (c *Command) TaskFinished(id int64, state string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.tasks[id] {
		return
	}
	delete(c.tasks, id)
	c.event("task_finished", slog.Int64("task_id", id), slog.String("state", state))
	c.finishLocked()
}

func (c *Command) FinishInput(outcome string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.processed {
		return
	}
	c.processed = true
	c.event("input_finished", slog.String("outcome", outcome))
	c.finishLocked()
}

func (c *Command) finishLocked() {
	if c.finished || !c.processed || len(c.tasks) != 0 {
		return
	}
	c.finished = true
	if c.timer != nil {
		c.timer.Stop()
	}
	c.event("finished", slog.Bool("response_committed", c.committed), slog.Bool("response_written", c.written))
}

// Committed is called only when an effect is published, or for a direct server
// reply. Only output to the initiating physical connection can be its response.
func (c *Command) Committed(connectionID int64) {
	if c == nil || connectionID != c.ConnectionID {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.committed {
		c.committed = true
		c.event("first_committed_output")
	}
}

// Output records successful transport writes separately from buffered output
// and failed writes. A completed write is not client rendering/receipt.
func (c *Command) Output(connectionID int64, started time.Time, err error) {
	if c == nil || connectionID != c.ConnectionID {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	attrs := []slog.Attr{slog.Int64("write_ns", int64(time.Since(started)))}
	if err != nil {
		c.event("output_failed", attrs...)
		return
	}
	if c.written {
		return
	}
	c.written = true
	if c.timer != nil {
		c.timer.Stop()
	}
	c.event("first_output", attrs...)
}
