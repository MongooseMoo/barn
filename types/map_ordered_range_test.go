package types

import (
	"fmt"
	"math"
	"testing"
)

func TestMapOrderedRangeMatchesTraversal(t *testing.T) {
	pairs := [][2]Value{
		{NewStr("~01"), NewInt(1)},
		{NewStr("\x01"), NewInt(2)},
		{NewFloat(math.NaN()), NewInt(3)},
		{NewFloat(1.5), NewInt(4)},
		{NewErr(E_PERM), NewInt(5)},
		{NewBool(false), NewInt(6)},
		{NewAnon(1), NewInt(7)},
		{NewObj(1), NewInt(8)},
	}
	for i := 38; i > 0; i-- {
		pairs = append(pairs, [2]Value{NewStr(fmt.Sprintf("k%02d", i)), NewInt(int64(i))})
	}
	bulk := NewMap(pairs)
	for _, m := range []Value{NewMap(pairs[:8]), bulk, bulk.MapDelete(NewStr("k03"))} {
		all := m.Pairs()
		for i, last := range []bool{false, true} {
			position := i * (len(all) - 1)
			if got, ok := m.MapBoundaryKey(last); !ok || !got.Identical(all[position][0]) {
				t.Fatalf("boundary last=%t = %v, %t, want %v", last, got, ok, all[position][0])
			}
		}
		for _, search := range append(pairs, [2]Value{NewStr("K10")}, [2]Value{NewStr("missing")}) {
			want := int64(0)
			for i, p := range all {
				if p[0].Equal(search[0]) {
					want = int64(i + 1)
					break
				}
			}
			if got := m.KeyPosition(search[0]); got != want {
				t.Fatalf("key position of %v = %d, want %d", search[0], got, want)
			}
		}
		for first := range all {
			for last := first; last < len(all); last++ {
				got := m.AppendMapPairs(nil, first+1, last+1)
				if len(got) != last-first+1 {
					t.Fatalf("interval %d..%d returned %d entries", first, last, len(got))
				}
				for i, pair := range got {
					want := all[first+i]
					if !pair[0].Identical(want[0]) || !pair[1].Identical(want[1]) {
						t.Fatalf("interval %d..%d entry %d = %v, want %v", first, last, i, pair, want)
					}
				}
			}
		}
	}
	if _, ok := NewEmptyMap().MapBoundaryKey(true); ok {
		t.Fatal("empty map exposed a last key")
	}
}

func BenchmarkMapTreeRange(b *testing.B) {
	pairs := make([][2]Value, 2_000)
	for i := range pairs {
		pairs[i] = [2]Value{NewStr(fmt.Sprintf("k%04d", i)), NewInt(int64(i))}
	}
	m := NewMap(pairs)
	m.MapBoundaryKey(false)
	from, to := NewStr("K0100"), NewStr("k0105")
	b.Run("boundaries", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			m.MapBoundaryKey(false)
			m.MapBoundaryKey(true)
		}
	})
	b.Run("six_pairs", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			start, end := m.KeyPosition(from), m.KeyPosition(to)
			if got := m.AppendMapPairs(nil, int(start), int(end)); len(got) != 6 {
				b.Fatalf("got %d pairs", len(got))
			}
		}
	})
}
