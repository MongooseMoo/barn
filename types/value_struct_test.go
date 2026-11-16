package types

import (
	"math"
	"testing"
)

// --- keyHash / map-key distinctness (LANDMINE 1) -------------------------

// TestKeyHashDistinctAcrossTypes proves that int 1, float 1.0, and string "1"
// hash to three DISTINCT map keys. On the old interface representation this held
// because keyHash used %T (the Go dynamic type). After collapsing every value to
// one struct type, %T is constant, so keyHash MUST namespace by v.Type() instead.
func TestKeyHashDistinctAcrossTypes(t *testing.T) {
	hInt := keyHash(NewInt(1))
	hFloat := keyHash(NewFloat(1.0))
	hStr := keyHash(NewStr("1"))

	if hInt == hFloat {
		t.Errorf("int 1 and float 1.0 hash to the same key %v", hInt)
	}
	if hInt == hStr {
		t.Errorf("int 1 and str \"1\" hash to the same key %v", hInt)
	}
	if hFloat == hStr {
		t.Errorf("float 1.0 and str \"1\" hash to the same key %v", hFloat)
	}
}

// TestMapKeepsThreeDistinctEntries is the end-to-end version: a MOO map with
// keys int 1, float 1.0, and string "1" must keep three separate entries.
func TestMapKeepsThreeDistinctEntries(t *testing.T) {
	m := NewMap([][2]Value{
		{NewInt(1), NewStr("int-one")},
		{NewFloat(1.0), NewStr("float-one")},
		{NewStr("1"), NewStr("str-one")},
	})
	if got := m.Len(); got != 3 {
		t.Fatalf("expected 3 distinct entries, got %d", got)
	}
	if v, ok := m.MapGet(NewInt(1)); !ok || v.Str() != "int-one" {
		t.Errorf("int key lookup failed: %v ok=%v", v, ok)
	}
	if v, ok := m.MapGet(NewFloat(1.0)); !ok || v.Str() != "float-one" {
		t.Errorf("float key lookup failed: %v ok=%v", v, ok)
	}
	if v, ok := m.MapGet(NewStr("1")); !ok || v.Str() != "str-one" {
		t.Errorf("str key lookup failed: %v ok=%v", v, ok)
	}
}

func TestMapKeepsAdjacentFloatKeysDistinct(t *testing.T) {
	one := NewFloat(1.0)
	next := NewFloat(math.Nextafter(1.0, 2.0))
	m := NewMap([][2]Value{
		{one, NewStr("one")},
		{next, NewStr("next")},
	})

	if got := m.Len(); got != 2 {
		t.Fatalf("adjacent float keys collapsed to %d entry, want 2", got)
	}
	if v, ok := m.MapGet(one); !ok || v.Str() != "one" {
		t.Errorf("1.0 key lookup = %v ok=%v, want one", v, ok)
	}
	if v, ok := m.MapGet(next); !ok || v.Str() != "next" {
		t.Errorf("next float key lookup = %v ok=%v, want next", v, ok)
	}
}

func TestMapIntegerKeysRemainDistinctAt32BitBoundary(t *testing.T) {
	zero := NewInt(0)
	wide := NewInt(1 << 32)
	m := NewMap([][2]Value{
		{zero, NewStr("zero")},
		{wide, NewStr("wide")},
	})

	if got := m.Len(); got != 2 {
		t.Fatalf("map length = %d, want 2", got)
	}
	if got, ok := m.MapGet(zero); !ok || got.Str() != "zero" {
		t.Fatalf("zero lookup = %v, %v; want zero, true", got, ok)
	}
	if got, ok := m.MapGet(wide); !ok || got.Str() != "wide" {
		t.Fatalf("wide lookup = %v, %v; want wide, true", got, ok)
	}
	keys := m.Keys()
	if len(keys) != 2 || keys[0].Int() != 0 || keys[1].Int() != 1<<32 {
		t.Fatalf("map keys = %v, want {0, 4294967296}", keys)
	}

	m = m.MapSet(zero, NewStr("replacement"))
	if got := m.Len(); got != 2 {
		t.Fatalf("replacement map length = %d, want 2", got)
	}
	if got, ok := m.MapGet(zero); !ok || got.Str() != "replacement" {
		t.Fatalf("replacement zero lookup = %v, %v; want replacement, true", got, ok)
	}
	if got, ok := m.MapGet(wide); !ok || got.Str() != "wide" {
		t.Fatalf("replacement wide lookup = %v, %v; want wide, true", got, ok)
	}
	keys = m.Keys()
	if len(keys) != 2 || keys[0].Int() != 0 || keys[1].Int() != 1<<32 {
		t.Fatalf("replacement map keys = %v, want {0, 4294967296}", keys)
	}
}

func TestMapObjectKeysRemainDistinctAt32BitBoundary(t *testing.T) {
	small := NewObj(0)
	large := NewObj(1 << 32)
	if small.Equal(large) {
		t.Fatal("full-width object values compare equal outside map storage")
	}
	m := NewMap([][2]Value{
		{small, NewStr("small")},
		{large, NewStr("large")},
	})

	if got := m.Len(); got != 2 {
		t.Fatalf("map length = %d, want 2", got)
	}
	if got, ok := m.MapGet(small); !ok || got.Str() != "small" {
		t.Fatalf("small lookup = %v, %v; want small, true", got, ok)
	}
	if got, ok := m.MapGet(large); !ok || got.Str() != "large" {
		t.Fatalf("large lookup = %v, %v; want large, true", got, ok)
	}
	keys := m.Keys()
	if len(keys) != 2 || keys[0].Obj() != 0 || keys[1].Obj() != 1<<32 {
		t.Fatalf("map keys = %v, want {#0, #4294967296}", keys)
	}
}

func TestWaifMapKeyOrderIsIndependentOfInsertionOrder(t *testing.T) {
	keys := make([]Value, 8)
	for i := range keys {
		keys[i] = NewWaif(1, 2)
	}
	if !IsValidMapKey(keys[0]) || !IsValidBuiltinMapKey(keys[0]) {
		t.Fatal("waif is not admitted as a map key")
	}
	if keyHash(keys[0]) == keyHash(keys[1]) {
		t.Fatalf("distinct waifs have the same key hash %v", keyHash(keys[0]))
	}
	assertIdentityKeyOrderIsTotal(t, keys)
}

func TestAnonymousMapKeyOrderIsIndependentOfInsertionOrder(t *testing.T) {
	keys := make([]Value, 8)
	for i := range keys {
		keys[i] = NewAnon(ObjID(i + 1))
	}
	if !IsValidMapKey(keys[0]) || IsValidBuiltinMapKey(keys[0]) {
		t.Fatal("anonymous map-key admission does not match Toast")
	}
	assertIdentityKeyOrderIsTotal(t, keys)
}

// assertIdentityKeyOrderIsTotal checks Toast 2.8's compare() contract for
// waif and anonymous-object keys (c0966e5): distinct keys order one way
// round, so the map's key order, literal and equality do not depend on the
// order the keys were inserted in, and the tree lookup finds every key.
func assertIdentityKeyOrderIsTotal(t *testing.T, keys []Value) {
	t.Helper()
	for i, a := range keys {
		for j, b := range keys {
			ab, ba := toastMapCompare(a, b, false), toastMapCompare(b, a, false)
			if (i == j) != (ab == 0) || (ab < 0) != (ba > 0) {
				t.Fatalf("toastMapCompare(keys[%d], keys[%d]) = %d, reverse %d: not a total order", i, j, ab, ba)
			}
		}
	}

	forward := make([][2]Value, len(keys))
	reverse := make([][2]Value, len(keys))
	for i, key := range keys {
		forward[i] = [2]Value{key, NewInt(int64(i))}
		reverse[len(keys)-1-i] = forward[i]
	}
	forwardMap, reverseMap := NewMap(forward), NewMap(reverse)

	forwardKeys, reverseKeys := forwardMap.Keys(), reverseMap.Keys()
	if len(forwardKeys) != len(keys) || len(reverseKeys) != len(keys) {
		t.Fatalf("key counts = %d, %d; want %d", len(forwardKeys), len(reverseKeys), len(keys))
	}
	for i := range forwardKeys {
		if !forwardKeys[i].Equal(reverseKeys[i]) {
			t.Fatalf("key %d differs between insertion orders", i+1)
		}
		if i > 0 && toastMapCompare(forwardKeys[i-1], forwardKeys[i], false) >= 0 {
			t.Fatalf("keys %d and %d are out of order", i, i+1)
		}
	}
	if !forwardMap.Equal(reverseMap) {
		t.Fatal("maps with the same identity-key pairs compare unequal")
	}
	for i, key := range keys {
		for _, m := range []Value{forwardMap, reverseMap} {
			if got, ok := m.GetWithCase(key, true); !ok || got.Int() != int64(i) {
				t.Fatalf("GetWithCase(keys[%d]) = %v, %v; want %d, true", i, got, ok, i)
			}
		}
	}
}

// --- int round-trip + zero-alloc ----------------------------------------

func TestIntRoundTrip(t *testing.T) {
	v := NewInt(42)
	if v.Type() != TYPE_INT {
		t.Errorf("Type() = %v, want TYPE_INT", v.Type())
	}
	if v.Int() != 42 {
		t.Errorf("Int() = %d, want 42", v.Int())
	}
	if !v.Truthy() {
		t.Error("NewInt(42).Truthy() should be true")
	}
	if NewInt(0).Truthy() {
		t.Error("NewInt(0).Truthy() should be false")
	}
	if v.String() != "42" {
		t.Errorf("String() = %q, want \"42\"", v.String())
	}
	// Negative round-trip exercises the uint64 bit-reinterpretation.
	if NewInt(-7).Int() != -7 {
		t.Errorf("NewInt(-7).Int() = %d, want -7", NewInt(-7).Int())
	}
}

// TestIntZeroAlloc asserts that constructing and reading a scalar int Value does
// not touch the heap. This is the entire point of the de-box: scalars live
// inline in the struct's n word with no boxing.
func TestIntZeroAlloc(t *testing.T) {
	var sink int64
	allocs := testing.AllocsPerRun(1000, func() {
		v := NewInt(12345)
		sink += v.Int()
	})
	if sink == 0 {
		t.Fatal("sink unused")
	}
	if allocs != 0 {
		t.Errorf("constructing+reading an int Value allocated %v times, want 0", allocs)
	}
}

// --- float round-trip ----------------------------------------------------

func TestFloatRoundTrip(t *testing.T) {
	cases := []float64{0.0, 1.5, -3.25, math.Pi, math.MaxFloat64, math.SmallestNonzeroFloat64}
	for _, f := range cases {
		v := NewFloat(f)
		if v.Type() != TYPE_FLOAT {
			t.Errorf("Type() = %v, want TYPE_FLOAT", v.Type())
		}
		if v.Float() != f {
			t.Errorf("NewFloat(%v).Float() = %v, not bit-exact", f, v.Float())
		}
	}
	if NewFloat(0.0).Truthy() {
		t.Error("NewFloat(0.0).Truthy() should be false")
	}
	if !NewFloat(1.5).Truthy() {
		t.Error("NewFloat(1.5).Truthy() should be true")
	}
	// NaN preserves its bits and is non-equal to itself under Equal.
	nan := NewFloat(math.NaN())
	if !math.IsNaN(nan.Float()) {
		t.Error("NaN did not round-trip")
	}
	if nan.Equal(nan) {
		t.Error("NaN.Equal(NaN) should be false (IEEE 754)")
	}
}

func TestNegativeZeroFloatLiteralPreservesSign(t *testing.T) {
	negativeZero := math.Copysign(0, -1)
	if got := NewFloat(negativeZero).String(); got != "-0.0" {
		t.Fatalf("negative zero literal = %q, want %q", got, "-0.0")
	}
}

// --- cross-type equality -------------------------------------------------

func TestEqualityAcrossTypes(t *testing.T) {
	one := NewInt(1)
	oneF := NewFloat(1.0)
	oneS := NewStr("1")

	if one.Equal(oneF) {
		t.Error("int 1 should not Equal float 1.0")
	}
	if one.Equal(oneS) {
		t.Error("int 1 should not Equal str \"1\"")
	}
	if oneF.Equal(oneS) {
		t.Error("float 1.0 should not Equal str \"1\"")
	}
	if !one.Equal(NewInt(1)) {
		t.Error("int 1 should Equal int 1")
	}
	if !oneF.Equal(NewFloat(1.0)) {
		t.Error("float 1.0 should Equal float 1.0")
	}
	// MOO strings compare case-insensitively.
	if !NewStr("Foo").Equal(NewStr("foo")) {
		t.Error("MOO strings should compare case-insensitively")
	}
}

// --- None / zero-value sentinel (LANDMINE 2) -----------------------------

func TestNoneSentinel(t *testing.T) {
	if !None.IsNone() {
		t.Error("None.IsNone() should be true")
	}
	if NewInt(0).IsNone() {
		t.Error("NewInt(0).IsNone() should be false (it is a valid integer 0)")
	}
	// The struct zero value has tag TYPE_INT (0), i.e. integer 0 — NOT None.
	// Absence must be represented explicitly with None. This documents/locks
	// that decision: we did not (and cannot, without renumbering persisted
	// TypeCodes) make Value{} mean None.
	var zero Value
	if zero.IsNone() {
		t.Error("zero Value{} must NOT be None (its tag is TYPE_INT==0)")
	}
	if zero.Type() != TYPE_INT {
		t.Errorf("zero Value{}.Type() = %v, want TYPE_INT", zero.Type())
	}
	if !None.Equal(None) {
		t.Error("None.Equal(None) should be true")
	}
	if None.Equal(NewInt(0)) {
		t.Error("None should not Equal integer 0")
	}
}

func TestUnboundSentinel(t *testing.T) {
	if !Unbound.IsUnbound() {
		t.Error("Unbound.IsUnbound() should be true")
	}
	if NewInt(0).IsUnbound() {
		t.Error("NewInt(0).IsUnbound() should be false")
	}
	// Preserve old UnboundValue behavior: type is not externally observable and
	// reports TYPE_INT.
	if Unbound.Type() != TYPE_INT {
		t.Errorf("Unbound.Type() = %v, want TYPE_INT", Unbound.Type())
	}
}

// --- obj / err / bool accessors -----------------------------------------

func TestScalarAccessors(t *testing.T) {
	o := NewObj(5)
	if o.Type() != TYPE_OBJ || o.Obj() != 5 || o.ID() != 5 {
		t.Errorf("obj accessors wrong: type=%v obj=%v", o.Type(), o.Obj())
	}
	if o.String() != "#5" {
		t.Errorf("obj String() = %q, want #5", o.String())
	}
	a := NewAnon(7)
	if a.Type() != TYPE_ANON || !a.IsAnonymous() || a.Obj() != 7 {
		t.Errorf("anon accessors wrong: type=%v anon=%v", a.Type(), a.IsAnonymous())
	}
	// Negative object ids round-trip through the uint64 payload.
	if NewObj(NOTHING).Obj() != NOTHING {
		t.Errorf("NewObj(NOTHING).Obj() = %v, want %v", NewObj(NOTHING).Obj(), NOTHING)
	}
	e := NewErr(E_TYPE)
	if e.Type() != TYPE_ERR || e.ErrCode() != E_TYPE || e.Code() != E_TYPE {
		t.Errorf("err accessors wrong: type=%v code=%v", e.Type(), e.ErrCode())
	}
	b := NewBool(true)
	if b.Type() != TYPE_BOOL || !b.Bool() || !b.Truthy() {
		t.Errorf("bool accessors wrong")
	}
	if NewBool(false).Truthy() {
		t.Error("NewBool(false).Truthy() should be false")
	}
}

// TestWaifIdentity verifies the waif-identity accessor added for the db/store
// live-waif registry: distinct waifs have distinct identities, and copies of the
// same waif Value share a stable identity (reference semantics).
func TestWaifIdentity(t *testing.T) {
	w1 := NewWaif(5, 2)
	w2 := NewWaif(5, 2)
	if w1.WaifIdentity() == w2.WaifIdentity() {
		t.Error("distinct waifs must have distinct identities")
	}
	w1copy := w1
	if w1.WaifIdentity() != w1copy.WaifIdentity() {
		t.Error("a copy of a waif Value must share its identity")
	}
	// Mutating through the copy is visible via the original (shared payload),
	// confirming identity tracks the shared heap rep.
	w1copy.SetProperty("x", NewInt(7))
	if got, ok := w1.GetProperty("x"); !ok || !got.Equal(NewInt(7)) {
		t.Error("waif identity copies must share the property map")
	}
}

func TestWaifIdentitySerialization(t *testing.T) {
	original := NewWaif(5, 2)
	encoded := original.WaifIdentity().String()
	if len(encoded) != 32 {
		t.Fatalf("serialized identity length = %d, want 32", len(encoded))
	}

	identity, err := ParseWaifIdentity(encoded)
	if err != nil {
		t.Fatalf("ParseWaifIdentity: %v", err)
	}
	restored := NewWaifWithIdentity(original.Class(), original.Owner(), identity)
	if restored.WaifIdentity() != original.WaifIdentity() {
		t.Fatal("restored WAIF did not preserve identity")
	}
	if !restored.Equal(original) {
		t.Fatal("WAIF equality did not follow restored identity")
	}
}

func TestWaifMapLookupDoesNotAllocate(t *testing.T) {
	waif := NewWaif(5, 2)
	m := NewMap([][2]Value{{waif, NewInt(7)}})

	allocs := testing.AllocsPerRun(1000, func() {
		if value, ok := m.MapGet(waif); !ok || value.Int() != 7 {
			panic("waif map lookup failed")
		}
	})
	if allocs != 0 {
		t.Fatalf("waif map lookup allocated %v times per run, want 0", allocs)
	}
}
