package bytecode

import (
	"bytes"
	"testing"
)

func TestCompactInternalLocalOperands(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after []byte
	}{
		{"constant", []byte{byte(OP_PUSH), 255}, []byte{byte(OP_PUSH), 255}},
		{"get", []byte{byte(OP_GET_VAR), 255}, []byte{byte(OP_GET_VAR), 2}},
		{"set", []byte{byte(OP_SET_VAR), 254}, []byte{byte(OP_SET_VAR), 1}},
		{"range", []byte{byte(OP_FOR_RANGE_NEXT_WIDE), 0, 255, 0, 0, 0, 255}, []byte{byte(OP_FOR_RANGE_NEXT_WIDE), 0, 2, 0, 0, 0, 255}},
		{"list", []byte{byte(OP_FOR_LIST_LOAD_KV), 254, 255, 0, 254}, []byte{byte(OP_FOR_LIST_LOAD_KV), 1, 2, 0, 1}},
		{"no fork binding", []byte{byte(OP_FORK_LOCAL_WIDE), 0, 0, 0, 0, 0, 1}, []byte{byte(OP_FORK_LOCAL_WIDE), 0, 0, 0, 0, 0, 1}},
		{"fork binding", []byte{byte(OP_FORK_LOCAL_WIDE), 1, 0, 0, 0, 0, 1}, []byte{byte(OP_FORK_LOCAL_WIDE), 0, 3, 0, 0, 0, 1}},
		{"catch", []byte{byte(OP_TRY_EXCEPT_LOCAL_WIDE), 1, 1, 5, 1, 0, 0, 0, 0, 255}, []byte{byte(OP_TRY_EXCEPT_LOCAL_WIDE), 1, 1, 5, 0, 3, 0, 0, 0, 255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &Program{Code: bytes.Clone(tc.before), NumLocals: 256, VarNames: []string{"source"}}
			if err := p.CompactInternalLocals(254); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(p.Code, tc.after) || p.NumLocals != 3 {
				t.Fatalf("code %v, locals %d; want %v, 3", p.Code, p.NumLocals, tc.after)
			}
		})
	}
}
