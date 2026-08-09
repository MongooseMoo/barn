package parser_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/parser"
	"github.com/MongooseMoo/barn/verb"
)

func TestNestingLimitUnaryAndParentheses(t *testing.T) {
	for _, depth := range []int{255, 256, 257, 2560} {
		for name, source := range map[string]string{
			"unary":       strings.Repeat("!", depth) + "1;",
			"parentheses": strings.Repeat("(", depth) + "1" + strings.Repeat(")", depth) + ";",
		} {
			t.Run(name+"/"+strings.Repeat("x", depth/100), func(t *testing.T) {
				_, err := parser.NewParser(source).ParseProgram()
				if depth <= verb.MaxNestingDepth && err != nil {
					t.Fatalf("depth %d: %v", depth, err)
				}
				if depth > verb.MaxNestingDepth && !errors.Is(err, verb.ErrMaxNestingDepth) {
					t.Fatalf("depth %d error = %v", depth, err)
				}
			})
		}
	}
}

func TestFormatMOOCheckedRejectsDeepDirectIR(t *testing.T) {
	var expr verb.Expr = &verb.LiteralExpr{Kind: verb.LiteralInt}
	for range verb.MaxNestingDepth + 1 {
		expr = &verb.UnaryExpr{Operand: expr}
	}
	program := &verb.Program{Statements: []verb.Stmt{&verb.ExprStmt{Expr: expr}}}
	lines, err := parser.FormatMOOChecked(program)
	if !errors.Is(err, verb.ErrMaxNestingDepth) || lines != nil {
		t.Fatalf("FormatMOOChecked() = (%v, %v)", lines, err)
	}
}

func TestNestingLimitDoesNotCountSiblings(t *testing.T) {
	program := &verb.Program{Statements: make([]verb.Stmt, 4096)}
	for i := range program.Statements {
		program.Statements[i] = &verb.ExprStmt{}
	}
	if err := verb.ValidateNesting(program); err != nil {
		t.Fatal(err)
	}
}
