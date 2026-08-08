package parser

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/verb"
)

// TestFixF29_EIntrptParsesToCode18 asserts the full E_INTRPT round-trip: the
// parser accepts the literal, records the canonical name, and the name resolves
// to ToastStunt's numeric value (code 18 — structures.h:73, last element of
// enum error{}). Fails against the old code where isErrorName("E_INTRPT") was
// false and the parser rejected the literal.
func TestFixF29_EIntrptParsesToCode18(t *testing.T) {
	if !isErrorName("E_INTRPT") {
		t.Fatalf("BUG: isErrorName(\"E_INTRPT\") is false; E_INTRPT is a valid MOO error code")
	}

	stmts, err := NewParser("x = E_INTRPT;").ParseProgram()
	if err != nil {
		t.Fatalf("BUG: parser rejected E_INTRPT literal: %v", err)
	}

	assign, ok := stmts.Statements[0].(*verb.ExprStmt)
	if !ok {
		t.Fatalf("expected *verb.ExprStmt, got %T", stmts.Statements[0])
	}
	bin, ok := assign.Expr.(*verb.AssignExpr)
	if !ok {
		t.Fatalf("expected *AssignExpr, got %T", assign.Expr)
	}
	lit, ok := bin.Value.(*verb.LiteralExpr)
	if !ok {
		t.Fatalf("expected *LiteralExpr RHS, got %T", bin.Value)
	}
	if lit.Kind != verb.LiteralErr {
		t.Fatalf("expected LiteralErr, got kind %d", lit.Kind)
	}
	if lit.ErrorName != "E_INTRPT" {
		t.Fatalf("expected ErrorName %q, got %q", "E_INTRPT", lit.ErrorName)
	}

	// Numeric value must match Toast's enum: E_INTRPT == 18.
	code, ok := types.ErrorFromString(lit.ErrorName)
	if !ok {
		t.Fatalf("types.ErrorFromString(%q) returned not-ok", lit.ErrorName)
	}
	if code != types.E_INTRPT || int(code) != 18 {
		t.Fatalf("E_INTRPT resolved to %d, want 18", int(code))
	}
}
