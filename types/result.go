package types

// ControlFlow represents the control flow state of evaluation
type ControlFlow int

const (
	FlowNormal    ControlFlow = iota // Normal execution
	FlowReturn                        // Return statement
	FlowBreak                         // Break statement
	FlowContinue                      // Continue statement
	FlowException                     // MOO error being raised
)

// Result represents the outcome of evaluating an expression or statement
// This unifies normal values, control flow (return/break/continue), and errors
type Result struct {
	Val   Value       // The value (if Flow == FlowNormal or FlowReturn)
	Flow  ControlFlow // Control flow state
	Error ErrorCode   // Only set when Flow == FlowException
	Label string      // Loop label for break/continue (empty = innermost loop)
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
func Break(label string) Result {
	return Result{Flow: FlowBreak, Label: label}
}

// Continue creates a Result for a continue statement
func Continue(label string) Result {
	return Result{Flow: FlowContinue, Label: label}
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
