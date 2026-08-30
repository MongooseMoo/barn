package types

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unsafe"
)

// Value is a value-typed tagged union representing every MOO value.
//
// It REPLACES the former Value interface and the boxed concrete types
// (IntValue, FloatValue, ObjValue, ErrValue, BoolValue, UnboundValue, StrValue,
// ListValue, MapValue, WaifValue). There is one and only one Value type.
//
// Encoding:
//   - tag: the discriminator. For externally-visible values it is one of the
//     TYPE_* constants (these are DB-persisted by db/format and MUST NOT be
//     renumbered). Two INTERNAL tags live OUTSIDE that range and are never
//     persisted or externally observable: tagNone (cleared/absent) and
//     tagUnbound (a declared-but-unbound local).
//   - n: the inline scalar payload. Holds an int64 (bit-reinterpreted via
//     uint64), a float64 (math.Float64bits), an object id, an error code, or a
//     bool (0/1). Scalars carry no pointer, so they never allocate and the GC
//     never scans them — this is the whole point of the de-box.
//   - ref: the heap payload for the reference types str/list/map/waif
//     (*strRep, *sliceList, *goMap, *waifRep); nil for scalars. It is a real
//     pointer-typed field so the garbage collector keeps the payload alive and
//     may relocate it. NEVER store a uintptr here — the GC would not trace it.
//
// The struct is three machine words (24 bytes on 64-bit). Constructing a scalar
// Value is zero-alloc.
//
// The None sentinel: the struct has no nil. Its zero value Value{} has tag
// TYPE_INT (== 0), which is a valid integer 0, NOT "none". Absence (the old
// interface-nil: cleared properties, out-of-bounds Get, an empty Result.Val)
// is represented EXPLICITLY by None and tested with IsNone. Do not treat the
// zero value as none.
type Value struct {
	ref unsafe.Pointer
	n   uint64
	tag TypeCode
}

// Internal tags, deliberately negative so they never collide with — and are
// never confused for — a persisted TypeCode (all of which are >= 0).
const (
	tagNone    TypeCode = -1 // replaces interface-nil: CLEAR / OOB Get / empty Result.Val
	tagUnbound TypeCode = -2 // replaces the old UnboundValue marker
)

// None is the explicit "no value" sentinel. It stands in for every place the
// old representation used a nil Value.
var None = Value{tag: tagNone}

// Unbound is the internal marker for a declared-but-unbound local variable. VM
// variable reads convert it into E_VARNF. Its type is not externally observable
// (Type() reports TYPE_INT), matching the old UnboundValue.
var Unbound = Value{tag: tagUnbound}

// IsNone reports whether v is the None sentinel (absent / cleared value).
func (v Value) IsNone() bool { return v.tag == tagNone }

// IsUnbound reports whether v is the unbound-local marker.
func (v Value) IsUnbound() bool { return v.tag == tagUnbound }

// Type returns the MOO type code. The internal sentinels (none/unbound) are not
// externally observable and report TYPE_INT, preserving the old UnboundValue
// behavior; callers distinguish them with IsNone/IsUnbound.
func (v Value) Type() TypeCode {
	if v.tag < 0 {
		return TYPE_INT
	}
	return v.tag
}

// ---- scalar constructors (zero-alloc) ----------------------------------

// NewInt creates an integer value.
func NewInt(i int64) Value { return Value{tag: TYPE_INT, n: uint64(i)} }

// NewFloat creates a floating-point value, storing the IEEE-754 bits verbatim.
func NewFloat(f float64) Value { return Value{tag: TYPE_FLOAT, n: math.Float64bits(f)} }

// NewObj creates a (regular) object reference.
func NewObj(id ObjID) Value { return Value{tag: TYPE_OBJ, n: uint64(id)} }

// NewAnon creates an anonymous object reference (type code TYPE_ANON).
func NewAnon(id ObjID) Value { return Value{tag: TYPE_ANON, n: uint64(id)} }

// NewErr creates an error value.
func NewErr(code ErrorCode) Value { return Value{tag: TYPE_ERR, n: uint64(code)} }

// NewBool creates a boolean value.
func NewBool(b bool) Value {
	var n uint64
	if b {
		n = 1
	}
	return Value{tag: TYPE_BOOL, n: n}
}

// ---- scalar accessors --------------------------------------------------

// Int returns the integer payload (only meaningful when Type()==TYPE_INT).
func (v Value) Int() int64 { return int64(v.n) }

// Float returns the float payload (only meaningful when Type()==TYPE_FLOAT).
func (v Value) Float() float64 { return math.Float64frombits(v.n) }

// Obj returns the object id (meaningful for TYPE_OBJ and TYPE_ANON).
func (v Value) Obj() ObjID { return ObjID(v.n) }

// ID is an alias of Obj kept for migration ergonomics (old ObjValue.ID()).
func (v Value) ID() ObjID { return ObjID(v.n) }

// IsAnonymous reports whether this object reference is anonymous.
func (v Value) IsAnonymous() bool { return v.tag == TYPE_ANON }

// ErrCode returns the error code (only meaningful when Type()==TYPE_ERR).
func (v Value) ErrCode() ErrorCode { return ErrorCode(v.n) }

// Code is an alias of ErrCode kept for migration ergonomics (old ErrValue.Code()).
func (v Value) Code() ErrorCode { return ErrorCode(v.n) }

// Bool returns the boolean payload (only meaningful when Type()==TYPE_BOOL).
func (v Value) Bool() bool { return v.n != 0 }

// IsNaN reports whether a float value is NaN.
func (v Value) IsNaN() bool { return math.IsNaN(math.Float64frombits(v.n)) }

// IsInf reports whether a float value is +/-Inf.
func (v Value) IsInf() bool { return math.IsInf(math.Float64frombits(v.n), 0) }

// ---- truthiness --------------------------------------------------------

// Truthy implements MOO truthiness: non-zero numbers, non-empty strings, and
// non-empty lists/maps are truthy; objects, errors, waifs, none and unbound are
// not.
func (v Value) Truthy() bool {
	switch v.tag {
	case TYPE_INT:
		return int64(v.n) != 0
	case TYPE_FLOAT:
		return math.Float64frombits(v.n) != 0
	case TYPE_BOOL:
		return v.n != 0
	case TYPE_STR:
		return v.strRep().byteLen() > 0
	case TYPE_LIST:
		return v.sliceList().Len() > 0
	case TYPE_MAP:
		return v.goMap().Len() > 0
	default:
		return false
	}
}

// ---- string representation ---------------------------------------------

// String returns the MOO literal representation of the value.
func (v Value) String() string {
	switch v.tag {
	case TYPE_INT:
		return strconv.FormatInt(int64(v.n), 10)
	case TYPE_FLOAT:
		return formatFloat(math.Float64frombits(v.n))
	case TYPE_OBJ:
		return fmt.Sprintf("#%d", int64(ObjID(v.n)))
	case TYPE_ANON:
		return fmt.Sprintf("*#%d", int64(ObjID(v.n)))
	case TYPE_ERR:
		return ErrorCode(v.n).String()
	case TYPE_BOOL:
		if v.n != 0 {
			return "true"
		}
		return "false"
	case TYPE_STR:
		return v.strRep().literal()
	case TYPE_LIST:
		return v.sliceList().literal()
	case TYPE_MAP:
		return v.goMap().literal()
	case TYPE_WAIF:
		return v.waifRep().literal()
	case tagUnbound:
		return "<unbound>"
	case tagNone:
		return "None"
	default:
		return "UNKNOWN"
	}
}

// formatFloat renders a float in MOO/ToastStunt style: 15 significant digits
// (printf "%.15g"), with a trailing .0 for integral values and special spellings
// for NaN/Inf.
func formatFloat(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.IsInf(f, 1) {
		return "Inf"
	}
	if math.IsInf(f, -1) {
		return "-Inf"
	}
	s := strconv.FormatFloat(f, 'g', 15, 64)
	if !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") {
		s += ".0"
	}
	return s
}

// ---- equality ----------------------------------------------------------

// Equal implements MOO deep equality. Distinct types are never equal (int 1,
// float 1.0 and str "1" are all different); strings compare case-insensitively;
// NaN is never equal to anything (IEEE 754).
func (v Value) Equal(other Value) bool {
	switch v.tag {
	case TYPE_INT:
		return other.tag == TYPE_INT && v.n == other.n
	case TYPE_FLOAT:
		if other.tag != TYPE_FLOAT {
			return false
		}
		a := math.Float64frombits(v.n)
		b := math.Float64frombits(other.n)
		if math.IsNaN(a) || math.IsNaN(b) {
			return false
		}
		return a == b
	case TYPE_OBJ, TYPE_ANON:
		// F13: dispatch on type FIRST, matching ToastStunt equality()
		// (src/utils.cc:444, cross-type never equal at :484). A regular object and
		// an anonymous object with the same id are NOT equal — the tag carries the
		// anonymous kind. (ffe6704 "Fix F13: ObjValue.Equal must respect anonymous
		// flag"; regression guarded by TestReview_ObjEqualIgnoresAnonFlag.)
		return v.tag == other.tag && v.n == other.n
	case TYPE_ERR:
		return other.tag == TYPE_ERR && v.n == other.n
	case TYPE_BOOL:
		return other.tag == TYPE_BOOL && v.n == other.n
	case TYPE_STR:
		return other.tag == TYPE_STR && compareFoldedASCII(normalizeBinaryString(v.strRep().str()), normalizeBinaryString(other.strRep().str())) == 0
	case TYPE_LIST:
		return other.tag == TYPE_LIST && v.sliceList().equal(other.sliceList())
	case TYPE_MAP:
		return other.tag == TYPE_MAP && v.goMap().equal(other.goMap())
	case TYPE_WAIF:
		return other.tag == TYPE_WAIF && v.waifRep().equal(other.waifRep())
	case tagNone:
		return other.tag == tagNone
	case tagUnbound:
		return other.tag == tagUnbound
	default:
		return false
	}
}

func normalizeBinaryString(s string) string {
	var result strings.Builder
	for i := 0; i < len(s); i++ {
		if i+2 < len(s) && s[i] == '~' {
			hi, hiOK := asciiHex(s[i+1])
			lo, loOK := asciiHex(s[i+2])
			if hiOK && loOK && hi<<4|lo < 0x20 {
				result.WriteByte(hi<<4 | lo)
				i += 2
				continue
			}
		}
		result.WriteByte(s[i])
	}
	return result.String()
}

func asciiHex(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	default:
		return 0, false
	}
}

// ---- unified length ----------------------------------------------------

// Len returns the length of a string (bytes), list (elements), or map (entries).
// For any other type it returns 0.
func (v Value) Len() int {
	switch v.tag {
	case TYPE_STR:
		return v.strRep().byteLen()
	case TYPE_LIST:
		return v.sliceList().Len()
	case TYPE_MAP:
		return v.goMap().Len()
	default:
		return 0
	}
}

// ---- heap payload helpers ----------------------------------------------
//
// These cast the ref word back to its concrete payload pointer. They are the
// only place unsafe.Pointer is dereferenced; every site is guarded by the tag.

func (v Value) strRep() *strRep       { return (*strRep)(v.ref) }
func (v Value) sliceList() *sliceList { return (*sliceList)(v.ref) }
func (v Value) goMap() *goMap         { return (*goMap)(v.ref) }
func (v Value) waifRep() *waifRep     { return (*waifRep)(v.ref) }
