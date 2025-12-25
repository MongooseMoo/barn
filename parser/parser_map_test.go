package parser

import (
	"barn/types"
	"testing"
)

func TestParseMapLiteral(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]int64 // For simple str->int maps
	}{
		{"empty", "[]", map[string]int64{}},
		{"single", `["a" -> 1]`, map[string]int64{"a": 1}},
		{"multiple", `["a" -> 1, "b" -> 2]`, map[string]int64{"a": 1, "b": 2}},
		{"trailing_comma", `["a" -> 1, "b" -> 2,]`, map[string]int64{"a": 1, "b": 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser(tt.input)
			val, err := p.ParseLiteral()
			if err != nil {
				t.Fatalf("ParseLiteral() error = %v", err)
			}

			mapVal, ok := val.(types.MapValue)
			if !ok {
				t.Fatalf("expected MapValue, got %T", val)
			}

			if mapVal.Len() != len(tt.expected) {
				t.Errorf("expected length %d, got %d", len(tt.expected), mapVal.Len())
			}

			// Check entries
			for key, expectedVal := range tt.expected {
				v, exists := mapVal.Get(types.NewStr(key))
				if !exists {
					t.Errorf("key %q not found", key)
					continue
				}
				intVal, ok := v.(types.IntValue)
				if !ok {
					t.Errorf("value for key %q: expected IntValue, got %T", key, v)
					continue
				}
				if intVal.Val != expectedVal {
					t.Errorf("value for key %q: expected %d, got %d", key, expectedVal, intVal.Val)
				}
			}

			// Check type
			if mapVal.Type() != types.TYPE_MAP {
				t.Errorf("expected type TYPE_MAP, got %v", mapVal.Type())
			}
		})
	}
}

func TestParseMapWithDifferentKeyTypes(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"int_key", "[1 -> \"one\"]"},
		{"float_key", "[3.14 -> \"pi\"]"},
		{"obj_key", "[#42 -> \"answer\"]"},
		{"err_key", "[E_TYPE -> \"type error\"]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser(tt.input)
			val, err := p.ParseLiteral()
			if err != nil {
				t.Fatalf("ParseLiteral() error = %v", err)
			}

			mapVal, ok := val.(types.MapValue)
			if !ok {
				t.Fatalf("expected MapValue, got %T", val)
			}

			if mapVal.Len() != 1 {
				t.Errorf("expected length 1, got %d", mapVal.Len())
			}
		})
	}
}

func TestParseMapWithNestedValue(t *testing.T) {
	input := `["x" -> {1, 2, 3}]`
	p := NewParser(input)
	val, err := p.ParseLiteral()
	if err != nil {
		t.Fatalf("ParseLiteral() error = %v", err)
	}

	mapVal, ok := val.(types.MapValue)
	if !ok {
		t.Fatalf("expected MapValue, got %T", val)
	}

	v, exists := mapVal.Get(types.NewStr("x"))
	if !exists {
		t.Fatal("key 'x' not found")
	}

	listVal, ok := v.(types.ListValue)
	if !ok {
		t.Fatalf("expected ListValue, got %T", v)
	}

	if listVal.Len() != 3 {
		t.Errorf("expected list length 3, got %d", listVal.Len())
	}
}

func TestMapTruthy(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"[]", false},             // Empty map is falsy
		{`["a" -> 1]`, true},      // Non-empty map is truthy
		{`[1 -> 2, 3 -> 4]`, true}, // Non-empty map is truthy
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			val, err := p.ParseLiteral()
			if err != nil {
				t.Fatalf("ParseLiteral() error = %v", err)
			}

			if val.Truthy() != tt.expected {
				t.Errorf("expected truthy=%v, got %v", tt.expected, val.Truthy())
			}
		})
	}
}

func TestMapEqual(t *testing.T) {
	m1 := types.NewMap([][2]types.Value{
		{types.NewStr("a"), types.NewInt(1)},
		{types.NewStr("b"), types.NewInt(2)},
	})

	m2 := types.NewMap([][2]types.Value{
		{types.NewStr("a"), types.NewInt(1)},
		{types.NewStr("b"), types.NewInt(2)},
	})

	m3 := types.NewMap([][2]types.Value{
		{types.NewStr("a"), types.NewInt(1)},
	})

	if !m1.Equal(m2) {
		t.Error("identical maps should be equal")
	}

	if m1.Equal(m3) {
		t.Error("different maps should not be equal")
	}

	// Test cross-type equality
	i := types.NewInt(1)
	if m1.Equal(i) {
		t.Error("map should not equal int")
	}
}

func TestMapString(t *testing.T) {
	tests := []struct {
		mapVal   types.MapValue
		expected string
	}{
		{types.NewEmptyMap(), "[]"},
		// Note: map order is non-deterministic, so we can't test multi-entry maps reliably
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.mapVal.String() != tt.expected {
				t.Errorf("expected String() %q, got %q", tt.expected, tt.mapVal.String())
			}
		})
	}
}

func TestInvalidMapKeyType(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"list_key", "[{1, 2} -> \"value\"]"},
		{"map_key", "[[\"nested\" -> 1] -> \"value\"]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser(tt.input)
			_, err := p.ParseLiteral()
			if err == nil {
				t.Error("expected error for invalid key type, got nil")
			}
		})
	}
}
