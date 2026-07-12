package scheduler

import (
	"log/slog"

	"barn/metrics"
	"barn/task"
	"barn/types"
)

// tbFrame is one verb activation as it appears in a logged traceback. The JSON
// tags are the contract read by log tooling, so an agent can reconstruct the
// call chain without parsing the rendered text.
type tbFrame struct {
	VerbLoc    types.ObjID `json:"verbloc"`
	Verb       string      `json:"verb"`
	This       types.ObjID `json:"this"`
	Player     types.ObjID `json:"player"`
	Programmer types.ObjID `json:"programmer"`
	Line       int         `json:"line"`
	Source     string      `json:"source,omitempty"`
	Eval       bool        `json:"eval,omitempty"`
}

// tbFrames converts a call stack into loggable frames, most recent first —
// the same order, and the same verb naming, the rendered traceback uses.
func tbFrames(stack []task.ActivationFrame) []tbFrame {
	frames := make([]tbFrame, 0, len(stack))
	for i := len(stack) - 1; i >= 0; i-- {
		frame := &stack[i]

		// Match FormatTraceback's naming: the stored name spec, or the eval marker.
		verbName := frame.Verb
		if frame.StoredVerb != "" {
			verbName = frame.StoredVerb
		}
		if frame.IsEvalFrame {
			verbName = "Input to EVAL"
		}

		frames = append(frames, tbFrame{
			VerbLoc:    frame.VerbLoc,
			Verb:       verbName,
			This:       frame.This,
			Player:     frame.Player,
			Programmer: frame.Programmer,
			Line:       frame.LineNumber,
			Source:     frame.SourceLine,
			Eval:       frame.IsEvalFrame,
		})
	}
	return frames
}

// tracebackAttrs describes an error and its call stack. A traceback is logged
// as a single record — one event, one line — because separate lines per frame
// interleave with other tasks' output and cannot be reassembled afterwards.
func tracebackAttrs(err types.ErrorCode, stack []task.ActivationFrame) []any {
	return []any{
		slog.String("error", types.NewErr(err).String()),
		slog.String("error_msg", err.Message()),
		slog.String("traceback", task.FormatTracebackString(stack, err)),
		slog.Any("frames", tbFrames(stack)),
	}
}

// SendTracebackToPlayer sends a formatted traceback through the server-owned output hook.
func (s *Scheduler) SendTracebackToPlayer(player types.ObjID, err types.ErrorCode, stack []task.ActivationFrame) {
	if s.tracebackSender != nil {
		s.tracebackSender(player, err, stack)
		return
	}

	attrs := append([]any{slog.Int64("player", int64(player))}, tracebackAttrs(err, stack)...)
	slog.Error("traceback (no output hook configured)", attrs...)
}

// logTraceback logs a task's uncaught exception to the server log. The stack is
// supplied by the caller so that the log records the same activation stack the
// player is shown — the task's live stack has already unwound by this point.
func (s *Scheduler) logTraceback(t *task.Task, err types.ErrorCode, stack []task.ActivationFrame) {
	metrics.UncaughtExceptions.Add(1)
	attrs := append([]any{
		slog.Int64("task_id", t.ID),
		slog.Int64("this", int64(t.This)),
		slog.String("verb", t.VerbName),
	}, tracebackAttrs(err, stack)...)
	slog.Error("uncaught exception", attrs...)
}

// logCallVerbTraceback logs an uncaught exception from a synchronous verb call.
// E_VERBNF is not logged because it's the normal case for optional hook verbs
func (s *Scheduler) logCallVerbTraceback(objID types.ObjID, verbName string, err types.ErrorCode, stack []task.ActivationFrame, player types.ObjID) {
	if err == types.E_VERBNF {
		return // Verb not found is expected for optional hooks
	}
	metrics.UncaughtExceptions.Add(1)
	attrs := append([]any{
		slog.Int64("this", int64(objID)),
		slog.String("verb", verbName),
		slog.Int64("player", int64(player)),
	}, tracebackAttrs(err, stack)...)
	slog.Error("verb call exception", attrs...)
}
