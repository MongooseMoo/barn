package parser_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/parser"
	"github.com/MongooseMoo/barn/verb"
)

func TestFormatMOOPreservesEverySamePrecedenceBinaryTree(t *testing.T) {
	groups := [][]verb.BinaryOperator{
		{verb.BinaryOr, verb.BinaryAnd},
		{verb.BinaryBitOr, verb.BinaryBitAnd, verb.BinaryBitXor},
		{verb.BinaryEqual, verb.BinaryNotEqual, verb.BinaryLess, verb.BinaryLessEqual, verb.BinaryGreater, verb.BinaryGreaterEqual, verb.BinaryIn},
		{verb.BinaryShiftLeft, verb.BinaryShiftRight},
		{verb.BinaryAdd, verb.BinarySubtract},
		{verb.BinaryMultiply, verb.BinaryDivide, verb.BinaryModulo},
		{verb.BinaryPower},
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
						var root *verb.BinaryExpr
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

func assertDirectProgramFormatRoundTrip(t *testing.T, program *verb.Program) {
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

func expressionProgram(expr verb.Expr) *verb.Program {
	return &verb.Program{Statements: []verb.Stmt{&verb.ExprStmt{Expr: expr}}}
}

func binary(operator verb.BinaryOperator, left, right verb.Expr) *verb.BinaryExpr {
	return &verb.BinaryExpr{Left: left, Operator: operator, Right: right}
}

func identifier(name string) *verb.IdentifierExpr { return &verb.IdentifierExpr{Name: name} }
