package types

// BoolValue represents a MOO boolean
type BoolValue struct {
	Val bool
}

// Type returns the type code for booleans
func (b BoolValue) Type() TypeCode {
	return TYPE_BOOL
}

// String returns the MOO literal representation
func (b BoolValue) String() string {
	if b.Val {
		return "true"
	}
	return "false"
}

// Equal checks deep equality
func (b BoolValue) Equal(other Value) bool {
	if other == nil {
		return false
	}
	if other.Type() != TYPE_BOOL {
		return false
	}
	otherBool, ok := other.(BoolValue)
	if !ok {
		return false
	}
	return b.Val == otherBool.Val
}

// Truthy returns the MOO truthiness
func (b BoolValue) Truthy() bool {
	return b.Val
}

// NewBool creates a new BoolValue
func NewBool(val bool) BoolValue {
	return BoolValue{Val: val}
}
