package parser

import (
	"github.com/MongooseMoo/barn/ir"
	"testing"
)

func parseExprForTest(t *testing.T, input string) ir.Expr {
	t.Helper()
	p := NewParser(input)
	expr, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		t.Fatalf("ParseExpression(%q) error = %v", input, err)
	}
	if p.current.Type != TOKEN_EOF {
		t.Fatalf("ParseExpression(%q) left trailing token %s", input, p.current.Type)
	}
	return expr
}

func parseLiteralForTest(t *testing.T, input string) *ir.LiteralExpr {
	t.Helper()
	expr := parseExprForTest(t, input)
	lit, ok := expr.(*ir.LiteralExpr)
	if !ok {
		t.Fatalf("ParseExpression(%q) returned %T, want *LiteralExpr", input, expr)
	}
	return lit
}
