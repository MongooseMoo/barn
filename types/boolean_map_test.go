package types

import "testing"

func TestReverseInsertedBooleanMapKeysExposeReverseTopology(t *testing.T) {
	if got := toastMapCompare(NewBool(false), NewBool(true), false); got <= 0 {
		t.Fatalf("toastMapCompare(false, true) = %d, want positive", got)
	}
	if got := toastMapCompare(NewBool(true), NewBool(false), false); got <= 0 {
		t.Fatalf("toastMapCompare(true, false) = %d, want positive", got)
	}

	falseFirst := NewMap([][2]Value{
		{NewBool(false), NewInt(0)},
		{NewBool(true), NewInt(1)},
	})
	trueFirst := NewMap([][2]Value{
		{NewBool(true), NewInt(1)},
		{NewBool(false), NewInt(0)},
	})

	assertBooleanKeys(t, falseFirst.Keys(), true, false)
	assertBooleanKeys(t, trueFirst.Keys(), false, true)
	if got := falseFirst.String(); got != "[true -> 1, false -> 0]" {
		t.Fatalf("false-first literal = %q, want reverse topology", got)
	}
	if got := trueFirst.String(); got != "[false -> 0, true -> 1]" {
		t.Fatalf("true-first literal = %q, want reverse topology", got)
	}
	if falseFirst.Equal(trueFirst) {
		t.Fatal("maps with different boolean-key topology compare equal")
	}
}

func assertBooleanKeys(t *testing.T, got []Value, want ...bool) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("key count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Type() != TYPE_BOOL || got[i].Bool() != want[i] {
			t.Fatalf("key %d = %v, want %t", i+1, got[i], want[i])
		}
	}
}
