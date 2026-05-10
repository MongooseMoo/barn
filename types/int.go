package types

import "fmt"

// IntValue represents a MOO integer
type IntValue struct {
	Val int64
}

// Type returns the type code for integers
func (i IntValue) Type() TypeCode {
	return TYPE_INT
}

// String returns the MOO literal representation
func (i IntValue) String() string {
	return fmt.Sprintf("%d", i.Val)
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
