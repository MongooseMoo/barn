package types

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// flatTestKeys is a pool wide enough to take a map past mapFlatLimit, with
// keys that differ only in ways the key rules fold together (string case,
// signed zero) or keep apart (int 1, float 1.0, "1", #1).
func flatTestKeys() []Value {
	keys := []Value{
		NewInt(0), NewInt(1), NewInt(-1), NewInt(math.MaxInt64),
		NewFloat(0), NewFloat(math.Copysign(0, -1)), NewFloat(1), NewFloat(math.NaN()),
		NewObj(0), NewObj(1), NewObj(-1),
		NewStr(""), NewStr("1"), NewStr("name"), NewStr("Name"), NewStr("NAME"), NewStr("naïve"), NewStr("NAÏVE"),
		NewErr(E_PERM), NewErr(E_TYPE),
		NewBool(true), NewBool(false),
		NewWaif(1, 2), NewWaif(1, 2),
	}
	for i := 0; i < 12; i++ {
		keys = append(keys, NewStr(fmt.Sprintf("k%d", i)))
	}
	return keys
}

func TestMapKeyMatchesAgreesWithKeyHash(t *testing.T) {
	keys := flatTestKeys()
	for _, a := range keys {
		for _, b := range keys {
			want := keyHash(a) == keyHash(b)
			if got := mapKeyMatches(a, b); got != want {
				t.Errorf("mapKeyMatches(%v, %v) = %v, keyHash equality = %v", a, b, got, want)
			}
			if want && flatKeyHash(a) != flatKeyHash(b) {
				t.Errorf("flatKeyHash differs for %v and %v, which are the same key", a, b)
			}
		}
	}
}

func TestSmallMapsAreFlatAndLargeMapsAreIndexed(t *testing.T) {
	pairs := make([][2]Value, mapFlatLimit+1)
	for i := range pairs {
		pairs[i] = [2]Value{NewInt(int64(i)), NewInt(int64(i))}
	}

	small := NewMap(pairs[:mapFlatLimit])
	if small.goMap().index != nil || small.goMap().order != nil {
		t.Fatalf("a map of %d pairs is indexed, want flat", mapFlatLimit)
	}
	// A larger map built whole is flat with a sorted hash index, and becomes
	// indexed on its first write, which leaves the map it came from alone.
	large := NewMap(pairs)
	if large.goMap().index != nil || large.goMap().bulk == nil {
		t.Fatalf("a map of %d pairs built whole is not in the bulk flat form", mapFlatLimit+1)
	}
	for name, written := range map[string]Value{
		"replacing a pair": large.MapSet(NewInt(0), NewInt(9)),
		"adding a pair":    large.MapSet(NewInt(99), NewInt(9)),
		"deleting a pair":  large.MapDelete(NewInt(0)),
	} {
		if written.goMap().index == nil || written.goMap().flat != nil {
			t.Fatalf("%s in a bulk map left it flat", name)
		}
	}
	if large.goMap().index != nil || large.Len() != mapFlatLimit+1 {
		t.Fatalf("writing to a bulk map changed the map it was written from")
	}
	if same := large.MapDelete(NewInt(99)); same.goMap() != large.goMap() {
		t.Fatalf("deleting an absent key from a bulk map built a new map")
	}
	// A repeated key cannot be held flat: the map is built indexed.
	repeated := append(append([][2]Value(nil), pairs...), [2]Value{NewInt(3), NewInt(33)})
	if m := NewMap(repeated); m.goMap().index == nil || m.Len() != mapFlatLimit+1 {
		t.Fatalf("a map built whole with a repeated key: indexed=%v len=%d", m.goMap().index != nil, m.Len())
	}

	grown := small.MapSet(NewInt(int64(mapFlatLimit)), NewInt(0))
	if grown.goMap().index == nil || grown.goMap().flat != nil {
		t.Fatalf("setting pair %d left the map flat", mapFlatLimit+1)
	}
	if small.goMap().index != nil || small.Len() != mapFlatLimit {
		t.Fatalf("growing a flat map changed the map it grew from")
	}
	if replaced := small.MapSet(NewInt(0), NewInt(9)); replaced.goMap().index != nil {
		t.Fatalf("replacing a pair in a full flat map indexed it")
	}
}

func TestFlatMapUpdatesDoNotChangeTheSourceMap(t *testing.T) {
	source := NewMap([][2]Value{{NewStr("a"), NewInt(1)}, {NewStr("b"), NewInt(2)}})
	before := source.PairsInInsertionOrder()

	source.MapSet(NewStr("A"), NewInt(10))
	source.MapSet(NewStr("c"), NewInt(3))
	source.MapDelete(NewStr("a"))

	after := source.PairsInInsertionOrder()
	if len(after) != len(before) {
		t.Fatalf("source map now has %d pairs, want %d", len(after), len(before))
	}
	for i := range before {
		if !before[i][0].Identical(after[i][0]) || !before[i][1].Identical(after[i][1]) {
			t.Fatalf("source pair %d changed: %v -> %v", i, before[i], after[i])
		}
	}
}

// forcedIndexed returns v's contents in the indexed form whatever its size, as
// the reference the flat form is compared with.
func forcedIndexed(v Value) Value {
	m := v.goMap()
	if m.index == nil && m.count > 0 {
		return mapValue(m.indexed())
	}
	return v
}

func TestFlatAndIndexedMapsBehaveTheSame(t *testing.T) {
	keys := flatTestKeys()
	for seed := int64(1); seed <= 60; seed++ {
		rng := rand.New(rand.NewSource(seed))
		subject := NewEmptyMap()
		reference := NewEmptyMap()
		// Odd seeds start from a map built whole instead of an empty one:
		// random pairs with repeats, or (every fourth seed) distinct keys
		// past mapFlatLimit, which gives the bulk flat form.
		if seed%2 == 1 {
			var pairs [][2]Value
			if seed%4 == 1 {
				for i, at := range rng.Perm(len(keys))[:mapFlatLimit+1+rng.Intn(8)] {
					// The pool has keys that are one key under the map's
					// rules; MapSet on the reference folds them, so skip
					// any that match one already taken.
					taken := false
					for _, p := range pairs {
						taken = taken || mapKeyMatches(p[0], keys[at])
					}
					if !taken {
						pairs = append(pairs, [2]Value{keys[at], NewInt(int64(i))})
					}
				}
			} else {
				for i := 0; i < rng.Intn(mapFlatLimit+4); i++ {
					pairs = append(pairs, [2]Value{keys[rng.Intn(len(keys))], NewInt(int64(i))})
				}
			}
			subject = NewMap(pairs)
			reference = forcedIndexed(NewEmptyMap())
			for _, p := range pairs {
				reference = forcedIndexed(reference.MapSet(p[0], p[1]))
			}
			// Before any write: the only point where a bulk map is read.
			compareMapForms(t, seed, -1, subject, reference, keys)
		}

		for step := 0; step < 250; step++ {
			key := keys[rng.Intn(len(keys))]
			// Deletes dominate once the map is large so it also shrinks back
			// through mapFlatLimit.
			deleteOdds := 3
			if subject.Len() > mapFlatLimit+2 {
				deleteOdds = 7
			}
			if rng.Intn(10) < deleteOdds {
				subject = subject.MapDelete(key)
				reference = forcedIndexed(reference.MapDelete(key))
			} else {
				val := Value(NewInt(int64(step)))
				if rng.Intn(8) == 0 {
					val = NewList([]Value{NewWaif(1, 2)})
				}
				subject = subject.MapSet(key, val)
				reference = forcedIndexed(reference.MapSet(key, val))
			}
			compareMapForms(t, seed, step, subject, reference, keys)
			if t.Failed() {
				return
			}
		}
	}
}

func compareMapForms(t *testing.T, seed int64, step int, subject, reference Value, keys []Value) {
	t.Helper()
	where := fmt.Sprintf("seed %d step %d", seed, step)

	if subject.Len() != reference.Len() {
		t.Fatalf("%s: Len %d, reference %d", where, subject.Len(), reference.Len())
	}
	for name, pairsOf := range map[string]func(Value) [][2]Value{
		"PairsInInsertionOrder": Value.PairsInInsertionOrder,
		"Pairs":                 Value.Pairs,
	} {
		got, want := pairsOf(subject), pairsOf(reference)
		if len(got) != len(want) {
			t.Fatalf("%s: %s has %d pairs, reference %d", where, name, len(got), len(want))
		}
		for i := range want {
			if !got[i][0].Identical(want[i][0]) || !got[i][1].Identical(want[i][1]) {
				t.Fatalf("%s: %s[%d] = %v, reference %v", where, name, i, got[i], want[i])
			}
		}
	}
	for _, key := range keys {
		got, gotOK := subject.MapGet(key)
		want, wantOK := reference.MapGet(key)
		if gotOK != wantOK || !got.Identical(want) {
			t.Fatalf("%s: MapGet(%v) = (%v, %v), reference (%v, %v)", where, key, got, gotOK, want, wantOK)
		}
		for _, caseSensitive := range []bool{false, true} {
			got, gotOK := subject.GetWithCase(key, caseSensitive)
			want, wantOK := reference.GetWithCase(key, caseSensitive)
			if gotOK != wantOK || !got.Identical(want) {
				t.Fatalf("%s: GetWithCase(%v, %v) = (%v, %v), reference (%v, %v)", where, key, caseSensitive, got, gotOK, want, wantOK)
			}
		}
		if got, want := subject.KeyPosition(key), reference.KeyPosition(key); got != want {
			t.Fatalf("%s: KeyPosition(%v) = %d, reference %d", where, key, got, want)
		}
	}
	if got, want := ValueBytes(subject), ValueBytes(reference); got != want {
		t.Fatalf("%s: ValueBytes %d, reference %d", where, got, want)
	}
	if got, want := subject.String(), reference.String(); got != want {
		t.Fatalf("%s: literal %s, reference %s", where, got, want)
	}
	if got, want := subject.MayHoldFinalizable(), reference.MayHoldFinalizable(); got != want {
		t.Fatalf("%s: MayHoldFinalizable %v, reference %v", where, got, want)
	}
	// A map holding a NaN key is not Equal to itself, so compare with what
	// the reference says of itself.
	selfEqual := reference.Equal(reference)
	if subject.Equal(reference) != selfEqual || reference.Equal(subject) != selfEqual || subject.Equal(subject) != selfEqual {
		t.Fatalf("%s: Equal between the two forms differs from the reference's Equal to itself (%v)", where, selfEqual)
	}
	if !subject.Identical(reference) || !reference.Identical(subject) {
		t.Fatalf("%s: Identical is false between the two forms", where)
	}
}

func BenchmarkFlatMapGet(b *testing.B) {
	for _, size := range []int{1, 4, 8, 16} {
		for _, form := range []string{"flat", "indexed"} {
			pairs := make([][2]Value, size)
			for i := range pairs {
				pairs[i] = [2]Value{NewStr(fmt.Sprintf("property_name_%d", i)), NewInt(int64(i))}
			}
			m := NewMap(pairs)
			if form == "indexed" {
				m = forcedIndexed(m)
			}
			last := NewStr(fmt.Sprintf("property_name_%d", size-1))
			missing := NewStr("property_name_x")
			b.Run(fmt.Sprintf("%s/size=%d/last", form, size), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					mapBenchmarkResult, _ = m.MapGet(last)
				}
			})
			b.Run(fmt.Sprintf("%s/size=%d/missing", form, size), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					mapBenchmarkResult, _ = m.MapGet(missing)
				}
			})
		}
	}
}
