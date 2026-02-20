package types

import (
	"time"
)

// ControlFlow represents the control flow state of evaluation
type ControlFlow int

const (
	FlowNormal     ControlFlow = iota // Normal execution
	FlowReturn                        // Return statement
	FlowBreak                         // Break statement
	FlowContinue                      // Continue statement
	FlowException                     // MOO error being raised
	FlowFork                          // Fork statement executed
	FlowSuspend                       // Suspend statement executed
	FlowParseError                    // Parse/syntax error (Val contains error message list)
)

// ForkInfo contains information needed to create a forked task
// Note: Body is []interface{} to avoid circular dependency with parser package
type ForkInfo struct {
	Body        interface{}       // []parser.Stmt - Fork body to execute
	SourceLines []string          // Original source lines (for database serialization)
	Delay       time.Duration     // Delay before execution
	VarName     string            // Variable to store task ID (empty = anonymous)
	Variables   map[string]Value  // Deep copy of variable environment
	ThisObj     ObjID             // this context
	Player      ObjID             // player context
	Caller      ObjID             // caller context
	Verb        string            // verb context
	VerbLoc     ObjID             // object where the enclosing verb is defined
}

// Result represents the outcome of evaluating an expression or statement
// This unifies normal values, control flow (return/break/continue), and errors
type Result struct {
	Val       Value       // The value (if Flow == FlowNormal or FlowReturn)
	Flow      ControlFlow // Control flow state
	Error     ErrorCode   // Only set when Flow == FlowException
	Label     string      // Loop label for break/continue (empty = innermost loop)
	ForkInfo  *ForkInfo   // Only set when Flow == FlowFork
	CallStack interface{} // []task.ActivationFrame - only set on exception from synchronous verb calls
}

// Ok creates a Result for normal execution with a value
func Ok(v Value) Result {
	return Result{Val: v, Flow: FlowNormal}
}

// Return creates a Result for a return statement
func Return(v Value) Result {
	return Result{Val: v, Flow: FlowReturn}
}

// Ret creates a Result for a return statement (alias for backward compatibility)
func Ret(v Value) Result {
	return Return(v)
}

// Err creates a Result for an error/exception
func Err(e ErrorCode) Result {
	return Result{Flow: FlowException, Error: e}
}

// Break creates a Result for a break statement
// The value, if non-nil, becomes the value of the enclosing loop
func Break(label string, val Value) Result {
	return Result{Flow: FlowBreak, Label: label, Val: val}
}

// Continue creates a Result for a continue statement
func Continue(label string) Result {
	return Result{Flow: FlowContinue, Label: label}
}

// Fork creates a Result for a fork statement
func Fork(info *ForkInfo) Result {
	return Result{Flow: FlowFork, ForkInfo: info}
}

// Suspend creates a Result for a suspend statement
// The Val field contains the seconds to suspend (0 = indefinite)
func Suspend(seconds float64) Result {
	return Result{Flow: FlowSuspend, Val: NewFloat(seconds)}
}

// IsNormal returns true if this is normal execution
func (r Result) IsNormal() bool {
	return r.Flow == FlowNormal
}

// IsError returns true if this is an exception
func (r Result) IsError() bool {
	return r.Flow == FlowException
}

// IsReturn returns true if this is a return statement
func (r Result) IsReturn() bool {
	return r.Flow == FlowReturn
}

// IsBreak returns true if this is a break statement
func (r Result) IsBreak() bool {
	return r.Flow == FlowBreak
}

// IsContinue returns true if this is a continue statement
func (r Result) IsContinue() bool {
	return r.Flow == FlowContinue
}

// IsFork returns true if this is a fork statement
func (r Result) IsFork() bool {
	return r.Flow == FlowFork
}
