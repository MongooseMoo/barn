package vm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/types"
)

// MooError carries a MOO error code and an optional detail. Formatted details
// retain their format and arguments until Error is called, keeping the normal
// caught-exception path from paying to construct a message it will not use.
type MooError struct {
	Code   types.ErrorCode
	Detail string
	Reason types.ExceptionReason

	detailFormat string
	detailArgs   []any
	publicDetail bool
	// value is the caught error list's third element; the zero Value is the
	// integer 0 every error without one carries.
	value types.Value
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

func (e MooError) detail() string {
	if e.detailFormat != "" {
		return fmt.Sprintf(e.detailFormat, e.detailArgs...)
	}
	return e.Detail
}

// withValue sets the value a handler sees as the error list's third element.
func (e MooError) withValue(value types.Value) MooError {
	e.value = value
	return e
}

// withMessage marks source-defined MOO error text, rather than a VM diagnostic.
func (e MooError) withMessage() MooError {
	e.publicDetail = true
	return e
}

func (e MooError) Error() string {
	detail := e.detail()
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
	Code   types.ErrorCode
	Value  types.Value
	Reason types.ExceptionReason
}

func (e VMException) Error() string {
	return e.Code.String()
}

// unreportedAbort is what a finished finally body re-raises for a
// task.PendingAbort reason: the task goes on ending, running enclosing
// finally bodies, with no error for a handler to catch or for
// #0:handle_uncaught_error to be told about.
type unreportedAbort struct{}

func (unreportedAbort) Error() string { return "task aborted" }

// secondsAbort ends the task as out of seconds: no handler catches it, no
// finally body runs, whatever the verb's d flag (Toast abort_task(ABORT_SECONDS)).
type secondsAbort struct{}

func (secondsAbort) Error() string { return "task ran out of seconds" }

// quotaError is Toast's PUSH_ERROR_UNLESS_QUOTA(E_QUOTA) for a value past a
// size limit: E_QUOTA when max_concat_catchable is true, else a seconds abort.
func (vm *VM) quotaError(detail string) error {
	if vm.Builtins.MaxConcatCatchable(vm.Context) {
		return newMooError(types.E_QUOTA, detail)
	}
	return secondsAbort{}
}

// Call arguments are held to max_list_value_bytes the way Toast holds them: its
// compiler builds f(a, b, ...) arguments as a list with OP_MAKE_SINGLETON_LIST
// and OP_LIST_ADD_TAIL (OP_LIST_APPEND for @), and each of those but the
// singleton checks the list. A lone unspliced argument is never checked.
//
// While max_list_value_bytes is at or above its default (64537861) no argument
// list is measured: reaching it takes more than 61 MiB of arguments in one
// call, and counting on every call cost a measurable share of each builtin
// call. That is the one place Barn knowingly differs from Toast here.

// checkCallArgsLimit checks a verb or pass() call's arguments; spliced is the
// args list of a splice call, or None.
func (vm *VM) checkCallArgsLimit(args []types.Value, spliced types.Value) error {
	if spliced.Type() == types.TYPE_LIST {
		return vm.checkSplicedArgsLimit(spliced)
	}
	if len(args) < 2 {
		return nil
	}
	return vm.checkArgListLimit(args)
}

// checkArgListLimit checks two or more unspliced arguments.
func (vm *VM) checkArgListLimit(args []types.Value) error {
	if !vm.Builtins.ListLimitLowered(vm.Context) {
		return nil
	}
	return vm.measureArgList(args)
}

//go:noinline
func (vm *VM) measureArgList(args []types.Value) error {
	if size := types.ListBytes(args); size >= builtins.MinListValueBytesLimit {
		return vm.checkArgBytes(size)
	}
	return nil
}

// checkSplicedArgsLimit checks the list a splice call passes.
func (vm *VM) checkSplicedArgsLimit(list types.Value) error {
	if !vm.Builtins.ListLimitLowered(vm.Context) {
		return nil
	}
	return vm.checkArgBytes(types.ValueBytes(list))
}

func (vm *VM) checkArgBytes(size int) error {
	// No limit is below MinListValueBytesLimit, so a small list needs no lookup.
	if size < builtins.MinListValueBytesLimit {
		return nil
	}
	// OP_LIST_ADD_TAIL refuses only a list of more than max_list_value_bytes.
	if limit := vm.Builtins.ListValueLimitForTask(vm.Context); limit > 0 && size > limit {
		return vm.quotaError("list too large")
	}
	return nil
}

// Unwinding may replace a lifecycle callback's error while the outward MOO
// result retains its original code. Only attach guard metadata to a compatible
// outward code; this does not change the existing code, value or control flow.
func outwardExceptionReason(code types.ErrorCode, reason types.ExceptionReason) types.ExceptionReason {
	if code != types.E_MAXREC || reason > types.ExceptionReasonSecondsBudget {
		return types.ExceptionReasonUnknown
	}
	return reason
}

// exceptionReason only reads typed metadata. Text, including a manually raised
// E_MAXREC message, cannot establish which native guard produced an error.
func exceptionReason(err error) types.ExceptionReason {
	for err != nil {
		switch e := err.(type) {
		case VMException:
			return e.Reason
		case *VMException:
			if e != nil {
				return e.Reason
			}
		case MooError:
			return e.Reason
		case *MooError:
			if e != nil {
				return e.Reason
			}
		}
		err = errors.Unwrap(err)
	}
	return types.ExceptionReasonUnknown
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
	// The usual shapes, read without the heap targets errors.As needs.
	for unwrapped := err; ; {
		switch e := unwrapped.(type) {
		case VMException:
			return e.Code
		case MooError:
			return e.Code
		case annotatedError:
			unwrapped = e.err
			continue
		}
		break
	}
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
	if first := frame.Program.Origin.FirstLine; first > 1 {
		// Source is a restored fork body numbered from first.
		line -= first - 1
	}
	if line <= 0 || line > len(frame.Program.Source) {
		return ""
	}
	return strings.TrimSpace(frame.Program.Source[line-1])
}

// typeMismatchMessage renders Toast's type_mismatch_string for one expected
// type: "Type mismatch (expected list; got string)".
func typeMismatchMessage(expected, got types.TypeCode) string {
	return fmt.Sprintf("Type mismatch (expected %s; got %s)", expected.ToastName(), got.ToastName())
}
