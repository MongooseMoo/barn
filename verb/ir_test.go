package verb

import "testing"

func TestExpressionFamilyIsSealed(t *testing.T) {
	var _ Expr = &LiteralExpr{}
	var _ Expr = &IdentifierExpr{}
	var _ Expr = &UnaryExpr{}
	var _ Expr = &BinaryExpr{}
	var _ Expr = &TernaryExpr{}
	var _ Expr = &IndexBoundaryExpr{}
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

func TestNodePosition(t *testing.T) {
	pos := Position{Line: 2, Column: 10, Offset: 20}
	expr := &BinaryExpr{
		Pos:      pos,
		Left:     &LiteralExpr{Pos: pos, Kind: LiteralInt, IntValue: 1},
		Operator: BinaryAdd,
		Right:    &LiteralExpr{Pos: pos, Kind: LiteralInt, IntValue: 2},
	}
	if expr.Position() != pos {
		t.Errorf("Position() = %v, want %v", expr.Position(), pos)
	}
}
