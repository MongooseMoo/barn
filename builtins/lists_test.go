package builtins

import (
	"testing"

	"barn/kernel"
	"barn/types"
)

// TestIsMemberPromoteNumbers: Toast mongoose branch (PROMOTE_NUMBERS on),
// verified live 2026-07-01: is_member(1, {1.0}) and is_member(1.0, {1}) => 1.
// Strict Toast master: both => 0.
func TestIsMemberPromoteNumbers(t *testing.T) {
	strict := kernel.NewTaskContext()
	promote := kernel.NewTaskContext()
	promote.RuntimeOptions.PromoteNumbers = true

	lst := types.NewList([]types.Value{types.NewFloat(1.0)})
	if got := builtinIsMember(strict, []types.Value{types.NewInt(1), lst}); got.Val.Int() != 0 {
		t.Fatalf("strict is_member(1, {1.0}) = %v, want 0", got.Val)
	}
	if got := builtinIsMember(promote, []types.Value{types.NewInt(1), lst}); got.Val.Int() != 1 {
		t.Fatalf("promote is_member(1, {1.0}) = %v, want 1", got.Val)
	}
	intList := types.NewList([]types.Value{types.NewInt(1)})
	if got := builtinIsMember(promote, []types.Value{types.NewFloat(1.0), intList}); got.Val.Int() != 1 {
		t.Fatalf("promote is_member(1.0, {1}) = %v, want 1", got.Val)
	}
	m := types.NewMap([][2]types.Value{{types.NewStr("a"), types.NewFloat(1.0)}})
	if got := builtinIsMember(promote, []types.Value{types.NewInt(1), m}); got.Val.Int() != 1 {
		t.Fatalf("promote is_member(1, [\"a\"->1.0]) = %v, want 1", got.Val)
	}
	if got := builtinIsMember(strict, []types.Value{types.NewInt(1), m}); got.Val.Int() != 0 {
		t.Fatalf("strict is_member(1, [\"a\"->1.0]) = %v, want 0", got.Val)
	}
}
