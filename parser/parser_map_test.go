package parser

import (
	"github.com/MongooseMoo/barn/verb"
	"testing"
)

func TestParseMapLiteral(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]int64
	}{
		{"empty", "[]", map[string]int64{}},
		{"single", `["a" -> 1]`, map[string]int64{"a": 1}},
		{"multiple", `["a" -> 1, "b" -> 2]`, map[string]int64{"a": 1, "b": 2}},
		{"trailing_comma", `["a" -> 1, "b" -> 2,]`, map[string]int64{"a": 1, "b": 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapExpr := parseMapForTest(t, tt.input)
			if len(mapExpr.Pairs) != len(tt.expected) {
				t.Fatalf("len(Pairs) = %d, want %d", len(mapExpr.Pairs), len(tt.expected))
			}

			for _, pair := range mapExpr.Pairs {
				key, ok := pair.Key.(*verb.LiteralExpr)
				if !ok {
					t.Fatalf("key = %T, want *LiteralExpr", pair.Key)
				}
				if key.Kind != verb.LiteralString {
					t.Fatalf("key kind = %v, want LiteralString", key.Kind)
				}
				expected, ok := tt.expected[key.StringValue]
				if !ok {
					t.Fatalf("unexpected key %q", key.StringValue)
				}
				assertIntLiteral(t, pair.Value, expected)
			}
		})
	}
}

func TestParseMapWithDifferentKeySyntax(t *testing.T) {
	tests := []struct {
		name  string
		input string
		kind  verb.LiteralKind
	}{
		{"int_key", `[1 -> "one"]`, verb.LiteralInt},
		{"float_key", `[3.14 -> "pi"]`, verb.LiteralFloat},
		{"obj_key", `[#42 -> "answer"]`, verb.LiteralObj},
		{"err_key", `[E_TYPE -> "type error"]`, verb.LiteralErr},
		{"list_key", `[{1, 2} -> "value"]`, -1},
		{"map_key", `[[ "nested" -> 1] -> "value"]`, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapExpr := parseMapForTest(t, tt.input)
			if len(mapExpr.Pairs) != 1 {
				t.Fatalf("len(Pairs) = %d, want 1", len(mapExpr.Pairs))
			}

			if tt.kind >= 0 {
				key, ok := mapExpr.Pairs[0].Key.(*verb.LiteralExpr)
				if !ok {
					t.Fatalf("key = %T, want *LiteralExpr", mapExpr.Pairs[0].Key)
				}
				if key.Kind != tt.kind {
					t.Errorf("key kind = %v, want %v", key.Kind, tt.kind)
				}
			}
		})
	}
}

func TestParseMapWithNestedValue(t *testing.T) {
	mapExpr := parseMapForTest(t, `["x" -> {1, 2, 3}]`)
	if len(mapExpr.Pairs) != 1 {
		t.Fatalf("len(Pairs) = %d, want 1", len(mapExpr.Pairs))
	}

	key, ok := mapExpr.Pairs[0].Key.(*verb.LiteralExpr)
	if !ok {
		t.Fatalf("key = %T, want *LiteralExpr", mapExpr.Pairs[0].Key)
	}
	if key.Kind != verb.LiteralString || key.StringValue != "x" {
		t.Fatalf("key = (%v, %q), want (LiteralString, \"x\")", key.Kind, key.StringValue)
	}

	list, ok := mapExpr.Pairs[0].Value.(*verb.ListExpr)
	if !ok {
		t.Fatalf("value = %T, want *ListExpr", mapExpr.Pairs[0].Value)
	}
	if len(list.Elements) != 3 {
		t.Fatalf("len(Elements) = %d, want 3", len(list.Elements))
	}
	assertIntLiteral(t, list.Elements[0], 1)
	assertIntLiteral(t, list.Elements[1], 2)
	assertIntLiteral(t, list.Elements[2], 3)
}

func parseMapForTest(t *testing.T, input string) *verb.MapExpr {
	t.Helper()
	expr := parseExprForTest(t, input)
	mapExpr, ok := expr.(*verb.MapExpr)
	if !ok {
		t.Fatalf("ParseExpression(%q) returned %T, want *MapExpr", input, expr)
	}
	return mapExpr
}
