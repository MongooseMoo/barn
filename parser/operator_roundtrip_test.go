package parser_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/parser"
)

func TestFormatMOOPreservesEverySamePrecedenceBinaryTree(t *testing.T) {
	groups := [][]ir.BinaryOperator{
		{ir.BinaryOr, ir.BinaryAnd},
		{ir.BinaryBitOr, ir.BinaryBitAnd, ir.BinaryBitXor},
		{ir.BinaryEqual, ir.BinaryNotEqual, ir.BinaryLess, ir.BinaryLessEqual, ir.BinaryGreater, ir.BinaryGreaterEqual, ir.BinaryIn},
		{ir.BinaryShiftLeft, ir.BinaryShiftRight},
		{ir.BinaryAdd, ir.BinarySubtract},
		{ir.BinaryMultiply, ir.BinaryDivide, ir.BinaryModulo},
		{ir.BinaryPower},
	}

	for _, group := range groups {
		for _, parent := range group {
			for _, child := range group {
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
}

func assertDirectProgramFormatRoundTrip(t *testing.T, program *ir.Program) {
	t.Helper()
	want := withoutPositions(reflect.ValueOf(program)).Interface()
	canonical := strings.Join(parser.FormatMOO(program), "\n")
	assertFormattedProgram(t, canonical, want, canonical)

	fullyParenthesized := strings.Join(parser.FormatMOOFullyParenthesized(program), "\n")
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
