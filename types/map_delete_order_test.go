package types

import "testing"

func TestMapDeletePreservesSnapshotInsertionOrder(t *testing.T) {
	base := NewEmptyMap()
	for i := 31; i >= 0; i-- {
		base = base.MapSet(NewInt(int64(i)), NewInt(int64(i)))
	}
	deleted := base.MapDelete(NewInt(31)).MapDelete(NewInt(14))
	changed := deleted.MapSet(NewInt(16), NewInt(100)).MapSet(NewInt(31), NewInt(200))
	for _, test := range []struct {
		mapValue Value
		want     []int64
	}{
		{base, []int64{31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0}},
		{deleted, []int64{30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0}},
		{changed, []int64{30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 31}},
	} {
		pairs := test.mapValue.PairsInInsertionOrder()
		if len(pairs) != len(test.want) {
			t.Fatalf("got %d insertion entries, want %d", len(pairs), len(test.want))
		}
		for i, pair := range pairs {
			if got := pair[0].Int(); got != test.want[i] {
				t.Fatalf("key %d = %d, want %d", i, got, test.want[i])
			}
		}
		if !test.mapValue.Identical(NewMap(pairs)) {
			t.Fatal("map changed after reconstructing its insertion-order snapshot")
		}
	}
	if got, _ := base.MapGet(NewInt(16)); got.Int() != 16 {
		t.Fatalf("overwrite changed original snapshot: %v", got)
	}
	if changed.Identical(NewMap(changed.Pairs())) {
		t.Fatal("Identical ignored insertion order")
	}
}

func BenchmarkMapDeleteOldest(b *testing.B) {
	m := NewEmptyMap()
	for i := range 2_000 {
		m = m.MapSet(NewInt(int64(i)), NewInt(int64(i)))
	}
	key := NewInt(0)
	b.ReportAllocs()
	for b.Loop() {
		if value := m.MapDelete(key); value.Len() != 1_999 {
			b.Fatal("delete did not remove the key")
		}
	}
}
