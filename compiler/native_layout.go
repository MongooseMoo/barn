package compiler

import "github.com/MongooseMoo/barn/ir"

// NativeValueKind describes one Toast operand's semantic origin. Descriptions
// are compile-time metadata, never instructions to reevaluate an expression.
type NativeValueKind uint8

const (
	NativeExpression NativeValueKind = iota
	NativePropertyName
	NativeListPrefix
	NativeMapPrefix
	NativeLoopContainer
	NativeLoopCursor
	NativeRangeNext
	NativeRangeLimit
	NativeExceptionCodes
	NativeHandlerLabel
	NativeCatchMarker
	NativeFinallyMarker
	NativeFinallyValue
	NativeFinallyReason
)

type NativeValueOrigin struct {
	Kind    NativeValueKind
	Node    ir.Node
	Aux     int
	LabelPC int
}

type NativeControlKind uint8

const (
	NativeCatch NativeControlKind = iota
	NativeExcept
	NativeFinallyBody
	NativeFinallyHandler
	NativeCollectionLoop
	NativeRangeLoop
	NativeWhileLoop
)

type NativeControlOrigin struct {
	Kind       NativeControlKind
	Node       ir.Node
	StackBase  int
	HandlerPCs []int
}

type NativeContinuation struct {
	Node     ir.Node
	Vector   int
	PC       int
	Stack    []NativeValueOrigin
	Controls []NativeControlOrigin
	// Cleared names the variables Toast holds unbound at this call, because
	// an OP_PUSH_CLEAR in the call's block unbound them (nativeVector.clearedAt).
	// ClearUnknown is set when that depends on the path into the block.
	Cleared      []string
	ClearUnknown bool
}

type NativeVectorLayout struct {
	Vector        int
	Body          []ir.Stmt
	Size          int
	MaxStack      int
	LiteralWidth  int
	ForkWidth     int
	VariableWidth int
	LabelWidth    int
	StackWidth    int
	Continuations []NativeContinuation
	Labels        []NativeLabelOrigin
}

type NativeLabelKind uint8

const (
	NativeBranchElse NativeLabelKind = iota
	NativeBranchEnd
	NativeLoopTop
	NativeLoopEnd
	NativeHandler
	NativeHandlerEnd
	NativeScatterDefault
	NativeScatterEnd
)

type NativeLabelOrigin struct {
	PC   int
	Node ir.Node
	Kind NativeLabelKind
	Aux  int
}

type NativeProgramLayout struct {
	VariableNames []string
	Vectors       []NativeVectorLayout
}
