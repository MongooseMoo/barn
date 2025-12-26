package parser

import "barn/types"

// Node is the base interface for all AST nodes
type Node interface {
	Position() Position
}

// Expr represents an expression node
type Expr interface {
	Node
	exprNode()
}

// Stmt represents a statement node
type Stmt interface {
	Node
	stmtNode()
}

// LiteralExpr wraps a Value from Phase 1
type LiteralExpr struct {
	Pos   Position
	Value types.Value
}

func (e *LiteralExpr) Position() Position { return e.Pos }
func (e *LiteralExpr) exprNode()          {}

// IdentifierExpr represents a variable reference
type IdentifierExpr struct {
	Pos  Position
	Name string
}

func (e *IdentifierExpr) Position() Position { return e.Pos }
func (e *IdentifierExpr) exprNode()          {}

// UnaryExpr represents a unary operation
type UnaryExpr struct {
	Pos      Position
	Operator TokenType // TOKEN_MINUS, TOKEN_NOT, TOKEN_BITNOT
	Operand  Expr
}

func (e *UnaryExpr) Position() Position { return e.Pos }
func (e *UnaryExpr) exprNode()          {}

// BinaryExpr represents a binary operation
type BinaryExpr struct {
	Pos      Position
	Left     Expr
	Operator TokenType
	Right    Expr
}

func (e *BinaryExpr) Position() Position { return e.Pos }
func (e *BinaryExpr) exprNode()          {}

// TernaryExpr represents conditional expression: cond ? then | else
type TernaryExpr struct {
	Pos       Position
	Condition Expr
	ThenExpr  Expr
	ElseExpr  Expr
}

func (e *TernaryExpr) Position() Position { return e.Pos }
func (e *TernaryExpr) exprNode()          {}

// ParenExpr represents a parenthesized expression
type ParenExpr struct {
	Pos  Position
	Expr Expr
}

func (e *ParenExpr) Position() Position { return e.Pos }
func (e *ParenExpr) exprNode()          {}

// IndexMarkerExpr represents special index markers: ^ (first) or $ (last)
type IndexMarkerExpr struct {
	Pos    Position
	Marker TokenType // TOKEN_CARET or TOKEN_DOLLAR
}

func (e *IndexMarkerExpr) Position() Position { return e.Pos }
func (e *IndexMarkerExpr) exprNode()          {}

// IndexExpr represents indexing: expr[index]
type IndexExpr struct {
	Pos   Position
	Expr  Expr
	Index Expr
}

func (e *IndexExpr) Position() Position { return e.Pos }
func (e *IndexExpr) exprNode()          {}

// RangeExpr represents range indexing: expr[start..end]
type RangeExpr struct {
	Pos   Position
	Expr  Expr
	Start Expr
	End   Expr
}

func (e *RangeExpr) Position() Position { return e.Pos }
func (e *RangeExpr) exprNode()          {}

// PropertyExpr represents property access: expr.property
type PropertyExpr struct {
	Pos      Position
	Expr     Expr
	Property string
}

func (e *PropertyExpr) Position() Position { return e.Pos }
func (e *PropertyExpr) exprNode()          {}

// VerbCallExpr represents verb call: expr:verb(args)
type VerbCallExpr struct {
	Pos  Position
	Expr Expr
	Verb string
	Args []Expr
}

func (e *VerbCallExpr) Position() Position { return e.Pos }
func (e *VerbCallExpr) exprNode()          {}

// BuiltinCallExpr represents builtin function call: func(args)
type BuiltinCallExpr struct {
	Pos  Position
	Name string
	Args []Expr
}

func (e *BuiltinCallExpr) Position() Position { return e.Pos }
func (e *BuiltinCallExpr) exprNode()          {}

// SpliceExpr represents splice operator: @expr
type SpliceExpr struct {
	Pos  Position
	Expr Expr
}

func (e *SpliceExpr) Position() Position { return e.Pos }
func (e *SpliceExpr) exprNode()          {}

// CatchExpr represents catch expression: expr `! codes => default
type CatchExpr struct {
	Pos     Position
	Expr    Expr
	Codes   []types.ErrorCode
	Default Expr
}

func (e *CatchExpr) Position() Position { return e.Pos }
func (e *CatchExpr) exprNode()          {}

// AssignExpr represents assignment: lvalue = expr
type AssignExpr struct {
	Pos    Position
	Target Expr // IdentifierExpr, IndexExpr, PropertyExpr, or RangeExpr
	Value  Expr
}

func (e *AssignExpr) Position() Position { return e.Pos }
func (e *AssignExpr) exprNode()          {}

// ListExpr represents a list expression: {expr, expr, ...}
// Unlike LiteralExpr with a list value, ListExpr can contain
// sub-expressions including splice (@expr) that must be evaluated.
type ListExpr struct {
	Pos      Position
	Elements []Expr
}

func (e *ListExpr) Position() Position { return e.Pos }
func (e *ListExpr) exprNode()          {}

// ListRangeExpr represents a range list expression: {start..end}
// This generates a list of integers from start to end (inclusive)
type ListRangeExpr struct {
	Pos   Position
	Start Expr
	End   Expr
}

func (e *ListRangeExpr) Position() Position { return e.Pos }
func (e *ListRangeExpr) exprNode()          {}

// MapExpr represents a map expression: [key -> value, ...]
// Unlike LiteralExpr with a map value, MapExpr can contain
// sub-expressions that must be evaluated.
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

// Statement AST nodes

// ExprStmt represents an expression used as a statement
type ExprStmt struct {
	Pos  Position
	Expr Expr
}

func (s *ExprStmt) Position() Position { return s.Pos }
func (s *ExprStmt) stmtNode()          {}

// IfStmt represents if/elseif/else/endif
type IfStmt struct {
	Pos       Position
	Condition Expr
	Body      []Stmt
	ElseIfs   []*ElseIfClause
	Else      []Stmt // Can be nil
}

type ElseIfClause struct {
	Pos       Position
	Condition Expr
	Body      []Stmt
}

func (s *IfStmt) Position() Position { return s.Pos }
func (s *IfStmt) stmtNode()          {}

// WhileStmt represents while loops
type WhileStmt struct {
	Pos       Position
	Label     string // Optional loop label for break/continue
	Condition Expr
	Body      []Stmt
}

func (s *WhileStmt) Position() Position { return s.Pos }
func (s *WhileStmt) stmtNode()          {}

// ForStmt represents for loops (list, range, or map iteration)
type ForStmt struct {
	Pos       Position
	Label     string // Optional loop label
	Value     string // Variable name for value
	Index     string // Variable name for index/key (optional)
	Container Expr   // List/map expression or nil for range
	RangeStart Expr  // For range loops: start expression
	RangeEnd   Expr  // For range loops: end expression
	Body      []Stmt
}

func (s *ForStmt) Position() Position { return s.Pos }
func (s *ForStmt) stmtNode()          {}

// BreakStmt represents break statement
// In MOO, break can optionally take an expression: break expr;
// This expression becomes the value of the loop
type BreakStmt struct {
	Pos   Position
	Label string // Optional loop label to break (only if Value is nil)
	Value Expr   // Optional value expression (loop evaluates to this)
}

func (s *BreakStmt) Position() Position { return s.Pos }
func (s *BreakStmt) stmtNode()          {}

// ContinueStmt represents continue statement
type ContinueStmt struct {
	Pos   Position
	Label string // Optional loop label to continue
}

func (s *ContinueStmt) Position() Position { return s.Pos }
func (s *ContinueStmt) stmtNode()          {}

// ReturnStmt represents return statement
type ReturnStmt struct {
	Pos   Position
	Value Expr // Can be nil (returns 0)
}

func (s *ReturnStmt) Position() Position { return s.Pos }
func (s *ReturnStmt) stmtNode()          {}

// TryExceptStmt represents try/except/endtry
type TryExceptStmt struct {
	Pos     Position
	Body    []Stmt
	Excepts []*ExceptClause
}

type ExceptClause struct {
	Pos      Position
	Variable string           // Optional: binds the caught error
	Codes    []types.ErrorCode // Error codes to catch (empty means ANY)
	IsAny    bool             // True if catching ANY
	Body     []Stmt
}

func (s *TryExceptStmt) Position() Position { return s.Pos }
func (s *TryExceptStmt) stmtNode()          {}

// TryFinallyStmt represents try/finally/endtry
type TryFinallyStmt struct {
	Pos     Position
	Body    []Stmt
	Finally []Stmt
}

func (s *TryFinallyStmt) Position() Position { return s.Pos }
func (s *TryFinallyStmt) stmtNode()          {}

// TryExceptFinallyStmt represents try/except/finally/endtry
type TryExceptFinallyStmt struct {
	Pos     Position
	Body    []Stmt
	Excepts []*ExceptClause
	Finally []Stmt
}

func (s *TryExceptFinallyStmt) Position() Position { return s.Pos }
func (s *TryExceptFinallyStmt) stmtNode()          {}

// ScatterStmt represents scatter assignment: {a, ?b, @rest} = list
type ScatterStmt struct {
	Pos     Position
	Targets []ScatterTarget
	Value   Expr
}

type ScatterTarget struct {
	Pos      Position
	Name     string
	Optional bool // ?var
	Rest     bool // @var
	Default  Expr // ?var = expr (can be nil)
}

func (s *ScatterStmt) Position() Position { return s.Pos }
func (s *ScatterStmt) stmtNode()          {}
