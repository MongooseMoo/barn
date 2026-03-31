package task

import (
	"barn/types"
	"fmt"
	"strings"
)

// FormatTraceback formats a call stack and error into a Toast-style traceback
// Toast format:
//   #<player> <- #<verb_loc>:<verb> (this == #<this>), line <N>:  <error message>
//   #<player> <- ... called from #<verb_loc>:<verb> (this == #<this>), line <N>
//   #<player> <- (End of traceback)
func FormatTraceback(stack []ActivationFrame, err types.ErrorCode, player types.ObjID) []string {
	return FormatTracebackWithMessage(stack, err.Message(), player)
}

// FormatTracebackWithMessage formats a call stack and explicit error message
// into a Toast-style traceback.
func FormatTracebackWithMessage(stack []ActivationFrame, errMsg string, player types.ObjID) []string {
	if strings.TrimSpace(errMsg) == "" {
		errMsg = "Unknown error"
	}

	if len(stack) == 0 {
		// No stack - just show error
		return []string{
			fmt.Sprintf("#%d <- (no stack):  %s", player, errMsg),
			fmt.Sprintf("#%d <- (End of traceback)", player),
		}
	}

	var lines []string

	// Walk the stack from top (most recent) to bottom (oldest)
	for i := len(stack) - 1; i >= 0; i-- {
		frame := &stack[i]

		var line string
		if i == len(stack)-1 {
			// Top frame - where the error occurred - include error message
			line = fmt.Sprintf("#%d <- #%d:%s (this == #%d), line %d:  %s",
				player,
				frame.VerbLoc,
				frame.Verb,
				frame.This,
				frame.LineNumber,
				errMsg)
		} else {
			// Lower frames - show as "called from"
			line = fmt.Sprintf("#%d <- ... called from #%d:%s (this == #%d), line %d",
				player,
				frame.VerbLoc,
				frame.Verb,
				frame.This,
				frame.LineNumber)
		}
		lines = append(lines, line)
	}

	// End of traceback marker
	lines = append(lines, fmt.Sprintf("#%d <- (End of traceback)", player))

	return lines
}

// FormatTracebackString returns the traceback as a single string with newlines
func FormatTracebackString(stack []ActivationFrame, err types.ErrorCode, player types.ObjID) string {
	lines := FormatTracebackWithMessage(stack, err.Message(), player)
	return strings.Join(lines, "\n")
}
