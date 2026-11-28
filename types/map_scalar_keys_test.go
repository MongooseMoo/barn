package types

import "testing"

func TestMapScalarKeys(t *testing.T) {
	keys := []Value{NewInt(1), NewObj(1), NewAnon(1), NewErr(E_TYPE), NewBool(true), NewBool(false), NewErr(100)}
	for _, size := range []int{len(keys), mapFlatLimit + 1, mapFlatLimit + 2} {
		pairs := make([][2]Value, 0, size)
		for i, key := range keys {
			pairs = append(pairs, [2]Value{key, NewInt(int64(i + 1))})
		}
		for len(pairs) < size {
			pairs = append(pairs, [2]Value{NewInt(-int64(len(pairs))), NewInt(0)})
		}
		m := NewMap(pairs)
		if size == mapFlatLimit+2 {
			m = m.MapSet(NewInt(-1), NewInt(0))
		}
		for i, key := range keys {
			if got, ok := m.MapGet(key); !ok || got.Int() != int64(i+1) {
				t.Fatalf("size %d: MapGet(%v) = %v, %t", size, key, got, ok)
			}
		}
		if got, ok := m.MapGet(NewErr(-1)); !ok || got.Int() != int64(len(keys)) {
			t.Fatalf("size %d: lookup of unknown error = %v, %t", size, got, ok)
		}
		changed := m.MapSet(NewAnon(1), NewInt(999)).MapDelete(NewErr(-1))
		if _, ok := changed.MapGet(NewErr(100)); ok {
			t.Fatal("unknown error key survived deletion by equivalent code")
		}
		if got, _ := m.MapGet(NewAnon(1)); got.Int() != 3 {
			t.Fatal("map update changed the prior snapshot")
		}
	}
}

func BenchmarkMapScalarKeys(b *testing.B) {
	for _, test := range []struct {
		name string
		key  Value
	}{
		{"anon", NewAnon(1000)},
		{"error", NewErr(E_RANGE)},
		{"bool", NewBool(true)},
		{"int", NewInt(1000)},
	} {
		for _, indexed := range []bool{false, true} {
			name, size := "flat", 8
			if indexed {
				name, size = "indexed", mapFlatLimit+1
			}
			b.Run(test.name+"/"+name, func(b *testing.B) {
				m := NewEmptyMap()
				for i := range size - 1 {
					m = m.MapSet(NewInt(int64(i)), NewInt(0))
				}
				m = m.MapSet(test.key, NewInt(1))
				b.ReportAllocs()
				for b.Loop() {
					if _, ok := m.MapGet(test.key); !ok {
						b.Fatal("lookup missed")
					}
				}
			})
		}
	}
}
