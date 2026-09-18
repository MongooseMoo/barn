package types

import "testing"

func TestMayHoldFinalizableScalars(t *testing.T) {
	for _, v := range []Value{NewInt(1), NewStr("x"), NewFloat(1.5), NewObj(3), NewErr(E_TYPE), NewBool(true), None} {
		if v.MayHoldFinalizable() {
			t.Fatalf("%v should not be finalizable", v)
		}
	}
	if !NewAnon(7).MayHoldFinalizable() {
		t.Fatal("anonymous object must be finalizable")
	}
}

func TestMayHoldFinalizableListPropagation(t *testing.T) {
	l := NewEmptyList()
	if l.sliceList().finalizable != finalizableNone {
		t.Fatal("empty list should start proven-clean")
	}
	for i := 0; i < 100; i++ {
		l = l.Append(NewInt(int64(i)))
		if l.sliceList().finalizable != finalizableNone {
			t.Fatalf("append %d of an int lost the clean proof", i)
		}
	}
	if l.MayHoldFinalizable() {
		t.Fatal("list of ints reported finalizable")
	}

	tainted := l.Append(NewAnon(9))
	if !tainted.MayHoldFinalizable() {
		t.Fatal("appending an anon must taint the list")
	}
	if l.MayHoldFinalizable() {
		t.Fatal("original header must not be tainted by a sibling append")
	}

	// Set/Concat/InsertAt/Slice carry the taint conservatively.
	if !tainted.Set(1, NewInt(0)).MayHoldFinalizable() {
		t.Fatal("Set on a tainted list must stay conservative")
	}
	if !l.Concat(tainted).MayHoldFinalizable() || l.Concat(l).MayHoldFinalizable() {
		t.Fatal("Concat taint propagation wrong")
	}
	if !l.InsertAt(1, NewAnon(1)).MayHoldFinalizable() {
		t.Fatal("InsertAt of an anon must taint")
	}
	if l.Slice(1, 5).MayHoldFinalizable() {
		t.Fatal("Slice of a clean list must stay clean")
	}

	// Removing the anon leaves the cache unknown; a rescan must find it clean.
	cleaned := tainted.DeleteAt(tainted.Len())
	if cleaned.sliceList().finalizable != finalizableUnknown {
		t.Fatal("DeleteAt from a tainted list should reset to unknown")
	}
	if cleaned.MayHoldFinalizable() {
		t.Fatal("rescan after removing the anon should report clean")
	}
}

func TestMayHoldFinalizableNestedAndLazy(t *testing.T) {
	inner := NewList([]Value{NewInt(1), NewList([]Value{NewAnon(2)})})
	outer := NewList([]Value{NewStr("a"), inner})
	if !outer.MayHoldFinalizable() {
		t.Fatal("nested anon must be found by the lazy scan")
	}
	clean := NewList([]Value{NewInt(1), NewList([]Value{NewInt(2)})})
	if clean.MayHoldFinalizable() {
		t.Fatal("nested clean list reported finalizable")
	}
	if clean.sliceList().finalizable != finalizableNone {
		t.Fatal("lazy scan result should be cached")
	}
}

// A list built from an unscanned source (NewList, e.g. a `{}` literal) must
// resolve the source once and hand a proven-clean state to every derived
// header. Before this held, `l = {@l, x}` re-scanned the whole list at each
// SET_VAR because each appended header inherited Unknown from its parent.
func TestMayHoldFinalizableDerivedFromUnscannedSource(t *testing.T) {
	l := NewList([]Value{NewInt(1)})
	if l.sliceList().finalizable != finalizableUnknown {
		t.Fatal("NewList should start unscanned")
	}
	for i := 0; i < 100; i++ {
		l = l.Append(NewInt(int64(i)))
		if l.sliceList().finalizable != finalizableNone {
			t.Fatalf("append %d from an unscanned clean source did not yield a proven-clean header", i)
		}
	}
	for name, derived := range map[string]Value{
		"Set":      NewList([]Value{NewInt(1), NewInt(2)}).Set(1, NewInt(0)),
		"Slice":    NewList([]Value{NewInt(1), NewInt(2)}).Slice(1, 1),
		"Concat":   NewList([]Value{NewInt(1)}).Concat(NewList([]Value{NewInt(2)})),
		"InsertAt": NewList([]Value{NewInt(1)}).InsertAt(1, NewInt(0)),
		"DeleteAt": NewList([]Value{NewInt(1), NewInt(2)}).DeleteAt(1),
	} {
		if derived.sliceList().finalizable != finalizableNone {
			t.Fatalf("%s from an unscanned clean source should be proven clean", name)
		}
	}
	dirty := NewList([]Value{NewAnon(1)}).Append(NewInt(2))
	if dirty.sliceList().finalizable != finalizableMaybe {
		t.Fatal("append from an unscanned tainted source must resolve to tainted")
	}

	m := NewMap([][2]Value{{NewStr("a"), NewInt(1)}})
	if m.goMap().finalizable != finalizableUnknown {
		t.Fatal("NewMap should start unscanned")
	}
	for i := 0; i < 50; i++ {
		m = m.MapSet(NewInt(int64(i)), NewInt(int64(i*2)))
		if m.goMap().finalizable != finalizableNone {
			t.Fatalf("MapSet %d from an unscanned clean source did not yield a proven-clean map", i)
		}
	}
	if NewMap([][2]Value{{NewStr("a"), NewInt(1)}, {NewStr("b"), NewInt(2)}}).MapDelete(NewStr("a")).goMap().finalizable != finalizableNone {
		t.Fatal("MapDelete from an unscanned clean source should be proven clean")
	}
}

func TestMayHoldFinalizableMap(t *testing.T) {
	m := NewEmptyMap()
	if m.MayHoldFinalizable() {
		t.Fatal("empty map reported finalizable")
	}
	m = m.MapSet(NewStr("k"), NewInt(1))
	if m.MayHoldFinalizable() || m.goMap().finalizable != finalizableNone {
		t.Fatal("clean MapSet lost the proof")
	}
	tainted := m.MapSet(NewStr("a"), NewList([]Value{NewAnon(4)}))
	if !tainted.MayHoldFinalizable() {
		t.Fatal("map value holding an anon must taint")
	}
	if !NewMap([][2]Value{{NewAnon(5), NewInt(1)}}).MayHoldFinalizable() {
		t.Fatal("anon map key must taint")
	}
	back := tainted.MapDelete(NewStr("a"))
	if back.MayHoldFinalizable() {
		t.Fatal("deleting the tainted entry should rescan clean")
	}
}
