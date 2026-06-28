package types

import (
	"fmt"
	"strings"
	"unsafe"
)

// strRep is the heap payload behind a TYPE_STR Value.
//
// It keeps the copy-on-write / append-in-place builder semantics of the old
// StrValue: several headers may share one backing slice, and watermark tracks
// the highest committed append index so growth is amortized-O(1) without
// mutating aliases.
type strRep struct {
	val       string
	data      []byte
	watermark *int
}

// NewStr creates a string value.
func NewStr(s string) Value {
	return Value{tag: TYPE_STR, ref: unsafe.Pointer(&strRep{val: s})}
}

// strValue boxes an existing strRep into a Value.
func strValue(r *strRep) Value {
	return Value{tag: TYPE_STR, ref: unsafe.Pointer(r)}
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

func (s *strRep) byteLen() int {
	if s.data != nil {
		return len(s.data)
	}
	return len(s.val)
}

func (s *strRep) copyTo(dst []byte) {
	if s.data != nil {
		copy(dst, s.data)
		return
	}
	copy(dst, s.val)
}

func (s *strRep) appendTo(dst []byte) []byte {
	if s.data != nil {
		return append(dst, s.data...)
	}
	return append(dst, s.val...)
}

// str returns the materialized Go string.
func (s *strRep) str() string {
	if s.data != nil {
		return string(s.data)
	}
	return s.val
}

// appendRep returns a strRep with other appended, preserving MOO value
// semantics (aliases keep their old content; the result may reuse uncommitted
// capacity when this header owns the append frontier).
func (s *strRep) appendRep(other *strRep) *strRep {
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
		return &strRep{data: extended, watermark: s.watermark}
	}

	buf := make([]byte, n, growStringCap(needed))
	s.copyTo(buf)
	buf = other.appendTo(buf)
	wm := needed
	return &strRep{data: buf, watermark: &wm}
}

// literal returns the MOO literal representation with binary encoding:
// non-printable bytes (< 32 or > 126) are encoded as ~XX.
func (s *strRep) literal() string {
	var result strings.Builder
	result.WriteByte('"')
	n := s.byteLen()
	for i := 0; i < n; i++ {
		var b byte
		if s.data != nil {
			b = s.data[i]
		} else {
			b = s.val[i]
		}
		switch {
		case b == '"':
			result.WriteString("\\\"")
		case b == '\\':
			result.WriteString("\\\\")
		case b == '\t':
			// Preserve tabs as literal tabs for protocol compatibility.
			result.WriteByte(b)
		case b >= 32 && b <= 126:
			result.WriteByte(b)
		default:
			result.WriteString(fmt.Sprintf("~%02X", b))
		}
	}
	result.WriteByte('"')
	return result.String()
}

// Str returns the string content (old StrValue.Value()).
func (v Value) Str() string { return v.strRep().str() }

// StrAppend returns a new string value with other appended. other must be a
// string value.
func (v Value) StrAppend(other Value) Value {
	return strValue(v.strRep().appendRep(other.strRep()))
}
