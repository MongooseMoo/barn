package types

import (
	"strings"
	"testing"
)

func BenchmarkStringEqual(b *testing.B) {
	for _, tc := range []struct{ name, value string }{
		{"equal_same_case", "item_499"},
		{"equal_diff_case", "ITEM_499"},
		{"unequal_same_len", "item_500"},
		{"unequal_diff_len", "item_1"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			left, right := NewStr("item_499"), NewStr(tc.value)
			for b.Loop() {
				left.Equal(right)
			}
		})
	}
}

func TestStringEqualityMatchesNormalizedComparison(t *testing.T) {
	// Compare the fast path to the same normalization and collation used
	// before it, including mismatches after a common prefix.
	inputs := []string{
		"", "x", "AB", "Ab", "K~0a", "K\n", "~09", "~0a", "~0A", "~0G", "~0",
		"\t", "\n", "~20", " ", "~~0A", "~\n", "\xc0A~09", "\xc0a\t", "\xe0a\t",
		"\xff", "�", "À", "à", "KA\xff", "Ka\xff", "AB~01", "ab~02",
	}
	for _, prefix := range []string{"", "ab~09", "\xc0", "é", "~0"} {
		for _, left := range inputs {
			for _, right := range inputs {
				a, b := prefix+left, prefix+right
				want := compareFoldedASCII(normalizeBinaryString(a), normalizeBinaryString(b)) == 0
				if got := NewStr(a).Equal(NewStr(b)); got != want {
					t.Errorf("Equal(%q, %q) = %v, want %v", a, b, got, want)
				}
			}
		}
	}
	word := "shared text"
	if !NewStr(word).Equal(NewStr(strings.Clone(word))) {
		t.Fatal("short identical strings with separate storage should compare equal")
	}
	if NewStr(word).Equal(NewStr(word[:6])) {
		t.Fatal("strings with the same starting address and different lengths should not compare equal")
	}
}

func TestStringEqualityFoldsASCIIWithoutFoldingRawHighBytes(t *testing.T) {
	if !NewStr("A").Equal(NewStr("a")) {
		t.Fatal("ASCII case variants should compare equal")
	}
	if NewStr(string([]byte{0xC0})).Equal(NewStr(string([]byte{0xE0}))) {
		t.Fatal("distinct raw high bytes should not compare equal")
	}
}

func TestStringEqualityTreatsBinaryControlEscapesAsBytes(t *testing.T) {
	if !NewStr("a~09b").Equal(NewStr("a\tb")) {
		t.Fatal("MOO binary tab escape should equal a literal tab byte")
	}
}

func TestStringEqualityWithoutEscapesDoesNotAllocate(t *testing.T) {
	a, b := NewStr("The quick brown fox"), NewStr("the quick brown FOX")
	if !a.Equal(b) {
		t.Fatal("case variants should compare equal")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = a.Equal(b) }); allocs != 0 {
		t.Fatalf("comparing two strings with no ~ escape allocated %v times", allocs)
	}
}

func TestStringEqualityFoldsValidUTF8Case(t *testing.T) {
	if !NewStr("À").Equal(NewStr("à")) {
		t.Fatal("valid UTF-8 case variants should compare equal")
	}
}

func TestCompareFoldedMapKeys(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"#10040:apply_heat", "#10040:apply_physics", -1},
		{"apply_heat", "APPLY_HEAT", 0},
		{"K", "K", 0},
		{"\xff", "\xfe", 1},
		{"\xff", "�", 1},
	} {
		if got := compareFoldedASCII(tc.a, tc.b); got != tc.want {
			t.Errorf("compareFoldedASCII(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestRawHighByteMapKeysRemainDistinct(t *testing.T) {
	upper := NewStr(string([]byte{0xC0}))
	lower := NewStr(string([]byte{0xE0}))
	m := NewMap([][2]Value{
		{upper, NewInt(1)},
		{lower, NewInt(2)},
	})

	if m.Len() != 2 {
		t.Fatalf("map length = %d, want two distinct raw-byte keys", m.Len())
	}
	if got, ok := m.MapGet(upper); !ok || got.Int() != 1 {
		t.Fatalf("upper raw-byte key lookup = (%v, %v), want (1, true)", got, ok)
	}
	if got, ok := m.MapGet(lower); !ok || got.Int() != 2 {
		t.Fatalf("lower raw-byte key lookup = (%v, %v), want (2, true)", got, ok)
	}
}
