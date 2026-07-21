package types

import "testing"

func TestBooleanMapKeysSortFalseBeforeTrue(t *testing.T) {
	if got := CompareMapKeys(NewBool(false), NewBool(true)); got >= 0 {
		t.Fatalf("CompareMapKeys(false, true) = %d, want negative", got)
	}
	if got := CompareMapKeys(NewBool(true), NewBool(false)); got <= 0 {
		t.Fatalf("CompareMapKeys(true, false) = %d, want positive", got)
	}
}
