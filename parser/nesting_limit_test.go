package parser_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/parser"
)

func TestNestingLimitUnaryAndParentheses(t *testing.T) {
	for _, depth := range []int{255, 256, 257, 2560} {
		for name, source := range map[string]string{
			"unary":       strings.Repeat("!", depth) + "1;",
			"parentheses": strings.Repeat("(", depth) + "1" + strings.Repeat(")", depth) + ";",
			"list":        strings.Repeat("{", depth) + "1" + strings.Repeat("}", depth) + ";",
			"map":         strings.Repeat("[1 -> ", depth) + "1" + strings.Repeat("]", depth) + ";",
			"power":       strings.Repeat("1 ^ ", depth) + "1;",
			"assignment":  strings.Repeat("a = ", depth) + "1;",
			"additive":    "1" + strings.Repeat(" + 1", depth) + ";",
			"postfix":     "a" + strings.Repeat("[1]", depth) + ";",
			"property":    "a" + strings.Repeat(".b", depth) + ";",
			"if":          strings.Repeat("if (1)\n", depth) + "return 1;\n" + strings.Repeat("endif\n", depth),
			"while":       strings.Repeat("while (0)\n", depth) + "return 1;\n" + strings.Repeat("endwhile\n", depth),
			"try":         strings.Repeat("try\n", depth) + "return 1;\n" + strings.Repeat("except (ANY)\nendtry\n", depth),
			"elseif":      "if (1)\n" + strings.Repeat("elseif (1)\n", depth-1) + "return 1;\nendif",
		} {
			t.Run(fmt.Sprintf("%s/%d", name, depth), func(t *testing.T) {
				_, err := parser.NewParser(source).ParseProgram()
				if depth <= ir.MaxNestingDepth && err != nil {
					t.Fatalf("depth %d: %v", depth, err)
				}
				if depth > ir.MaxNestingDepth && !errors.Is(err, ir.ErrMaxNestingDepth) {
					t.Fatalf("depth %d error = %v", depth, err)
				}
			})
		}
	}
}

func TestFormatMOOCheckedRejectsDeepDirectIR(t *testing.T) {
	var expr ir.Expr = &ir.LiteralExpr{Kind: ir.LiteralInt}
	for range ir.MaxNestingDepth + 1 {
		expr = &ir.UnaryExpr{Operand: expr}
	}
	program := &ir.Program{Statements: []ir.Stmt{&ir.ExprStmt{Expr: expr}}}
	lines, err := parser.FormatMOOChecked(program)
	if !errors.Is(err, ir.ErrMaxNestingDepth) || lines != nil {
		t.Fatalf("FormatMOOChecked() = (%v, %v)", lines, err)
	}
}

func TestNestingLimitDoesNotCountSiblings(t *testing.T) {
	program := &ir.Program{Statements: make([]ir.Stmt, 4096)}
	for i := range program.Statements {
		program.Statements[i] = &ir.ExprStmt{}
	}
	if err := ir.ValidateNesting(program); err != nil {
		t.Fatal(err)
	}
}

func TestNestingLimitChecksDirectIRBoundariesAndBothFormatters(t *testing.T) {
	for _, depth := range []int{255, 256, 257, 2560} {
		var expr ir.Expr = &ir.LiteralExpr{Kind: ir.LiteralInt}
		for range depth {
			expr = &ir.UnaryExpr{Operator: ir.UnaryNot, Operand: expr}
		}
		program := &ir.Program{Statements: []ir.Stmt{&ir.ReturnStmt{Value: expr}}}
		for _, formatter := range []func(*ir.Program) ([]string, error){parser.FormatMOOChecked, parser.FormatMOOFullyParenthesizedChecked} {
			lines, err := formatter(program)
			if depth <= ir.MaxNestingDepth {
				if err != nil || len(lines) != 1 {
					t.Fatalf("direct depth %d: %v / %v", depth, lines, err)
				}
			} else if lines != nil || !errors.Is(err, ir.ErrMaxNestingDepth) {
				t.Fatalf("direct depth %d: %v / %v", depth, lines, err)
			}
		}
	}
}

func TestNestingBudgetDoesNotAccumulateSequentialSourceOrListElements(t *testing.T) {
	for name, source := range map[string]string{
		"statements": strings.Repeat(strings.Repeat("(", 16)+"1"+strings.Repeat(")", 16)+";\n", 4096),
		"elements":   "{" + strings.Repeat("1,", 4095) + "1};",
	} {
		program, err := parser.NewParser(source).ParseProgram()
		if err != nil || program == nil {
			t.Fatalf("wide %s: %v / %v", name, program, err)
		}
	}
}
