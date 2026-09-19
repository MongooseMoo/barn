package types

import (
	"math/rand"
	"sync"
	"testing"
)

func TestMapUpdatesPreserveSnapshotsAndInsertionOrder(t *testing.T) {
	rng := rand.New(rand.NewSource(743))
	m := NewEmptyMap()
	reference := map[int64]int64{}
	var order []int64
	var snapshots []Value
	var expected [][][2]Value
	check := func(m Value, pairs [][2]Value) {
		t.Helper()
		got := m.PairsInInsertionOrder()
		if len(got) != len(pairs) || m.Len() != len(pairs) {
			t.Fatalf("map length %d/%d, want %d", len(got), m.Len(), len(pairs))
		}
		for i, p := range pairs {
			if !got[i][0].Identical(p[0]) || !got[i][1].Identical(p[1]) {
				t.Fatalf("insertion pair %d = %v, want %v", i, got[i], p)
			}
			value, ok := m.MapGet(p[0])
			if !ok || !value.Identical(p[1]) {
				t.Fatalf("lookup %v = %v/%v", p[0], value, ok)
			}
		}
		if ValueBytes(m) != ValueBytes(NewMap(pairs)) {
			t.Fatal("cached size changed")
		}
	}
	for i := 0; i < 3000; i++ {
		key := int64(rng.Intn(120))
		if rng.Intn(4) == 0 {
			m = m.MapDelete(NewInt(key))
			delete(reference, key)
			for j, k := range order {
				if k == key {
					order = append(order[:j], order[j+1:]...)
					break
				}
			}
			if _, ok := m.MapGet(NewInt(key)); ok {
				t.Fatalf("deleted key %d remains", key)
			}
		} else {
			value := int64(i)
			if _, exists := reference[key]; !exists {
				order = append(order, key)
			}
			reference[key] = value
			m = m.MapSet(NewInt(key), NewInt(value))
		}
		if i%31 == 0 {
			pairs := make([][2]Value, len(order))
			for j, k := range order {
				pairs[j] = [2]Value{NewInt(k), NewInt(reference[k])}
			}
			check(m, pairs)
			// Force the lazy Toast tree too, before later aliases change.
			_ = m.Pairs()
			snapshots = append(snapshots, m)
			expected = append(expected, pairs)
		}
	}
	for i, snapshot := range snapshots {
		check(snapshot, expected[i])
	}
}

func TestMapOverwriteCaseAndDeleteReinsert(t *testing.T) {
	base := NewMap([][2]Value{{NewStr("A"), NewInt(1)}, {NewStr("b"), NewInt(2)}})
	updated := base.MapSet(NewStr("a"), NewInt(3))
	if got := updated.PairsInInsertionOrder(); len(got) != 2 || got[0][0].Str() != "a" || got[0][1].Int() != 3 {
		t.Fatalf("overwrite: %v", got)
	}
	if got := base.PairsInInsertionOrder(); got[0][0].Str() != "A" || got[0][1].Int() != 1 {
		t.Fatalf("alias mutated: %v", got)
	}
	reinserted := updated.MapDelete(NewStr("A")).MapSet(NewStr("A"), NewInt(4))
	if got := reinserted.PairsInInsertionOrder(); len(got) != 2 || got[0][0].Str() != "b" || got[1][0].Str() != "A" {
		t.Fatalf("reinsert: %v", got)
	}
}

func TestMapIndexFullHashCollision(t *testing.T) {
	a, b := keyHash(NewInt(1)), keyHash(NewInt(2))
	// At depth 16 the hash has been exhausted: only exact typed keys decide.
	n := mapIndexSet(nil, a, mapEntry{NewInt(1), NewInt(10)}, 0, 16, true)
	collision := mapIndexSet(n, b, mapEntry{NewInt(2), NewInt(20)}, 0, 16, true)
	updated := mapIndexSet(collision, a, mapEntry{NewInt(1), NewInt(30)}, 0, 16, true)
	for _, tc := range []struct {
		n    *mapIndex
		key  mapHash
		want int64
	}{{n, a, 10}, {collision, a, 10}, {collision, b, 20}, {updated, a, 30}} {
		got, ok := mapIndexGet(tc.n, tc.key, 0)
		if !ok || got.val.Int() != tc.want {
			t.Fatalf("collision lookup %v/%v want %d", got, ok, tc.want)
		}
	}
	deleted := mapIndexDelete(updated, a, 0)
	if _, ok := mapIndexGet(deleted, a, 0); ok {
		t.Fatal("deleted collision key remains")
	}
	if got, ok := mapIndexGet(deleted, b, 0); !ok || got.val.Int() != 20 {
		t.Fatal("deleted wrong collision entry")
	}
}

func TestMapConcurrentAliasUpdates(t *testing.T) {
	base := NewMap([][2]Value{{NewStr("a"), NewInt(1)}, {NewStr("b"), NewInt(2)}})
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				updated := base.MapSet(NewInt(int64(i)), NewInt(int64(i)))
				_ = base.MayHoldFinalizable()
				_ = updated.Pairs()
				if updated.MapDelete(NewStr("a")).Len() != 2 {
					t.Error("bad derived length")
				}
			}
		}()
	}
	wg.Wait()
	if base.Len() != 2 {
		t.Fatal("shared map changed")
	}
}

func TestMapBulkBuilderPublishesIndependentLeaves(t *testing.T) {
	pairs := make([][2]Value, 513)
	for i := range pairs {
		pairs[i] = [2]Value{NewInt(int64(i)), NewInt(int64(i))}
	}
	// Duplicate replacement must not add an order record or retain its old value.
	pairs = append(pairs, [2]Value{NewInt(256), NewInt(999)})
	base := NewMap(pairs)
	updated := base
	for i := 0; i < 513; i++ {
		updated = updated.MapSet(NewInt(int64(i)), NewInt(-1))
		if i%2 == 0 {
			updated = updated.MapDelete(NewInt(int64(i)))
		}
	}
	if base.Len() != 513 || updated.Len() != 256 {
		t.Fatalf("lengths %d/%d", base.Len(), updated.Len())
	}
	for i := 0; i < 513; i++ {
		want := int64(i)
		if i == 256 {
			want = 999
		}
		if got, ok := base.MapGet(NewInt(int64(i))); !ok || got.Int() != want {
			t.Fatalf("bulk alias key %d = %v/%v", i, got, ok)
		}
		got, ok := updated.MapGet(NewInt(int64(i)))
		if ok != (i%2 == 1) || (ok && got.Int() != -1) {
			t.Fatalf("updated key %d = %v/%v", i, got, ok)
		}
	}
	order := base.PairsInInsertionOrder()
	for i, pair := range order {
		if pair[0].Int() != int64(i) {
			t.Fatalf("bulk order %d = %v", i, pair[0])
		}
	}
}

func TestMapFinalizationScanDoesNotAllocate(t *testing.T) {
	pairs := make([][2]Value, 2000)
	for i := range pairs {
		pairs[i] = [2]Value{NewInt(int64(i)), NewInt(int64(i))}
	}
	for _, tainted := range []bool{false, true} {
		if tainted {
			pairs[len(pairs)-1][1] = NewAnon(47)
		}
		m := NewMap(pairs).goMap()
		allocs := testing.AllocsPerRun(20, func() {
			// Force the first scan, not the cheap cached follow-up.
			m.finalizableOnce = sync.Once{}
			if got := m.mayHoldFinalizable(); got != tainted {
				t.Fatalf("finalizable=%v, want %v", got, tainted)
			}
		})
		if allocs != 0 {
			t.Fatalf("first finalization scan allocated %v times", allocs)
		}
	}
}
