package types

import "testing"

// ListBytes reads cached sizes in place; it must equal ValueBytes of the list.
func TestListBytesMatchesValueBytes(t *testing.T) {
	m := NewMap([][2]Value{{NewInt(1), NewStr("abc")}})
	args := []Value{m, NewInt(5), NewInt(-5), NewStr("hello"), NewFloat(1.5), NewList([]Value{NewInt(1), m}), NewErr(E_QUOTA), NewObj(3), NewBool(true), NewStr(""), NewEmptyList(), NewEmptyMap()}
	if got, want := ListBytes(args), ValueBytes(NewList(args)); got != want {
		t.Fatalf("ListBytes = %d, ValueBytes = %d", got, want)
	}
}
