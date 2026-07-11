// Package verb defines the language-neutral semantic representation of a verb.
package verb

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
	Target Expr
	Value  Expr
}

func (e *AssignExpr) Position() Position { return e.Pos }
func (e *AssignExpr) exprNode()          {}

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

type ScatterStmt struct {
	Pos     Position
	Targets []ScatterTarget
	Value   Expr
}

type ScatterTarget struct {
	Pos      Position
	Name     string
	Optional bool
	Rest     bool
	Default  Expr
}

func (s *ScatterStmt) Position() Position { return s.Pos }
func (s *ScatterStmt) stmtNode()          {}

type ForkStmt struct {
	Pos     Position
	Delay   Expr
	VarName string
	Body    []Stmt
}

func (s *ForkStmt) Position() Position { return s.Pos }
func (s *ForkStmt) stmtNode()          {}
