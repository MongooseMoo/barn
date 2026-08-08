package parser

import (
	"github.com/MongooseMoo/barn/verb"
	"testing"
)

func TestParseListLiteral(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []int64
	}{
		{"empty", "{}", []int64{}},
		{"single", "{1}", []int64{1}},
		{"multiple", "{1, 2, 3}", []int64{1, 2, 3}},
		{"trailing_comma", "{1, 2, 3,}", []int64{1, 2, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := parseListForTest(t, tt.input)
			if len(list.Elements) != len(tt.expected) {
				t.Fatalf("len(Elements) = %d, want %d", len(list.Elements), len(tt.expected))
			}

			for i, expected := range tt.expected {
				lit, ok := list.Elements[i].(*verb.LiteralExpr)
				if !ok {
					t.Fatalf("element %d = %T, want *LiteralExpr", i, list.Elements[i])
				}
				if lit.Kind != verb.LiteralInt {
					t.Fatalf("element %d kind = %v, want LiteralInt", i, lit.Kind)
				}
				if lit.IntValue != expected {
					t.Errorf("element %d IntValue = %d, want %d", i, lit.IntValue, expected)
				}
			}
		})
	}
}

func TestParseNestedList(t *testing.T) {
	list := parseListForTest(t, "{1, {2, 3}, 4}")
	if len(list.Elements) != 3 {
		t.Fatalf("len(Elements) = %d, want 3", len(list.Elements))
	}

	assertIntLiteral(t, list.Elements[0], 1)
	inner, ok := list.Elements[1].(*verb.ListExpr)
	if !ok {
		t.Fatalf("element 1 = %T, want *ListExpr", list.Elements[1])
	}
	if len(inner.Elements) != 2 {
		t.Fatalf("len(inner.Elements) = %d, want 2", len(inner.Elements))
	}
	assertIntLiteral(t, inner.Elements[0], 2)
	assertIntLiteral(t, inner.Elements[1], 3)
	assertIntLiteral(t, list.Elements[2], 4)
}

func parseListForTest(t *testing.T, input string) *verb.ListExpr {
	t.Helper()
	expr := parseExprForTest(t, input)
	list, ok := expr.(*verb.ListExpr)
	if !ok {
		t.Fatalf("ParseExpression(%q) returned %T, want *ListExpr", input, expr)
	}
	return list
}

func assertIntLiteral(t *testing.T, expr verb.Expr, expected int64) {
	t.Helper()
	lit, ok := expr.(*verb.LiteralExpr)
	if !ok {
		t.Fatalf("expr = %T, want *LiteralExpr", expr)
	}
	if lit.Kind != verb.LiteralInt {
		t.Fatalf("literal kind = %v, want LiteralInt", lit.Kind)
	}
	if lit.IntValue != expected {
		t.Errorf("IntValue = %d, want %d", lit.IntValue, expected)
	}
}
