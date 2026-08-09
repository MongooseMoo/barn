// Package verb defines the language-neutral semantic representation of a verb.
package verb

import (
	"errors"
	"reflect"
)

// MaxNestingDepth is the maximum number of nested semantic constructs accepted
// by every frontend and backend that recursively walks a verb program.
const MaxNestingDepth = 256

var ErrMaxNestingDepth = errors.New("maximum nesting depth exceeded (max 256)")

// ValidateNesting checks semantic IR without recursively walking attacker
// controlled data. Siblings retain the same depth, so wide programs are not
// penalized. Leaf nodes are allowed beneath the deepest construct.
func ValidateNesting(program *Program) error {
	return validateNesting(reflect.ValueOf(program), -2)
}

// ValidateNode applies the same limit to direct compiler inputs that do not
// pass through a Program.
func ValidateNode(node Node) error {
	return validateNesting(reflect.ValueOf(node), -1)
}

func validateNesting(root reflect.Value, initialDepth int) error {
	type item struct {
		value reflect.Value
		depth int
	}
	// The program container and its root statement are structural anchors, not
	// nested source constructs.
	stack := []item{{root, initialDepth}}
	nodeType := reflect.TypeOf((*Node)(nil)).Elem()
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		v := current.value
		if v.IsValid() && v.Type().Implements(nodeType) {
			current.depth++
			if current.depth > MaxNestingDepth {
				return ErrMaxNestingDepth
			}
		}
		for v.IsValid() && (v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer) {
			if v.IsNil() {
				v = reflect.Value{}
				break
			}
			v = v.Elem()
		}
		if !v.IsValid() {
			continue
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				f := v.Field(i)
				stack = append(stack, item{f, current.depth})
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				stack = append(stack, item{v.Index(i), current.depth})
			}
		}
	}
	return nil
}

// Position identifies a location in verb source.
type Position struct {
	Line   int
	Column int
	Offset int
}

// Node is the base interface for semantic verb nodes.
type Node interface {
	Position() Position
}

// Expr is the sealed family of semantic expressions.
type Expr interface {
	Node
	exprNode()
}

// Stmt is the sealed family of semantic statements.
type Stmt interface {
	Node
	stmtNode()
}

// Program is the semantic representation of one verb body.
type Program struct {
	Statements []Stmt
}

// LiteralKind identifies a semantic literal payload.
type LiteralKind int

const (
	LiteralInt LiteralKind = iota
	LiteralFloat
	LiteralString
	LiteralBool
	LiteralObj
	LiteralErr
)

// UnaryOperator identifies a semantic unary operation.
type UnaryOperator int

const (
	UnaryNegate UnaryOperator = iota
	UnaryNot
	UnaryBitwiseNot
)

func (op UnaryOperator) String() string {
	switch op {
	case UnaryNegate:
		return "negate"
	case UnaryNot:
		return "not"
	case UnaryBitwiseNot:
		return "bitwise-not"
	default:
		return "unknown-unary-operator"
	}
}

// BinaryOperator identifies a semantic binary operation.
type BinaryOperator int

const (
	BinaryAdd BinaryOperator = iota
	BinarySubtract
	BinaryMultiply
	BinaryDivide
	BinaryModulo
	BinaryPower
	BinaryEqual
	BinaryNotEqual
	BinaryLess
	BinaryLessEqual
	BinaryGreater
	BinaryGreaterEqual
	BinaryIn
	BinaryAnd
	BinaryOr
	BinaryBitAnd
	BinaryBitOr
	BinaryBitXor
	BinaryShiftLeft
	BinaryShiftRight
)

func (op BinaryOperator) String() string {
	switch op {
	case BinaryAdd:
		return "add"
	case BinarySubtract:
		return "subtract"
	case BinaryMultiply:
		return "multiply"
	case BinaryDivide:
		return "divide"
	case BinaryModulo:
		return "modulo"
	case BinaryPower:
		return "power"
	case BinaryEqual:
		return "equal"
	case BinaryNotEqual:
		return "not-equal"
	case BinaryLess:
		return "less"
	case BinaryLessEqual:
		return "less-equal"
	case BinaryGreater:
		return "greater"
	case BinaryGreaterEqual:
		return "greater-equal"
	case BinaryIn:
		return "in"
	case BinaryAnd:
		return "and"
	case BinaryOr:
		return "or"
	case BinaryBitAnd:
		return "bitwise-and"
	case BinaryBitOr:
		return "bitwise-or"
	case BinaryBitXor:
		return "bitwise-xor"
	case BinaryShiftLeft:
		return "shift-left"
	case BinaryShiftRight:
		return "shift-right"
	default:
		return "unknown-binary-operator"
	}
}

// IndexBoundary identifies a semantic first- or last-index operation.
type IndexBoundary int

const (
	IndexFirst IndexBoundary = iota
	IndexLast
)

type LiteralExpr struct {
	Pos         Position
	Kind        LiteralKind
	IntValue    int64
	FloatValue  float64
	StringValue string
	BoolValue   bool
	ObjID       int64
	ErrorName   string
}

func (e *LiteralExpr) Position() Position { return e.Pos }
func (e *LiteralExpr) exprNode()          {}

type IdentifierExpr struct {
	Pos  Position
	Name string
}

func (e *IdentifierExpr) Position() Position { return e.Pos }
func (e *IdentifierExpr) exprNode()          {}

type UnaryExpr struct {
	Pos      Position
	Operator UnaryOperator
	Operand  Expr
}

func (e *UnaryExpr) Position() Position { return e.Pos }
func (e *UnaryExpr) exprNode()          {}

type BinaryExpr struct {
	Pos      Position
	Left     Expr
	Operator BinaryOperator
	Right    Expr
}

func (e *BinaryExpr) Position() Position { return e.Pos }
func (e *BinaryExpr) exprNode()          {}

type TernaryExpr struct {
	Pos       Position
	Condition Expr
	ThenExpr  Expr
	ElseExpr  Expr
}

func (e *TernaryExpr) Position() Position { return e.Pos }
func (e *TernaryExpr) exprNode()          {}

type IndexBoundaryExpr struct {
	Pos      Position
	Boundary IndexBoundary
}

func (e *IndexBoundaryExpr) Position() Position { return e.Pos }
func (e *IndexBoundaryExpr) exprNode()          {}

type IndexExpr struct {
	Pos   Position
	Expr  Expr
	Index Expr
}

func (e *IndexExpr) Position() Position { return e.Pos }
func (e *IndexExpr) exprNode()          {}

type RangeExpr struct {
	Pos   Position
	Expr  Expr
	Start Expr
	End   Expr
}

func (e *RangeExpr) Position() Position { return e.Pos }
func (e *RangeExpr) exprNode()          {}

type PropertyExpr struct {
	Pos          Position
	Expr         Expr
	Property     string
	PropertyExpr Expr
}

func (e *PropertyExpr) Position() Position { return e.Pos }
func (e *PropertyExpr) exprNode()          {}

type VerbCallExpr struct {
	Pos      Position
	Expr     Expr
	Verb     string
	VerbExpr Expr
	Args     []Expr
}

func (e *VerbCallExpr) Position() Position { return e.Pos }
func (e *VerbCallExpr) exprNode()          {}

type BuiltinCallExpr struct {
	Pos  Position
	Name string
	Args []Expr
}

func (e *BuiltinCallExpr) Position() Position { return e.Pos }
func (e *BuiltinCallExpr) exprNode()          {}

type SpliceExpr struct {
	Pos  Position
	Expr Expr
}

func (e *SpliceExpr) Position() Position { return e.Pos }
func (e *SpliceExpr) exprNode()          {}

type CatchExpr struct {
	Pos     Position
	Expr    Expr
	Codes   []string
	IsAny   bool
	Default Expr
}

func (e *CatchExpr) Position() Position { return e.Pos }
func (e *CatchExpr) exprNode()          {}

type AssignExpr struct {
	Pos    Position
	Target Target
	Value  Expr
}

func (e *AssignExpr) Position() Position { return e.Pos }
func (e *AssignExpr) exprNode()          {}

type Target interface {
	Node
	targetNode()
}

type CollectionTarget interface {
	Target
	collectionTargetNode()
}

type VariableTarget struct {
	Pos  Position
	Name string
}

func (t *VariableTarget) Position() Position    { return t.Pos }
func (t *VariableTarget) targetNode()           {}
func (t *VariableTarget) collectionTargetNode() {}

type PropertyTarget struct {
	Pos      Position
	Object   Expr
	Name     string
	NameExpr Expr
}

func (t *PropertyTarget) Position() Position    { return t.Pos }
func (t *PropertyTarget) targetNode()           {}
func (t *PropertyTarget) collectionTargetNode() {}

type IndexTarget struct {
	Pos        Position
	Collection CollectionTarget
	Index      Expr
}

func (t *IndexTarget) Position() Position    { return t.Pos }
func (t *IndexTarget) targetNode()           {}
func (t *IndexTarget) collectionTargetNode() {}

type RangeTarget struct {
	Pos        Position
	Collection CollectionTarget
	Start      Expr
	End        Expr
}

func (t *RangeTarget) Position() Position { return t.Pos }
func (t *RangeTarget) targetNode()        {}

type DestructuringTarget struct {
	Pos      Position
	Bindings []Binding
}

func (t *DestructuringTarget) Position() Position { return t.Pos }
func (t *DestructuringTarget) targetNode()        {}

type Binding interface {
	Node
	bindingNode()
}

type RequiredBinding struct {
	Pos  Position
	Name string
}

func (b *RequiredBinding) Position() Position { return b.Pos }
func (b *RequiredBinding) bindingNode()       {}

type OptionalBinding struct {
	Pos     Position
	Name    string
	Default Expr
}

func (b *OptionalBinding) Position() Position { return b.Pos }
func (b *OptionalBinding) bindingNode()       {}

type RestBinding struct {
	Pos  Position
	Name string
}

func (b *RestBinding) Position() Position { return b.Pos }
func (b *RestBinding) bindingNode()       {}

type ListExpr struct {
	Pos      Position
	Elements []Expr
}

func (e *ListExpr) Position() Position { return e.Pos }
func (e *ListExpr) exprNode()          {}

type ListRangeExpr struct {
	Pos   Position
	Start Expr
	End   Expr
}

func (e *ListRangeExpr) Position() Position { return e.Pos }
func (e *ListRangeExpr) exprNode()          {}

type MapExpr struct {
	Pos   Position
	Pairs []MapPair
}

type MapPair struct {
	Key   Expr
	Value Expr
}

func (e *MapExpr) Position() Position { return e.Pos }
func (e *MapExpr) exprNode()          {}

type ExprStmt struct {
	Pos  Position
	Expr Expr
}

func (s *ExprStmt) Position() Position { return s.Pos }
func (s *ExprStmt) stmtNode()          {}

type IfStmt struct {
	Pos       Position
	Condition Expr
	Body      []Stmt
	Else      []Stmt
}

func (s *IfStmt) Position() Position { return s.Pos }
func (s *IfStmt) stmtNode()          {}

type WhileStmt struct {
	Pos       Position
	Label     string
	Condition Expr
	Body      []Stmt
}

func (s *WhileStmt) Position() Position { return s.Pos }
func (s *WhileStmt) stmtNode()          {}

type CollectionLoopStmt struct {
	Pos        Position
	Label      string
	Value      string
	Index      string
	Collection Expr
	Body       []Stmt
}

func (s *CollectionLoopStmt) Position() Position { return s.Pos }
func (s *CollectionLoopStmt) stmtNode()          {}

type RangeLoopStmt struct {
	Pos   Position
	Label string
	Value string
	Start Expr
	End   Expr
	Body  []Stmt
}

func (s *RangeLoopStmt) Position() Position { return s.Pos }
func (s *RangeLoopStmt) stmtNode()          {}

type BreakStmt struct {
	Pos   Position
	Label string
}

func (s *BreakStmt) Position() Position { return s.Pos }
func (s *BreakStmt) stmtNode()          {}

type ContinueStmt struct {
	Pos   Position
	Label string
}

func (s *ContinueStmt) Position() Position { return s.Pos }
func (s *ContinueStmt) stmtNode()          {}

type ReturnStmt struct {
	Pos   Position
	Value Expr
}

func (s *ReturnStmt) Position() Position { return s.Pos }
func (s *ReturnStmt) stmtNode()          {}

type TryStmt struct {
	Pos       Position
	Body      []Stmt
	Handlers  []ExceptionHandler
	Finalizer *Finalizer
}

type ExceptionHandler struct {
	Pos      Position
	Variable string
	Codes    []string
	IsAny    bool
	Body     []Stmt
}

type Finalizer struct {
	Pos  Position
	Body []Stmt
}

func (s *TryStmt) Position() Position { return s.Pos }
func (s *TryStmt) stmtNode()          {}

type ForkStmt struct {
	Pos     Position
	Delay   Expr
	VarName string
	Body    []Stmt
}

func (s *ForkStmt) Position() Position { return s.Pos }
func (s *ForkStmt) stmtNode()          {}
