package types

import "fmt"

// StrValue represents a MOO string
type StrValue struct {
	val string
}

// NewStr creates a new string value
func NewStr(s string) StrValue {
	return StrValue{val: s}
}

// String returns the Go string representation
func (s StrValue) String() string {
	return fmt.Sprintf("%q", s.val)
}

// Type returns the MOO type
func (s StrValue) Type() TypeCode {
	return TYPE_STR
}

// Truthy returns whether the value is truthy
// Empty strings are falsy, non-empty strings are truthy
func (s StrValue) Truthy() bool {
	return len(s.val) > 0
}

// Equal compares two values for equality
func (s StrValue) Equal(other Value) bool {
	if o, ok := other.(StrValue); ok {
		return s.val == o.val
	}
	return false
}

// Value returns the internal string value
func (s StrValue) Value() string {
	return s.val
}
