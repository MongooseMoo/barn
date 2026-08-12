package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestStringAppendRoutingCharacterization locks the observable behavior of the
// compatibility opcode reached via the self-accumulation peephole `x = x + expr`.
func TestStringAppendRoutingCharacterization(t *testing.T) {
	cases := []struct {
		name string
		code string
		// want is the expected res.Val.String(); if wantErr is set, we assert an
		// exception with that error code instead.
		want    string
		wantErr types.ErrorCode
	}{
		{
			name: "int accumulation sums to 5050",
			code: `x = 0; for i in [1..100]; x = x + i; endfor; return x;`,
			want: "5050",
		},
		{
			name: "float accumulation sums to 15.0",
			code: `x = 0.0; for i in [1..10]; x = x + 1.5; endfor; return x;`,
			want: "15.0",
		},
		{
			name: "string concat still works",
			code: `s = ""; for i in [1..5]; s = s + "x"; endfor; return s;`,
			want: `"xxxxx"`,
		},
		{
			name:    "string plus int still raises E_TYPE",
			code:    `s = "a"; s = s + 1; return s;`,
			wantErr: types.E_TYPE,
		},
		{
			name:    "mixed int+float self-append raises E_TYPE under default settings",
			code:    `x = 0; x = x + 1.5; return x;`,
			wantErr: types.E_TYPE,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runBytecodeProgram(t, tc.code, nil, nil)
			if tc.wantErr != types.E_NONE {
				if res.Flow != types.FlowException || res.Error != tc.wantErr {
					t.Fatalf("got flow=%v error=%v val=%v, want exception %v",
						res.Flow, res.Error, res.Val, tc.wantErr)
				}
				return
			}
			if res.Flow == types.FlowException {
				t.Fatalf("unexpected exception %v (val=%v), want %s", res.Error, res.Val, tc.want)
			}
			if got := res.Val.String(); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestSelfAddMatchesAddFloatOverflow(t *testing.T) {
	for _, code := range []string{
		`value = 1.0e308; value = value + 1.0e308; return value;`,
		`value = 1.0e308; result = value + 1.0e308; return result;`,
	} {
		result := runBytecodeProgram(t, code, nil, nil)
		if result.Flow != types.FlowException || result.Error != types.E_FLOAT {
			t.Fatalf("code %q: got flow=%v error=%v val=%v, want E_FLOAT", code, result.Flow, result.Error, result.Val)
		}
	}
}

func TestSelfAddMatchesAddWithPromotedNumbers(t *testing.T) {
	for _, code := range []string{
		`value = 0; value = value + 1.5; return value;`,
		`value = 0; result = value + 1.5; return result;`,
	} {
		result := runBytecodeProgram(t, code, nil, promoteCtx())
		if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_FLOAT || result.Val.Float() != 1.5 {
			t.Fatalf("code %q: got flow=%v error=%v val=%v, want float 1.5", code, result.Flow, result.Error, result.Val)
		}
	}
}
