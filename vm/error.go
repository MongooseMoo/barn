package vm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/MongooseMoo/barn/types"
)

// MooError carries a MOO error code and an optional detail. Formatted details
// retain their format and arguments until Error is called, keeping the normal
// caught-exception path from paying to construct a message it will not use.
type MooError struct {
	Code   types.ErrorCode
	Detail string

	detailFormat string
	detailArgs   []any
}

type annotatedError struct {
	err  error
	line int
}

func (e annotatedError) Error() string {
	return fmt.Sprintf("%s (line %d)", e.err, e.line)
}

func (e annotatedError) Unwrap() error {
	return e.err
}

func (e MooError) Error() string {
	detail := e.Detail
	if e.detailFormat != "" {
		detail = fmt.Sprintf(e.detailFormat, e.detailArgs...)
	}
	if detail == "" {
		return e.Code.String()
	}
	return e.Code.String() + ": " + detail
}

func newMooError(code types.ErrorCode, detail string) MooError {
	return MooError{Code: code, Detail: detail}
}

func newMooErrorf(code types.ErrorCode, format string, args ...any) MooError {
	return MooError{Code: code, detailFormat: format, detailArgs: args}
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

// errorCode obtains the code directly from structured VM errors, including
// errors wrapped with source context. String parsing remains solely as a
// compatibility fallback for errors returned by external integrations.
func errorCode(err error) types.ErrorCode {
	var exception VMException
	if errors.As(err, &exception) {
		return exception.Code
	}
	var mooErr MooError
	if errors.As(err, &mooErr) {
		return mooErr.Code
	}
	return extractErrorCode(err)
}

// annotateError wraps an error with source line information if available.
// If the line is 0 (no line info), the original error is returned unchanged.
func (vm *VM) annotateError(err error, line int) error {
	if line > 0 {
		return annotatedError{err: err, line: line}
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

// typeMismatchMessage renders Toast's type_mismatch_string for one expected
// type: "Type mismatch (expected list; got string)".
func typeMismatchMessage(expected, got types.TypeCode) string {
	return fmt.Sprintf("Type mismatch (expected %s; got %s)", toastTypeName(expected), toastTypeName(got))
}

// toastTypeName is Toast's parse_type() spelling of a value type.
func toastTypeName(t types.TypeCode) string {
	switch t {
	case types.TYPE_INT:
		return "integer"
	case types.TYPE_OBJ:
		return "object"
	case types.TYPE_ERR:
		return "error"
	case types.TYPE_STR:
		return "string"
	case types.TYPE_FLOAT:
		return "float"
	case types.TYPE_LIST:
		return "list"
	case types.TYPE_MAP:
		return "map"
	case types.TYPE_ANON:
		return "anonymous object"
	case types.TYPE_WAIF:
		return "waif"
	case types.TYPE_BOOL:
		return "bool"
	}
	return "unknown type"
}
