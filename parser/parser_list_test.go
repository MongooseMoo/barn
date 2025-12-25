package parser

import (
	"barn/types"
	"testing"
)

func TestParseListLiteral(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []int64 // For simple int lists
	}{
		{"empty", "{}", []int64{}},
		{"single", "{1}", []int64{1}},
		{"multiple", "{1, 2, 3}", []int64{1, 2, 3}},
		{"trailing_comma", "{1, 2, 3,}", []int64{1, 2, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser(tt.input)
			val, err := p.ParseLiteral()
			if err != nil {
				t.Fatalf("ParseLiteral() error = %v", err)
			}

			list, ok := val.(types.ListValue)
			if !ok {
				t.Fatalf("expected ListValue, got %T", val)
			}

			if list.Len() != len(tt.expected) {
				t.Errorf("expected length %d, got %d", len(tt.expected), list.Len())
			}

			// Check elements
			for i, expectedVal := range tt.expected {
				elem := list.Get(i + 1) // 1-based indexing
				intVal, ok := elem.(types.IntValue)
				if !ok {
					t.Errorf("element %d: expected IntValue, got %T", i, elem)
					continue
				}
				if intVal.Val != expectedVal {
					t.Errorf("element %d: expected %d, got %d", i, expectedVal, intVal.Val)
				}
			}

			// Check type
			if list.Type() != types.TYPE_LIST {
				t.Errorf("expected type TYPE_LIST, got %v", list.Type())
			}
		})
	}
}

func TestParseNestedList(t *testing.T) {
	input := "{1, {2, 3}, 4}"
	p := NewParser(input)
	val, err := p.ParseLiteral()
	if err != nil {
		t.Fatalf("ParseLiteral() error = %v", err)
	}

	list, ok := val.(types.ListValue)
	if !ok {
		t.Fatalf("expected ListValue, got %T", val)
	}

	if list.Len() != 3 {
		t.Fatalf("expected length 3, got %d", list.Len())
	}

	// Check first element (1)
	elem1 := list.Get(1)
	if intVal, ok := elem1.(types.IntValue); !ok || intVal.Val != 1 {
		t.Errorf("element 1: expected IntValue(1), got %v", elem1)
	}

	// Check second element ({2, 3})
	elem2 := list.Get(2)
	innerList, ok := elem2.(types.ListValue)
	if !ok {
		t.Fatalf("element 2: expected ListValue, got %T", elem2)
	}
	if innerList.Len() != 2 {
		t.Errorf("inner list: expected length 2, got %d", innerList.Len())
	}

	// Check third element (4)
	elem3 := list.Get(3)
	if intVal, ok := elem3.(types.IntValue); !ok || intVal.Val != 4 {
		t.Errorf("element 3: expected IntValue(4), got %v", elem3)
	}
}

func TestListTruthy(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"{}", false},       // Empty list is falsy
		{"{1}", true},       // Non-empty list is truthy
		{"{1, 2, 3}", true}, // Non-empty list is truthy
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

func TestListEqual(t *testing.T) {
	l1 := types.NewList([]types.Value{
		types.NewInt(1),
		types.NewInt(2),
		types.NewInt(3),
	})

	l2 := types.NewList([]types.Value{
		types.NewInt(1),
		types.NewInt(2),
		types.NewInt(3),
	})

	l3 := types.NewList([]types.Value{
		types.NewInt(1),
		types.NewInt(2),
	})

	if !l1.Equal(l2) {
		t.Error("identical lists should be equal")
	}

	if l1.Equal(l3) {
		t.Error("different lists should not be equal")
	}

	// Test cross-type equality
	i := types.NewInt(1)
	if l1.Equal(i) {
		t.Error("list should not equal int")
	}
}

func TestListDeepEqual(t *testing.T) {
	// Test deep equality with nested lists
	l1 := types.NewList([]types.Value{
		types.NewInt(1),
		types.NewList([]types.Value{
			types.NewInt(2),
			types.NewInt(3),
		}),
	})

	l2 := types.NewList([]types.Value{
		types.NewInt(1),
		types.NewList([]types.Value{
			types.NewInt(2),
			types.NewInt(3),
		}),
	})

	l3 := types.NewList([]types.Value{
		types.NewInt(1),
		types.NewList([]types.Value{
			types.NewInt(2),
			types.NewInt(4), // Different
		}),
	})

	if !l1.Equal(l2) {
		t.Error("identical nested lists should be equal")
	}

	if l1.Equal(l3) {
		t.Error("different nested lists should not be equal")
	}
}

func TestListString(t *testing.T) {
	tests := []struct {
		list     types.ListValue
		expected string
	}{
		{types.NewEmptyList(), "{}"},
		{types.NewList([]types.Value{types.NewInt(1)}), "{1}"},
		{types.NewList([]types.Value{types.NewInt(1), types.NewInt(2)}), "{1, 2}"},
		{types.NewList([]types.Value{
			types.NewInt(1),
			types.NewList([]types.Value{types.NewInt(2)}),
		}), "{1, {2}}"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.list.String() != tt.expected {
				t.Errorf("expected String() %q, got %q", tt.expected, tt.list.String())
			}
		})
	}
}
