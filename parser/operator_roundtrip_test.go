package parser_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/parser"
)

func TestFormatMOOPreservesEveryBinaryTree(t *testing.T) {
	for parent := ir.BinaryAdd; parent <= ir.BinaryShiftRight; parent++ {
		for child := ir.BinaryAdd; child <= ir.BinaryShiftRight; child++ {
			for _, childOnLeft := range []bool{true, false} {
				name := parent.String() + "/" + child.String()
				if childOnLeft {
					name += "/left"
				} else {
					name += "/right"
				}
				t.Run(name, func(t *testing.T) {
					childExpr := binary(child, identifier("a"), identifier("b"))
					var root *ir.BinaryExpr
					if childOnLeft {
						root = binary(parent, childExpr, identifier("c"))
					} else {
						root = binary(parent, identifier("a"), childExpr)
					}
					assertDirectProgramFormatRoundTrip(t, expressionProgram(root))
				})
			}
		}
	}
}

func assertDirectProgramFormatRoundTrip(t *testing.T, program *ir.Program) {
	t.Helper()
	want := withoutPositions(reflect.ValueOf(program)).Interface()
	lines, err := parser.FormatMOOChecked(program)
	if err != nil {
		t.Fatalf("FormatMOOChecked() error = %v", err)
	}
	canonical := strings.Join(lines, "\n")
	assertFormattedProgram(t, canonical, want, canonical)

	lines, err = parser.FormatMOOFullyParenthesizedChecked(program)
	if err != nil {
		t.Fatalf("FormatMOOFullyParenthesizedChecked() error = %v", err)
	}
	fullyParenthesized := strings.Join(lines, "\n")
	assertFormattedProgram(t, fullyParenthesized, want, canonical)
}

func assertFormattedProgram(t *testing.T, source string, want any, wantCanonical string) {
	t.Helper()
	reparsed, err := parser.NewParser(source).ParseProgram()
	if err != nil {
		t.Fatalf("ParseProgram(%q) error = %v", source, err)
	}
	got := withoutPositions(reflect.ValueOf(reparsed)).Interface()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("semantic IR changed after formatting\nformatted: %s\ngot:  %#v\nwant: %#v", source, got, want)
	}
	if stable := strings.Join(parser.FormatMOO(reparsed), "\n"); stable != wantCanonical {
		t.Fatalf("formatter is unstable\nfirst:  %s\nsecond: %s", source, stable)
	}
}

func expressionProgram(expr ir.Expr) *ir.Program {
	return &ir.Program{Statements: []ir.Stmt{&ir.ExprStmt{Expr: expr}}}
}

func binary(operator ir.BinaryOperator, left, right ir.Expr) *ir.BinaryExpr {
	return &ir.BinaryExpr{Left: left, Operator: operator, Right: right}
}

func identifier(name string) *ir.IdentifierExpr { return &ir.IdentifierExpr{Name: name} }
