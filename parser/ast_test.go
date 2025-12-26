package parser

import (
	"barn/types"
	"testing"
)

func TestASTNodes(t *testing.T) {
	// Test that all AST nodes implement the Expr interface
	var _ Expr = &LiteralExpr{}
	var _ Expr = &IdentifierExpr{}
	var _ Expr = &UnaryExpr{}
	var _ Expr = &BinaryExpr{}
	var _ Expr = &TernaryExpr{}
	var _ Expr = &ParenExpr{}
	var _ Expr = &IndexExpr{}
	var _ Expr = &RangeExpr{}
	var _ Expr = &PropertyExpr{}
	var _ Expr = &VerbCallExpr{}
	var _ Expr = &BuiltinCallExpr{}
	var _ Expr = &SpliceExpr{}
	var _ Expr = &CatchExpr{}
	var _ Expr = &AssignExpr{}
	var _ Expr = &ListExpr{}
	var _ Expr = &ListRangeExpr{}
	var _ Expr = &MapExpr{}
}

func TestLiteralExprPosition(t *testing.T) {
	pos := Position{Line: 1, Column: 5, Offset: 10}
	expr := &LiteralExpr{
		Pos:   pos,
		Value: types.NewInt(42),
	}
	if expr.Position() != pos {
		t.Errorf("Position() = %v, want %v", expr.Position(), pos)
	}
}

func TestBinaryExprPosition(t *testing.T) {
	pos := Position{Line: 2, Column: 10, Offset: 20}
	expr := &BinaryExpr{
		Pos:      pos,
		Left:     &LiteralExpr{Pos: pos, Value: types.NewInt(1)},
		Operator: TOKEN_PLUS,
		Right:    &LiteralExpr{Pos: pos, Value: types.NewInt(2)},
	}
	if expr.Position() != pos {
		t.Errorf("Position() = %v, want %v", expr.Position(), pos)
	}
}
