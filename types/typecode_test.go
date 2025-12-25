package types

import "testing"

func TestTypeCodes(t *testing.T) {
	tests := []struct {
		code TypeCode
		val  int
		name string
	}{
		{TYPE_INT, 0, "INT"},
		{TYPE_OBJ, 1, "OBJ"},
		{TYPE_STR, 2, "STR"},
		{TYPE_ERR, 3, "ERR"},
		{TYPE_LIST, 4, "LIST"},
		{TYPE_FLOAT, 9, "FLOAT"},
		{TYPE_MAP, 10, "MAP"},
		{TYPE_WAIF, 13, "WAIF"},
		{TYPE_BOOL, 14, "BOOL"},
	}

	for _, tt := range tests {
		if int(tt.code) != tt.val {
			t.Errorf("Type code %s should be %d, got %d", tt.name, tt.val, int(tt.code))
		}
		if tt.code.String() != tt.name {
			t.Errorf("Type code %d should stringify to %s, got %s", tt.val, tt.name, tt.code.String())
		}
	}
}
