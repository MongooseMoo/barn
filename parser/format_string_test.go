package parser

import (
	"errors"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/verb"
)

func TestFormatMOORejectsNULWithoutOutput(t *testing.T) {
	for _, value := range []string{"\x00", "nul\x00byte", "\x00\x00"} {
		bad := &verb.LiteralExpr{Kind: verb.LiteralString, StringValue: value}
		for _, expr := range []verb.Expr{bad, &verb.ListExpr{Elements: []verb.Expr{bad}},
			&verb.MapExpr{Pairs: []verb.MapPair{{Key: &verb.LiteralExpr{Kind: verb.LiteralString, StringValue: "key"}, Value: &verb.ListExpr{Elements: []verb.Expr{bad}}}}}} {
			program := &verb.Program{Statements: []verb.Stmt{
				&verb.ReturnStmt{Value: &verb.LiteralExpr{Kind: verb.LiteralInt, IntValue: 1}},
				&verb.IfStmt{Condition: &verb.LiteralExpr{Kind: verb.LiteralInt, IntValue: 1}, Body: []verb.Stmt{&verb.ReturnStmt{Value: expr}}},
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
	program := &verb.Program{Statements: []verb.Stmt{&verb.ReturnStmt{Value: &verb.LiteralExpr{Kind: verb.LiteralString, StringValue: value}}}}
	for _, fully := range []bool{false, true} {
		lines, err := formatMOOChecked(program, fully)
		if err != nil {
			t.Fatal(err)
		}
		reparsed, err := NewParser(strings.Join(lines, "\n")).ParseProgram()
		if err != nil {
			t.Fatal(err)
		}
		got := reparsed.Statements[0].(*verb.ReturnStmt).Value.(*verb.LiteralExpr).StringValue
		if got != value {
			t.Fatalf("byte preservation failed: got %q want %q", got, value)
		}
	}
}
