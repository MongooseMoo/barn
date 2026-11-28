package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestListIndexPreservesFirstMatch(t *testing.T) {
	strict := &VM{Context: strictCtx()}
	promoting := &VM{Context: promoteCtx()}
	for _, tc := range []struct {
		name   string
		host   *VM
		needle types.Value
		items  []types.Value
		want   int
	}{
		{"strict int ignores float and bool", strict, types.NewInt(499),
			[]types.Value{types.NewFloat(499), types.NewBool(true), types.NewInt(499)}, 3},
		{"promoted int finds float first", promoting, types.NewInt(499),
			[]types.Value{types.NewFloat(499), types.NewInt(499)}, 1},
		{"boolean matches integer one", strict, types.NewBool(true),
			[]types.Value{types.NewInt(1), types.NewBool(true)}, 1},
		{"integer one matches boolean", strict, types.NewInt(1),
			[]types.Value{types.NewBool(true), types.NewInt(1)}, 1},
		{"folded string and escaped tab", promoting, types.NewStr("a\t"),
			[]types.Value{types.NewInt(1), types.NewStr("A~09"), types.NewStr("a\t")}, 2},
		{"empty list", strict, types.NewInt(499), nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.host.listIndex(tc.needle, types.NewList(tc.items)); got != tc.want {
				t.Fatalf("listIndex = %d, want %d", got, tc.want)
			}
		})
	}
}
