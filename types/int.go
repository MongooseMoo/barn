package types

import "strconv"

// IntValue represents a MOO integer
type IntValue struct {
	Val int64
}

// Type returns the type code for integers
func (i IntValue) Type() TypeCode {
	return TYPE_INT
}

// String returns the MOO literal representation.
// strconv.FormatInt avoids fmt.Sprintf's reflection/alloc overhead on the
// hot stringify path (tostr/toliteral over integers).
func (i IntValue) String() string {
	return strconv.FormatInt(i.Val, 10)
}

// Equal checks deep equality
func (i IntValue) Equal(other Value) bool {
	if other == nil {
		return false
	}
	if other.Type() != TYPE_INT {
		return false
	}
	otherInt, ok := other.(IntValue)
	if !ok {
		return false
	}
	return i.Val == otherInt.Val
}

// Truthy returns the MOO truthiness
// 0 is falsy, all other integers are truthy
func (i IntValue) Truthy() bool {
	return i.Val != 0
}

// NewInt creates a new IntValue
func NewInt(val int64) IntValue {
	return IntValue{Val: val}
}
