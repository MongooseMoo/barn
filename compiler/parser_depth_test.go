package compiler

import (
	"errors"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/sourcekey"
	"github.com/MongooseMoo/barn/verb"
)

func TestCompileParserDepthLimitDiagnosticsAndCache(t *testing.T) {
	for _, depth := range []int{255, 256, 257, 2560} {
		c := New(nil)
		lines := []string{"return " + strings.Repeat("!", depth) + "1;"}
		program, diagnostics := c.CompileMOO(lines)
		if depth <= verb.MaxNestingDepth {
			if program == nil || len(diagnostics) != 0 {
				t.Fatalf("depth %d: %v / %v", depth, program, diagnostics)
			}
			continue
		}
		if program != nil || len(diagnostics) != 1 || diagnostics[0].Stage != SyntaxStage || diagnostics[0].Position.Line != 1 || diagnostics[0].Message != "syntax error" || !errors.Is(diagnostics[0].Detail, verb.ErrMaxNestingDepth) {
			t.Fatalf("depth %d: %v / %v", depth, program, diagnostics)
		}
		if diagnostics[0].Detail.Error() != "maximum nesting depth exceeded (max 256)" {
			t.Fatal("limit detail changed")
		}
		if _, cached := c.cache.get(sourcekey.Of(lines)); cached {
			t.Fatal("over-limit source entered cache")
		}
	}
}

func TestBytecodeEntryValidatesDirectIRDepthBoundaries(t *testing.T) {
	for _, depth := range []int{255, 256, 257, 2560} {
		var expr verb.Expr = &verb.LiteralExpr{Kind: verb.LiteralInt}
		for range depth {
			expr = &verb.UnaryExpr{Operator: verb.UnaryNot, Operand: expr}
		}
		program, err := newLowerer(nil).compileProgram(&verb.Program{Statements: []verb.Stmt{&verb.ReturnStmt{Value: expr}}})
		if depth <= verb.MaxNestingDepth {
			if program == nil || err != nil {
				t.Fatalf("direct depth %d: %v / %v", depth, program, err)
			}
		} else if program != nil || !errors.Is(err, verb.ErrMaxNestingDepth) {
			t.Fatalf("direct depth %d: %v / %v", depth, program, err)
		}
	}
}
