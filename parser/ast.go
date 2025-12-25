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
