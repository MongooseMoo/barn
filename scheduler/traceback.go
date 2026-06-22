package scheduler

import (
	"barn/task"
	"barn/types"
	"log"
)

// SendTracebackToPlayer sends a formatted traceback through the server-owned output hook.
func (s *Scheduler) SendTracebackToPlayer(player types.ObjID, err types.ErrorCode, stack []task.ActivationFrame) {
	if s.tracebackSender != nil {
		s.tracebackSender(player, err, stack)
		return
	}

	lines := task.FormatTraceback(stack, err)
	log.Printf("Traceback for player %v (output hook not configured):", player)
	for _, line := range lines {
		log.Printf("  %s", line)
	}
}

// logTraceback logs a formatted traceback to the server log for a task
func (s *Scheduler) logTraceback(t *task.Task, err types.ErrorCode) {
	stack := t.GetCallStack()
	lines := task.FormatTraceback(stack, err)
	log.Printf("TRACEBACK: Task %d (#%d:%s) uncaught exception %s",
		t.ID, t.This, t.VerbName, types.NewErr(err).String())
	for _, line := range lines {
		log.Printf("TRACEBACK:   %s", line)
	}
	s.logTracebackSource(stack)
}

// logCallVerbTraceback logs a formatted traceback to the server log for a synchronous verb call
// E_VERBNF is not logged because it's the normal case for optional hook verbs
func (s *Scheduler) logCallVerbTraceback(objID types.ObjID, verbName string, err types.ErrorCode, stack []task.ActivationFrame, player types.ObjID) {
	if err == types.E_VERBNF {
		return // Verb not found is expected for optional hooks
	}
	lines := task.FormatTraceback(stack, err)
	log.Printf("TRACEBACK: #%d:%s uncaught exception %s (player #%d)",
		objID, verbName, types.NewErr(err).String(), player)
	for _, line := range lines {
		log.Printf("TRACEBACK:   %s", line)
	}
	s.logTracebackSource(stack)
}

func (s *Scheduler) logTracebackSource(stack []task.ActivationFrame) {
	for i := len(stack) - 1; i >= 0; i-- {
		frame := stack[i]
		if frame.SourceLine == "" {
			continue
		}
		log.Printf("TRACEBACK:     #%d:%s line %d => %s",
			frame.VerbLoc, frame.Verb, frame.LineNumber, frame.SourceLine)
	}
}

// sendTraceback sends a formatted traceback to the player
func (s *Scheduler) sendTraceback(t *task.Task, err types.ErrorCode) {
	if s.tracebackSender == nil {
		return
	}
	s.tracebackSender(t.Owner, err, t.GetCallStack())
}
