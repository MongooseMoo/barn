package vm

import (
	"testing"

	"barn/types"
)

// TestStringAppendRoutingCharacterization locks the observable behavior of the
// OP_STRING_APPEND handler (executeStringAppend) reached via the self-accumulation
// peephole `x = x + expr` (bytecode/compiler.go). These are characterization
// tests: they encode current master behavior and MUST stay green across the
// numeric-first reorder of executeStringAppend, which is a pure reordering with
// no behavior change.
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
			// Mixed numeric self-append under default (non-promote) settings.
			// Observed master behavior: E_TYPE (executeStringAppend has no
			// PROMOTE_NUMBERS branch). Locked here, not assumed away.
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
