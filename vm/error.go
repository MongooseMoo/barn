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

// Call arguments are held to max_list_value_bytes the way Toast holds them.
// Its compiler builds f(a1, ..., an) arguments as a list (code_gen.cc
// generate_arg_list): OP_MAKE_SINGLETON_LIST for a1, unchecked, then
// OP_LIST_ADD_TAIL for each later argument, which keeps the list only while
// value_bytes(list) <= max_list_value_bytes (execute.cc OP_LIST_ADD_TAIL) and
// otherwise does PUSH_ERROR_UNLESS_QUOTA(E_QUOTA). Barn keeps unspliced
// arguments on the stack instead, and checks the same prefixes:
//
//   - OP_CHECK_ARGS k, which the compiler emits before evaluating an argument
//     after the second unless it is a literal, checks the k arguments already
//     pushed, so a later argument never runs once the list is refused;
//   - the call itself checks all n (checkArgList).
//
// A refusal raises E_QUOTA when max_concat_catchable is true and aborts the
// task as out of seconds when it is not. In a verb without the d flag Toast
// pushes E_QUOTA in place of the list and keeps evaluating; every later
// OP_LIST_ADD_TAIL then pushes E_TYPE, and the call, given a non-list, yields
// E_TYPE. OP_CHECK_ARGS marks that state by putting None in the first
// argument's slot (types.RefusedArgListBytes), and the call then yields E_TYPE.
//
// The sizes are the values' cached sizes (types.ListBytes), so a check costs
// a few loads per argument, and a list below the smallest limit needs no
// limit lookup. A spliced argument list is built by OP_LIST_APPEND and
// OP_LIST_EXTEND, which check it as Toast's opcodes do.

// checkArgList checks a call's two or more unspliced arguments.
func (vm *VM) checkArgList(args []types.Value) error {
	if size := types.ListBytes(args); size >= builtins.MinListValueBytesLimit {
		return vm.refuseArgList(args, size)
	}
	return nil
}

// refuseArgList is the error a call with arguments args, together size bytes,
// fails with, or nil.
//
//go:noinline
func (vm *VM) refuseArgList(args []types.Value, size int) error {
	if args[0].IsNone() {
		// Refused earlier in a verb without the d flag: the call has no list.
		return newMooError(types.E_TYPE, "argument list refused")
	}
	if limit := vm.Builtins.ListValueLimitForTask(vm.Context); limit <= 0 || size <= limit {
		return nil
	}
	if !vm.Builtins.MaxConcatCatchable(vm.Context) {
		return secondsAbort{}
	}
	if frame := vm.CurrentFrame(); frame != nil && !frame.VerbDebug {
		// E_QUOTA takes the list's place, and the call is given that.
		return newMooError(types.E_TYPE, "argument list refused")
	}
	return newMooError(types.E_QUOTA, "list too large")
}

// argsBelowSmallestLimit reports whether the count values on top of the
// stack, as a list, are smaller than any max_list_value_bytes: OP_CHECK_ARGS's
// dispatch fast path. Out of line, like the other fast-path helpers.
func (vm *VM) argsBelowSmallestLimit(count int) bool {
	return count <= vm.SP && types.ListBytes(vm.Stack[vm.SP-count:vm.SP]) < builtins.MinListValueBytesLimit
}

// executeCheckArgs runs OP_CHECK_ARGS: the arguments pushed so far, as the
// list Toast would hold, against max_list_value_bytes.
func (vm *VM) executeCheckArgs() error {
	count := int(vm.FetchByte())
	if count > vm.SP {
		panic("stack underflow")
	}
	args := vm.Stack[vm.SP-count : vm.SP]
	size := types.ListBytes(args)
	if size < builtins.MinListValueBytesLimit || args[0].IsNone() {
		return nil
	}
	if limit := vm.Builtins.ListValueLimitForTask(vm.Context); limit <= 0 || size <= limit {
		return nil
	}
	if !vm.Builtins.MaxConcatCatchable(vm.Context) {
		return secondsAbort{}
	}
	if frame := vm.CurrentFrame(); frame != nil && !frame.VerbDebug {
		args[0] = types.None
		return nil
	}
	return newMooError(types.E_QUOTA, "list too large")
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

// typeMismatchError is Toast's RAISE_TYPE_MISMATCH/PUSH_TYPE_MISMATCH: E_TYPE
// with type_mismatch_string's message and type_mismatch_value's value,
// {{expected runtime type codes...}, got type code}. The expected codes keep
// Toast's TYPE_COMPLEX_FLAG (a string is 130); the got code is masked.
func typeMismatchError(got types.TypeCode, expected ...types.TypeCode) MooError {
	var msg strings.Builder
	msg.WriteString("Type mismatch (expected ")
	codes := make([]types.Value, len(expected))
	for i, t := range expected {
		if i > 0 {
			if i == len(expected)-1 {
				msg.WriteString(" or ")
			} else {
				msg.WriteString(", ")
			}
		}
		msg.WriteString(t.ToastName())
		codes[i] = types.NewInt(int64(toastRuntimeType(t)))
	}
	msg.WriteString("; got ")
	msg.WriteString(got.ToastName())
	msg.WriteString(")")
	value := types.NewList([]types.Value{types.NewList(codes), types.NewInt(int64(got))})
	return newMooError(types.E_TYPE, msg.String()).withMessage().withValue(value)
}

// toastRuntimeType is Toast's in-memory var_type: the pointer-carrying types
// have TYPE_COMPLEX_FLAG (0x80) set on their database code.
func toastRuntimeType(t types.TypeCode) int {
	switch t {
	case types.TYPE_STR, types.TYPE_LIST, types.TYPE_MAP, types.TYPE_ANON, types.TYPE_WAIF:
		return int(t) | 0x80
	}
	return int(t)
}

// typeMismatchMessage renders Toast's type_mismatch_string for one expected
// type: "Type mismatch (expected list; got string)".
func typeMismatchMessage(expected, got types.TypeCode) string {
	return fmt.Sprintf("Type mismatch (expected %s; got %s)", expected.ToastName(), got.ToastName())
}
