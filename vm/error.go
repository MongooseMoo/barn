package vm

import (
	"barn/types"
	"fmt"
	"strings"
)

// MooError wraps an ErrorCode as a Go error
type MooError struct {
	Code types.ErrorCode
}

func (e MooError) Error() string {
	return fmt.Sprintf("E_%d", e.Code)
}

// VMException carries a structured exception value alongside an error code.
// Used for propagating builtin raise() payloads into except variables.
type VMException struct {
	Code  types.ErrorCode
	Value types.Value
}

func (e VMException) Error() string {
	return e.Code.String()
}

// extractErrorCode parses an error code from an error message string.
// Handles messages like "E_DIV: division by zero" or "E_TYPE: ..."
func extractErrorCode(err error) types.ErrorCode {
	msg := err.Error()
	// Look for "E_XXX" at the start or after a space
	for _, prefix := range []string{
		"E_TYPE", "E_DIV", "E_PERM", "E_PROPNF", "E_VERBNF", "E_VARNF",
		"E_INVIND", "E_RECMOVE", "E_MAXREC", "E_RANGE", "E_ARGS",
		"E_NACC", "E_INVARG", "E_QUOTA", "E_FLOAT", "E_FILE", "E_EXEC",
		"E_INTRPT",
	} {
		if len(msg) >= len(prefix) && msg[:len(prefix)] == prefix {
			if code, ok := types.ErrorFromString(prefix); ok {
				return code
			}
		}
	}
	return types.E_NONE
}

// annotateError wraps an error with source line information if available.
// If the line is 0 (no line info), the original error is returned unchanged.
func (vm *VM) annotateError(err error, line int) error {
	if line > 0 {
		return fmt.Errorf("%w (line %d)", err, line)
	}
	return err
}

func (vm *VM) sourceLineForFrame(frame *StackFrame, line int) string {
	if frame == nil || frame.Program == nil || line <= 0 {
		return ""
	}
	if line > len(frame.Program.Source) {
		return ""
	}
	return strings.TrimSpace(frame.Program.Source[line-1])
}
