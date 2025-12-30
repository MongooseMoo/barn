package types

import (
	"fmt"
	"strings"
)

// StrValue represents a MOO string
type StrValue struct {
	val string
}

// NewStr creates a new string value
func NewStr(s string) StrValue {
	return StrValue{val: s}
}

// String returns the MOO string representation with binary encoding
// Non-printable characters (< 32 or > 126) are encoded as ~XX
func (s StrValue) String() string {
	var result strings.Builder
	result.WriteByte('"')
	for i := 0; i < len(s.val); i++ {
		b := s.val[i]
		if b == '"' {
			result.WriteString("\\\"")
		} else if b == '\\' {
			result.WriteString("\\\\")
		} else if b >= 32 && b <= 126 {
			// Printable ASCII
			result.WriteByte(b)
		} else {
			// Non-printable: use ~XX encoding
			result.WriteString(fmt.Sprintf("~%02X", b))
		}
	}
	result.WriteByte('"')
	return result.String()
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
// MOO strings are case-insensitive
func (s StrValue) Equal(other Value) bool {
	if o, ok := other.(StrValue); ok {
		return strings.EqualFold(s.val, o.val)
	}
	return false
}

// Value returns the internal string value
func (s StrValue) Value() string {
	return s.val
}
