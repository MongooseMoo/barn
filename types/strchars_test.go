package types

import (
	"reflect"
	"testing"
)

func TestCharCountCountsCodePointsAndInvalidBytes(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"café", 4},
		{"é", 2},             // combining accent is its own code point
		{"👨‍👩‍👧", 5},          // ZWJ sequence: 3 emoji + 2 joiners
		{"a\xe9b", 3},         // Latin-1 byte: one character
		{"\xff\xfe", 2},       // each invalid byte is one character
		{"\xe2\x82", 2},       // truncated sequence: one character per byte
		{"�", 1},              // a real U+FFFD is one character
		{"x\xe2\x82\xacy", 3}, // valid euro sign stays one character
	} {
		if got := CharCount(tc.s); got != tc.want {
			t.Errorf("CharCount(%q) = %d, want %d", tc.s, got, tc.want)
		}
	}
}

func TestCharSlicePreservesRawBytes(t *testing.T) {
	s := "a\xe9€b"
	if got := CharSlice(s, 1, 2); got != "\xe9" {
		t.Fatalf("invalid byte slice = %q, want raw 0xE9", got)
	}
	if got := CharSlice(s, 2, 3); got != "€" {
		t.Fatalf("euro slice = %q", got)
	}
	if got := CharSlice(s, 0, 4); got != s {
		t.Fatalf("full slice = %q, want %q", got, s)
	}
	if got := CharSlice(s, 2, 2); got != "" {
		t.Fatalf("empty slice = %q", got)
	}
}

func TestSplitCharsRoundTripsBytes(t *testing.T) {
	s := "c\xe9fé€\xff"
	parts := SplitChars(s)
	if want := []string{"c", "\xe9", "f", "é", "€", "\xff"}; !reflect.DeepEqual(parts, want) {
		t.Fatalf("SplitChars = %q, want %q", parts, want)
	}
	joined := ""
	for _, p := range parts {
		joined += p
	}
	if joined != s {
		t.Fatalf("rejoined %q != %q", joined, s)
	}
}

func TestCharOffsetsMapCharactersToBytes(t *testing.T) {
	if got, want := CharOffsets("aé\xffb"), []int{0, 1, 3, 4, 5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CharOffsets = %v, want %v", got, want)
	}
}

func TestStrCharLenIsCachedPerValue(t *testing.T) {
	v := NewStr("héllo")
	if got := v.StrCharLen(); got != 5 {
		t.Fatalf("StrCharLen = %d, want 5", got)
	}
	if got := v.StrCharLen(); got != 5 {
		t.Fatalf("cached StrCharLen = %d, want 5", got)
	}
	appended := v.StrAppend(NewStr("€"))
	if got := appended.StrCharLen(); got != 6 {
		t.Fatalf("appended StrCharLen = %d, want 6", got)
	}
	if got := v.StrCharLen(); got != 5 {
		t.Fatalf("original StrCharLen after append = %d, want 5", got)
	}
	if got := NewStr("").StrCharLen(); got != 0 {
		t.Fatalf("empty StrCharLen = %d", got)
	}
}
