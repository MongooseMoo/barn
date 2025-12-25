package types

import "testing"

func TestErrorCodes(t *testing.T) {
	// Verify error codes match spec/errors.md
	tests := []struct {
		code     ErrorCode
		value    int
		name     string
	}{
		{E_NONE, 0, "E_NONE"},
		{E_TYPE, 1, "E_TYPE"},
		{E_DIV, 2, "E_DIV"},
		{E_PERM, 3, "E_PERM"},
		{E_PROPNF, 4, "E_PROPNF"},
		{E_VERBNF, 5, "E_VERBNF"},
		{E_VARNF, 6, "E_VARNF"},
		{E_INVIND, 7, "E_INVIND"},
		{E_RECMOVE, 8, "E_RECMOVE"},
		{E_MAXREC, 9, "E_MAXREC"},
		{E_RANGE, 10, "E_RANGE"},
		{E_ARGS, 11, "E_ARGS"},
		{E_NACC, 12, "E_NACC"},
		{E_INVARG, 13, "E_INVARG"},
		{E_QUOTA, 14, "E_QUOTA"},
		{E_FLOAT, 15, "E_FLOAT"},
		{E_FILE, 16, "E_FILE"},
		{E_EXEC, 17, "E_EXEC"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.code) != tt.value {
				t.Errorf("%s: expected value %d, got %d", tt.name, tt.value, int(tt.code))
			}

			if tt.code.String() != tt.name {
				t.Errorf("%s: String() returned %q, expected %q", tt.name, tt.code.String(), tt.name)
			}
		})
	}
}

func TestObjIDConstants(t *testing.T) {
	if ObjNothing != -1 {
		t.Errorf("ObjNothing should be -1, got %d", ObjNothing)
	}

	if ObjAmbiguous != -2 {
		t.Errorf("ObjAmbiguous should be -2, got %d", ObjAmbiguous)
	}

	if ObjFailedMatch != -3 {
		t.Errorf("ObjFailedMatch should be -3, got %d", ObjFailedMatch)
	}
}
