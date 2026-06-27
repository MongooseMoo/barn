package types

import (
	"fmt"
	"strings"
)

// StrValue represents a MOO string
type StrValue struct {
	val       string
	data      []byte
	watermark *int
}

// NewStr creates a new string value
func NewStr(s string) StrValue {
	return StrValue{val: s}
}

func growStringCap(needed int) int {
	if needed <= 0 {
		return 0
	}
	capacity := 16
	for capacity < needed {
		capacity *= 2
	}
	return capacity
}

func (s StrValue) byteLen() int {
	if s.data != nil {
		return len(s.data)
	}
	return len(s.val)
}

func (s StrValue) copyTo(dst []byte) {
	if s.data != nil {
		copy(dst, s.data)
		return
	}
	copy(dst, s.val)
}

func (s StrValue) appendTo(dst []byte) []byte {
	if s.data != nil {
		return append(dst, s.data...)
	}
	return append(dst, s.val...)
}

// Len returns the byte length of the string without materializing builder-backed
// strings into immutable Go strings.
func (s StrValue) Len() int {
	return s.byteLen()
}

// Append returns a string with other appended. It preserves MOO value semantics:
// previous aliases keep their old length/content, while the returned value may
// reuse uncommitted capacity when this header owns the append frontier.
func (s StrValue) Append(other StrValue) StrValue {
	n := s.byteLen()
	otherLen := other.byteLen()
	if otherLen == 0 {
		return s
	}

	needed := n + otherLen
	if s.watermark != nil && *s.watermark == n && cap(s.data) >= needed {
		extended := s.data[:needed]
		other.copyTo(extended[n:])
		*s.watermark = needed
		return StrValue{data: extended, watermark: s.watermark}
	}

	buf := make([]byte, n, growStringCap(needed))
	s.copyTo(buf)
	buf = other.appendTo(buf)
	wm := needed
	return StrValue{data: buf, watermark: &wm}
}

// String returns the MOO string representation with binary encoding
// Non-printable characters (< 32 or > 126) are encoded as ~XX
func (s StrValue) String() string {
	var result strings.Builder
	result.WriteByte('"')
	for i := 0; i < s.byteLen(); i++ {
		var b byte
		if s.data != nil {
			b = s.data[i]
		} else {
			b = s.val[i]
		}
		if b == '"' {
			result.WriteString("\\\"")
		} else if b == '\\' {
			result.WriteString("\\\\")
		} else if b == '\t' {
			// Preserve tabs as literal tab characters for protocol compatibility.
			result.WriteByte(b)
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
	return s.byteLen() > 0
}

// Equal compares two values for equality
// MOO strings are case-insensitive
func (s StrValue) Equal(other Value) bool {
	if o, ok := other.(StrValue); ok {
		return strings.EqualFold(s.Value(), o.Value())
	}
	return false
}

// Value returns the internal string value
func (s StrValue) Value() string {
	if s.data != nil {
		return string(s.data)
	}
	return s.val
}
