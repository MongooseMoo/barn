package sourcekey

import "testing"

func TestOfIsDeterministicAndSet(t *testing.T) {
	lines := []string{"return 1;", "\"trailing\";"}
	first := Of(lines)
	second := Of([]string{"return 1;", "\"trailing\";"})
	if first != second {
		t.Fatalf("Of() is not deterministic: %v != %v", first, second)
	}
	if !first.IsSet() {
		t.Fatalf("Of() returned an unset key")
	}
}

func TestZeroKeyIsUnset(t *testing.T) {
	var zero Key
	if zero.IsSet() {
		t.Fatalf("zero Key reports IsSet")
	}
	if Of(nil).IsSet() != true {
		t.Fatalf("Of(nil) must still be a set key: empty source is a real source")
	}
	if Of(nil) == zero {
		t.Fatalf("Of(nil) collides with the unset sentinel")
	}
}

func TestDistinctSourcesGetDistinctKeys(t *testing.T) {
	cases := [][]string{
		nil,
		{},
		{""},
		{"", ""},
		{"a"},
		{"a", ""},
		{"a", "b"},
		{"ab"},
		{"ab", ""},
		{"a", "b", "c"},
	}
	seen := make(map[Key]int, len(cases))
	for i, lines := range cases {
		key := Of(lines)
		if prior, ok := seen[key]; ok && !sameLines(cases[prior], lines) {
			t.Fatalf("cases %d (%q) and %d (%q) share a key", prior, cases[prior], i, lines)
		}
		seen[key] = i
	}
}

func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
