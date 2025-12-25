package types

import "fmt"

// ObjValue represents a MOO object reference
type ObjValue struct {
	id ObjID
}

// Special object constants
const (
	NOTHING      = ObjID(-1)
	AMBIGUOUS    = ObjID(-2)
	FAILED_MATCH = ObjID(-3)
)

// NewObj creates a new object value
func NewObj(id ObjID) ObjValue {
	return ObjValue{id: id}
}

// String returns the MOO string representation
func (o ObjValue) String() string {
	return fmt.Sprintf("#%d", o.id)
}

// Type returns the MOO type
func (o ObjValue) Type() TypeCode {
	return TYPE_OBJ
}

// Truthy returns whether the value is truthy
// In MOO, objects are never truthy (only non-zero ints and non-empty strings are truthy)
func (o ObjValue) Truthy() bool {
	return false
}

// Equal compares two values for equality
func (o ObjValue) Equal(other Value) bool {
	if otherObj, ok := other.(ObjValue); ok {
		return o.id == otherObj.id
	}
	return false
}

// ID returns the object ID
func (o ObjValue) ID() ObjID {
	return o.id
}
