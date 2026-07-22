package types

import "testing"

func TestStringEqualityFoldsASCIIWithoutFoldingRawHighBytes(t *testing.T) {
	if !NewStr("A").Equal(NewStr("a")) {
		t.Fatal("ASCII case variants should compare equal")
	}
	if NewStr(string([]byte{0xC0})).Equal(NewStr(string([]byte{0xE0}))) {
		t.Fatal("distinct raw high bytes should not compare equal")
	}
}

func TestStringEqualityFoldsValidUTF8Case(t *testing.T) {
	if !NewStr("À").Equal(NewStr("à")) {
		t.Fatal("valid UTF-8 case variants should compare equal")
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
