package types

import "fmt"

// ObjValue represents a MOO object reference
type ObjValue struct {
	id        ObjID
	anonymous bool // true for anonymous objects (type code 12)
}

// Special object constants
const (
	NOTHING      = ObjID(-1)
	AMBIGUOUS    = ObjID(-2)
	FAILED_MATCH = ObjID(-3)
)

// NewObj creates a new object value
func NewObj(id ObjID) ObjValue {
	return ObjValue{id: id, anonymous: false}
}

// NewAnon creates a new anonymous object value
func NewAnon(id ObjID) ObjValue {
	return ObjValue{id: id, anonymous: true}
}

// String returns the MOO string representation
// Anonymous objects use *#N format, regular objects use #N
func (o ObjValue) String() string {
	if o.anonymous {
		return fmt.Sprintf("*#%d", o.id)
	}
	return fmt.Sprintf("#%d", o.id)
}

// Type returns the MOO type
// Anonymous objects return TYPE_ANON (12), regular objects return TYPE_OBJ (1)
func (o ObjValue) Type() TypeCode {
	if o.anonymous {
		return TYPE_ANON
	}
	return TYPE_OBJ
}

// IsAnonymous returns whether this is an anonymous object
func (o ObjValue) IsAnonymous() bool {
	return o.anonymous
}

// Truthy returns whether the value is truthy
// In MOO, objects are never truthy (only non-zero ints and non-empty strings are truthy)
func (o ObjValue) Truthy() bool {
	return false
}

// Equal compares two values for equality.
//
// This mirrors ToastStunt's equality() (src/utils.cc:444): it switches on the
// value type FIRST, so values of different types never compare equal (the
// cross-type fall-through returns 0, src/utils.cc:484). A regular object is
// TYPE_OBJ and an anonymous object is TYPE_ANON, so they are never equal even
// with the same numeric id. Within a type:
//   - TYPE_OBJ compares by id (utils.cc:455: lhs.v.obj == rhs.v.obj).
//   - TYPE_ANON compares by reference identity (utils.cc:476: lhs.v.anon ==
//     rhs.v.anon). Barn's ObjValue does not carry an anon pointer/handle — it
//     only has the numeric id — so we approximate anon identity with id
//     equality. This is correct for distinguishing anon from regular and for
//     comparing the same anon handle to itself; it cannot detect two distinct
//     anon instances that happen to share an id (a known limitation of the
//     ObjValue representation).
func (o ObjValue) Equal(other Value) bool {
	if otherObj, ok := other.(ObjValue); ok {
		return o.anonymous == otherObj.anonymous && o.id == otherObj.id
	}
	return false
}

// ID returns the object ID
func (o ObjValue) ID() ObjID {
	return o.id
}
