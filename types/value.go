package types

import (
	"math"
	"strings"
)

// Value is the unboxed representation of a MOO value: a tagged union held by
// value, so scalars (int/float/obj/err/bool) never escape to the heap. This
// replaces the former Value interface, whose per-scalar boxing dominated the VM
// hot path.
//
// Layout (safe, no unsafe): kind tag + scalar word + string + heap ref.
//   - num holds int64 bits, float64 bits, an ObjID, an ErrorCode, or a bool.
//   - str holds the string payload (KindStr).
//   - ref holds the heap payload for collections (MooList / MooMap / WaifValue).
//
// The zero value (Value{}) is KindNone, which represents an uninitialized /
// unbound MOO variable (Toast's TYPE_NONE). Making none the zero value defuses
// the TYPE_INT==0 hazard: fresh local slots are none, and any accidental
// zero-value is fail-loud (E_VARNF on read) rather than a silent integer 0.
type Value struct {
	kind Kind
	num  uint64
	str  string
	ref  any
}

// Kind is the internal type tag. It is distinct from TypeCode (the MOO-visible
// typeof() number) so that KindNone can be the zero value while TYPE_INT stays 0.
type Kind uint8

const (
	KindNone Kind = iota // zero value: uninitialized/unbound variable
	KindInt
	KindFloat
	KindStr
	KindObj
	KindAnon // anonymous object reference (typeof => TYPE_ANON)
	KindErr
	KindBool
	KindList
	KindMap
	KindWaif
)

// --- Constructors (scalars) ---

// NewInt creates an integer value.
func NewInt(v int64) Value { return Value{kind: KindInt, num: uint64(v)} }

// NewFloat creates a floating-point value.
func NewFloat(v float64) Value { return Value{kind: KindFloat, num: math.Float64bits(v)} }

// NewStr creates a string value.
func NewStr(s string) Value { return Value{kind: KindStr, str: s} }

// NewObj creates a regular object reference.
func NewObj(id ObjID) Value { return Value{kind: KindObj, num: uint64(int64(id))} }

// NewAnon creates an anonymous object reference (typeof => TYPE_ANON).
func NewAnon(id ObjID) Value { return Value{kind: KindAnon, num: uint64(int64(id))} }

// NewErr creates an error value.
func NewErr(c ErrorCode) Value { return Value{kind: KindErr, num: uint64(int64(c))} }

// NewBool creates a boolean value.
func NewBool(b bool) Value {
	var n uint64
	if b {
		n = 1
	}
	return Value{kind: KindBool, num: n}
}

// None returns the none/unbound value (also the struct zero value).
func None() Value { return Value{} }

// Unbound is the marker for declared-but-unassigned locals; it is the same
// representation as None (Toast's TYPE_NONE). Retained as a named constructor
// for call sites that previously used UnboundValue{}.
func Unbound() Value { return Value{} }

// --- Kind / type queries ---

// Kind returns the internal type tag.
func (v Value) Kind() Kind { return v.kind }

// IsNone reports whether the value is none/unbound (the zero value).
func (v Value) IsNone() bool { return v.kind == KindNone }

// IsUnbound is an alias for IsNone, for the unbound-local read path.
func (v Value) IsUnbound() bool { return v.kind == KindNone }

func (v Value) IsInt() bool   { return v.kind == KindInt }
func (v Value) IsFloat() bool { return v.kind == KindFloat }
func (v Value) IsStr() bool   { return v.kind == KindStr }
func (v Value) IsObj() bool   { return v.kind == KindObj || v.kind == KindAnon }
func (v Value) IsErr() bool   { return v.kind == KindErr }
func (v Value) IsBool() bool  { return v.kind == KindBool }
func (v Value) IsList() bool  { return v.kind == KindList }
func (v Value) IsMap() bool   { return v.kind == KindMap }
func (v Value) IsWaif() bool  { return v.kind == KindWaif }

// IsAnonymous reports whether this is an anonymous object reference.
func (v Value) IsAnonymous() bool { return v.kind == KindAnon }

// Type returns the MOO-visible type code (typeof()).
func (v Value) Type() TypeCode {
	switch v.kind {
	case KindInt:
		return TYPE_INT
	case KindFloat:
		return TYPE_FLOAT
	case KindStr:
		return TYPE_STR
	case KindObj:
		return TYPE_OBJ
	case KindAnon:
		return TYPE_ANON
	case KindErr:
		return TYPE_ERR
	case KindBool:
		return TYPE_BOOL
	case KindList:
		return TYPE_LIST
	case KindMap:
		return TYPE_MAP
	case KindWaif:
		return TYPE_WAIF
	default:
		// none/unbound is not externally observable; report INT to match the
		// former UnboundValue.Type() behavior.
		return TYPE_INT
	}
}

// --- Unchecked accessors (caller must know the kind) ---

// Int returns the integer payload.
func (v Value) Int() int64 { return int64(v.num) }

// Float returns the float payload.
func (v Value) Float() float64 { return math.Float64frombits(v.num) }

// Str returns the string payload.
func (v Value) Str() string { return v.str }

// ObjNum returns the object id payload (for KindObj or KindAnon).
func (v Value) ObjNum() ObjID { return ObjID(int64(v.num)) }

// ErrCode returns the error code payload.
func (v Value) ErrCode() ErrorCode { return ErrorCode(int64(v.num)) }

// Bool returns the boolean payload.
func (v Value) Bool() bool { return v.num != 0 }

// List returns the list view of the value (caller must know IsList()).
func (v Value) List() ListValue { return ListValue{data: v.ref.(MooList)} }

// Map returns the map view of the value (caller must know IsMap()).
func (v Value) Map() MapValue { return MapValue{data: v.ref.(MooMap)} }

// Waif returns the waif view of the value (caller must know IsWaif()).
func (v Value) Waif() WaifValue { return v.ref.(WaifValue) }

// --- Checked accessors: (payload, ok) ---

func (v Value) AsInt() (int64, bool)     { return int64(v.num), v.kind == KindInt }
func (v Value) AsFloat() (float64, bool) { return math.Float64frombits(v.num), v.kind == KindFloat }
func (v Value) AsStr() (string, bool)    { return v.str, v.kind == KindStr }
func (v Value) AsErr() (ErrorCode, bool) { return ErrorCode(int64(v.num)), v.kind == KindErr }
func (v Value) AsBool() (bool, bool)     { return v.num != 0, v.kind == KindBool }
func (v Value) AsObjID() (ObjID, bool) {
	return ObjID(int64(v.num)), v.kind == KindObj || v.kind == KindAnon
}
func (v Value) AsList() (ListValue, bool) {
	if v.kind != KindList {
		return ListValue{}, false
	}
	return ListValue{data: v.ref.(MooList)}, true
}
func (v Value) AsMap() (MapValue, bool) {
	if v.kind != KindMap {
		return MapValue{}, false
	}
	return MapValue{data: v.ref.(MooMap)}, true
}
func (v Value) AsWaif() (WaifValue, bool) {
	if v.kind != KindWaif {
		return WaifValue{}, false
	}
	return v.ref.(WaifValue), true
}

// --- Float helpers (parity with old FloatValue) ---

func (v Value) IsNaN() bool { return v.kind == KindFloat && math.IsNaN(v.Float()) }
func (v Value) IsInf() bool { return v.kind == KindFloat && math.IsInf(v.Float(), 0) }

// --- Internal heap wrappers (used by list/map/waif constructors) ---

func newListVal(data MooList) Value { return Value{kind: KindList, ref: data} }
func newMapVal(data MooMap) Value   { return Value{kind: KindMap, ref: data} }
func newWaifVal(w WaifValue) Value  { return Value{kind: KindWaif, ref: w} }

// --- Behavior: Truthy / Equal (String lives in value_string.go) ---

// Truthy implements MOO truthiness rules.
func (v Value) Truthy() bool {
	switch v.kind {
	case KindInt:
		return int64(v.num) != 0
	case KindFloat:
		return v.Float() != 0
	case KindStr:
		return len(v.str) != 0
	case KindBool:
		return v.num != 0
	case KindList:
		return v.List().Len() > 0
	case KindMap:
		return v.Map().Len() > 0
	default:
		// none, obj, anon, err, waif are never truthy
		return false
	}
}

// Equal reports MOO deep equality.
func (v Value) Equal(o Value) bool {
	if v.kind != o.kind {
		return false
	}
	switch v.kind {
	case KindNone:
		return true
	case KindInt, KindBool:
		return v.num == o.num
	case KindFloat:
		// NaN != NaN (IEEE 754 semantics)
		if v.IsNaN() || o.IsNaN() {
			return false
		}
		return v.Float() == o.Float()
	case KindStr:
		// MOO string equality is case-insensitive.
		return strings.EqualFold(v.str, o.str)
	case KindObj, KindAnon, KindErr:
		return v.num == o.num
	case KindList:
		return v.List().Equal(o.List())
	case KindMap:
		return v.Map().Equal(o.Map())
	case KindWaif:
		return v.Waif().Equal(o.Waif())
	default:
		return false
	}
}
