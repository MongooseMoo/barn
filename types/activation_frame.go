package types

import "strings"

// ActivationFrame represents a single verb call on the call stack.
// This is what callers() returns.
type ActivationFrame struct {
	This                    ObjID  // Object this verb is called on (prototype for primitives)
	ThisValue               Value  // For primitive prototype calls: the actual primitive value
	Player                  ObjID  // Player who initiated this task
	Programmer              ObjID  // Programmer (for permissions)
	Caller                  ObjID  // Object that called this verb
	Verb                    string // Verb name as invoked (callers()/task_stack())
	BuiltinCaller           string // Builtin whose continuation invoked this activation
	BuiltinPC               int
	StoredVerb              string                   // Verb's stored name spec incl. wildcards; used by printed tracebacks
	StoredVerbNames         []string                 // Lazy form used by live frames; immutable verb storage owns the backing array
	VerbLoc                 ObjID                    // Object where verb is defined
	Args                    []Value                  // Arguments passed to verb
	LineNumber              int                      // Current line number in verb
	RuntimeVariables        Value                    // Bound runtime variables in declaration order
	RuntimeVariableSnapshot *RuntimeVariableSnapshot // Immutable locals captured before suspension
	SourceLine              string                   // Source text at LineNumber (best-effort, for debugging/logging)
	ServerInitiated         bool                     // True if this is a server-invoked call (do_login_command, etc.)
	IsEvalFrame             bool                     // True if this is an eval() infrastructure frame (excluded from tracebacks)
}

// ExpandBuiltinFrames inserts native continuation entries below their callback
// activations for language-visible stacks. The input remains the verb stack.
func ExpandBuiltinFrames(stack []ActivationFrame) []ActivationFrame {
	result := make([]ActivationFrame, 0, len(stack))
	for index, frame := range stack {
		if index > 0 && frame.BuiltinCaller != "" {
			result = append(result, ActivationFrame{
				This: ObjNothing, ThisValue: None, Programmer: ObjNothing,
				VerbLoc: ObjNothing, Player: frame.Player,
				Verb: frame.BuiltinCaller, StoredVerb: frame.BuiltinCaller,
				LineNumber:              frame.BuiltinPC,
				RuntimeVariables:        frame.RuntimeVariables,
				RuntimeVariableSnapshot: frame.RuntimeVariableSnapshot,
			})
		}
		frame.BuiltinCaller, frame.BuiltinPC = "", 0
		result = append(result, frame)
	}
	return result
}

// TracebackVerb returns the stored verb name spec without requiring callers to
// know whether the frame came from live execution or a persisted checkpoint.
func (a *ActivationFrame) TracebackVerb() string {
	if a.StoredVerb != "" {
		return a.StoredVerb
	}
	return strings.Join(a.StoredVerbNames, " ")
}

// ToList converts an activation frame to a MOO list for callers().
// Format: {this, verb_name, programmer, verb_loc, player, line_number}.
// For primitive/anonymous targets, ThisValue carries the real "this" value.
func (a *ActivationFrame) ToList() Value {
	thisVal := NewObj(a.This)
	if !a.ThisValue.IsNone() {
		thisVal = a.ThisValue
	}

	return NewList([]Value{
		thisVal,
		NewStr(a.Verb),
		NewObj(a.Programmer),
		NewObj(a.VerbLoc),
		NewObj(a.Player),
		NewInt(int64(a.LineNumber)),
	})
}

// ToMap converts an activation frame to a MOO map for task_stack().
// Keys: "this", "verb", "programmer", "verb_loc", "player", "line_number".
// For primitive prototype calls, "this" is #-1 (matching Toast).
func (a *ActivationFrame) ToMap() Value {
	return NewMap([][2]Value{
		{NewStr("this"), NewObj(a.This)}, // Always use object ID (#-1 for primitives)
		{NewStr("verb"), NewStr(a.Verb)},
		{NewStr("programmer"), NewObj(a.Programmer)},
		{NewStr("verb_loc"), NewObj(a.VerbLoc)},
		{NewStr("player"), NewObj(a.Player)},
		{NewStr("line_number"), NewInt(int64(a.LineNumber))},
	})
}
