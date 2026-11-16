package types

import "testing"

// Toast 2.8 (c0966e5) orders bools in compare() as false < true, so a map's
// boolean keys come out false first whatever order they went in.
func TestBooleanMapKeysOrderFalseBeforeTrue(t *testing.T) {
	if got := toastMapCompare(NewBool(false), NewBool(true), false); got >= 0 {
		t.Fatalf("toastMapCompare(false, true) = %d, want negative", got)
	}
	if got := toastMapCompare(NewBool(true), NewBool(false), false); got <= 0 {
		t.Fatalf("toastMapCompare(true, false) = %d, want positive", got)
	}
	if got := toastMapCompare(NewBool(true), NewBool(true), false); got != 0 {
		t.Fatalf("toastMapCompare(true, true) = %d, want 0", got)
	}

	falseFirst := NewMap([][2]Value{
		{NewBool(false), NewInt(0)},
		{NewBool(true), NewInt(1)},
	})
	trueFirst := NewMap([][2]Value{
		{NewBool(true), NewInt(1)},
		{NewBool(false), NewInt(0)},
	})

	assertBooleanKeys(t, falseFirst.Keys(), false, true)
	assertBooleanKeys(t, trueFirst.Keys(), false, true)
	for _, m := range []Value{falseFirst, trueFirst} {
		if got := m.String(); got != "[false -> 0, true -> 1]" {
			t.Fatalf("literal = %q, want [false -> 0, true -> 1]", got)
		}
	}
	if !falseFirst.Equal(trueFirst) {
		t.Fatal("boolean-key maps with the same pairs compare unequal")
	}
}

// The mixed true/1/false/0 map from boolean_authority.yaml: with a total
// order every key is reachable through the builtin (tree) lookup.
func TestBooleanAndIntegerKeysAllReachableThroughTreeLookup(t *testing.T) {
	m := NewEmptyMap()
	m = m.MapSet(NewBool(true), NewStr("boolean one"))
	m = m.MapSet(NewInt(1), NewStr("integer one"))
	m = m.MapSet(NewBool(false), NewStr("boolean zero"))
	m = m.MapSet(NewInt(0), NewStr("integer zero"))
	for _, key := range []Value{NewBool(true), NewInt(1), NewBool(false), NewInt(0)} {
		if _, ok := m.GetWithCase(key, true); !ok {
			t.Fatalf("GetWithCase(%v) found nothing", key)
		}
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
