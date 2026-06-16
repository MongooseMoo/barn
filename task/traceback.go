package task

import (
	"barn/types"
	"fmt"
	"strings"
)

// FormatTraceback formats a call stack and error into a Toast-style traceback
// Toast format:
//   #<verb_loc>:<verb> (this == #<this>), line <N>:  <error message>
//   ... called from #<verb_loc>:<verb> (this == #<this>), line <N>
//   (End of traceback)
func FormatTraceback(stack []ActivationFrame, err types.ErrorCode) []string {
	if len(stack) == 0 {
		// No stack - just show error
		return []string{
			fmt.Sprintf("(no stack):  %s", err.Message()),
			"(End of traceback)",
		}
	}

	var lines []string

	// Walk the stack from top (most recent) to bottom (oldest)
	for i := len(stack) - 1; i >= 0; i-- {
		frame := &stack[i]

		var line string
		if i == len(stack)-1 {
			// Top frame - where the error occurred - include error message
			line = fmt.Sprintf("#%d:%s (this == #%d), line %d:  %s",
				frame.VerbLoc,
				frame.Verb,
				frame.This,
				frame.LineNumber,
				err.Message())
		} else {
			// Lower frames - show as "called from"
			line = fmt.Sprintf("... called from #%d:%s (this == #%d), line %d",
				frame.VerbLoc,
				frame.Verb,
				frame.This,
				frame.LineNumber)
		}
		lines = append(lines, line)
	}

	// End of traceback marker
	lines = append(lines, "(End of traceback)")

	return lines
}

// FormatTracebackString returns the traceback as a single string with newlines
func FormatTracebackString(stack []ActivationFrame, err types.ErrorCode) string {
	lines := FormatTraceback(stack, err)
	return strings.Join(lines, "\n")
}
