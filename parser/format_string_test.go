package parser

import (
	"errors"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/ir"
)

func TestFormatMOORejectsNULWithoutOutput(t *testing.T) {
	for _, value := range []string{"\x00", "nul\x00byte", "\x00\x00"} {
		bad := &ir.LiteralExpr{Kind: ir.LiteralString, StringValue: value}
		for _, expr := range []ir.Expr{bad, &ir.ListExpr{Elements: []ir.Expr{bad}},
			&ir.MapExpr{Pairs: []ir.MapPair{{Key: &ir.LiteralExpr{Kind: ir.LiteralString, StringValue: "key"}, Value: &ir.ListExpr{Elements: []ir.Expr{bad}}}}}} {
			program := &ir.Program{Statements: []ir.Stmt{
				&ir.ReturnStmt{Value: &ir.LiteralExpr{Kind: ir.LiteralInt, IntValue: 1}},
				&ir.IfStmt{Condition: &ir.LiteralExpr{Kind: ir.LiteralInt, IntValue: 1}, Body: []ir.Stmt{&ir.ReturnStmt{Value: expr}}},
			}}
			for _, fully := range []bool{false, true} {
				lines, err := formatMOOChecked(program, fully)
				if lines != nil || !errors.Is(err, ErrNULInSource) || err.Error() != "NUL byte is not representable in MOO source" {
					t.Fatalf("fully=%t lines=%q error=%v; want deterministic rejection without prefix output", fully, lines, err)
				}
			}
			if lines, err := FormatMOOChecked(program); lines != nil || err == nil {
				t.Fatalf("checked: lines=%q err=%v", lines, err)
			}
			if lines := FormatMOO(program); lines != nil {
				t.Fatalf("unchecked returned partial source: %q", lines)
			}
			if lines := FormatMOOFullyParenthesized(program); lines != nil {
				t.Fatalf("fully parenthesized returned partial source: %q", lines)
			}
		}
	}
}

func TestFormatMOOPreservesAllNonNULBytes(t *testing.T) {
	var bytes []byte
	for i := 1; i <= 255; i++ {
		bytes = append(bytes, byte(i))
	}
	value := string(bytes)
	program := &ir.Program{Statements: []ir.Stmt{&ir.ReturnStmt{Value: &ir.LiteralExpr{Kind: ir.LiteralString, StringValue: value}}}}
	for _, fully := range []bool{false, true} {
		lines, err := formatMOOChecked(program, fully)
		if err != nil {
			t.Fatal(err)
		}
		reparsed, err := NewParser(strings.Join(lines, "\n")).ParseProgram()
		if err != nil {
			t.Fatal(err)
		}
		got := reparsed.Statements[0].(*ir.ReturnStmt).Value.(*ir.LiteralExpr).StringValue
		if got != value {
			t.Fatalf("byte preservation failed: got %q want %q", got, value)
		}
		again, err := formatMOOChecked(reparsed, fully)
		if err != nil || strings.Join(again, "\n") != strings.Join(lines, "\n") {
			t.Fatalf("byte-preserving formatter is not idempotent: first %q, second %q, error %v", lines, again, err)
		}
	}
}
