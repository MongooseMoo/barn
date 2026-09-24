package types

import (
	"sync/atomic"
	"unicode"
	"unicode/utf8"
)

// MOO string positions and lengths count characters. A character is one
// UTF-8 decoding step: a valid code point, or a single byte that is not part
// of a valid encoding (legacy Latin-1 text, decode_binary output). Slicing
// always cuts the original bytes, so invalid bytes round-trip unchanged.
//
// When a string's character count equals its byte length, every character is
// one byte and positions are byte offsets; callers use that as a fast path.

// CharCount returns the number of characters in s.
func CharCount(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] < utf8.RuneSelf {
			i++
		} else {
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
		n++
	}
	return n
}

// CharOffsets returns the byte offset of each character in s followed by
// len(s), so character i spans offsets[i]:offsets[i+1].
func CharOffsets(s string) []int {
	offsets := make([]int, 0, len(s)+1)
	for i := 0; i < len(s); {
		offsets = append(offsets, i)
		if s[i] < utf8.RuneSelf {
			i++
		} else {
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
	}
	return append(offsets, len(s))
}

// CharByteOffset returns the byte offset where character index i (0-based)
// starts, or len(s) when i is the character count.
func CharByteOffset(s string, i int) int {
	pos := 0
	for ; i > 0 && pos < len(s); i-- {
		if s[pos] < utf8.RuneSelf {
			pos++
		} else {
			_, size := utf8.DecodeRuneInString(s[pos:])
			pos += size
		}
	}
	return pos
}

// CharSlice returns characters [from, to) of s (0-based, half-open).
func CharSlice(s string, from, to int) string {
	start := CharByteOffset(s, from)
	end := start + CharByteOffset(s[start:], to-from)
	return s[start:end]
}

// SplitChars returns each character of s as its own string of raw bytes.
func SplitChars(s string) []string {
	chars := make([]string, 0, len(s))
	for i := 0; i < len(s); {
		size := 1
		if s[i] >= utf8.RuneSelf {
			_, size = utf8.DecodeRuneInString(s[i:])
		}
		chars = append(chars, s[i:i+size])
		i += size
	}
	return chars
}

// StrCharLen returns the character count of a TYPE_STR value. The count is
// cached on the string's immutable payload.
func (v Value) StrCharLen() int {
	r := v.strRep()
	if r == emptyStrRep {
		return 0
	}
	if n := atomic.LoadInt64(&r.chars); n != 0 {
		return int(n)
	}
	var n int
	if r.data != nil {
		n = charCountBytes(r.data)
	} else {
		n = CharCount(r.val)
	}
	atomic.StoreInt64(&r.chars, int64(n))
	return n
}

// StrIsSingleByte reports whether every character of a TYPE_STR value is one
// byte, so character positions are byte offsets.
func (v Value) StrIsSingleByte() bool {
	return v.StrCharLen() == v.strRep().byteLen()
}

func charCountBytes(b []byte) int {
	n := 0
	for i := 0; i < len(b); {
		if b[i] < utf8.RuneSelf {
			i++
		} else {
			_, size := utf8.DecodeRune(b[i:])
			i += size
		}
		n++
	}
	return n
}

// CharView indexes the characters of a string without copying them. For a
// single-byte string it indexes bytes directly and allocates nothing.
type CharView struct {
	s   string
	off []int // nil when every character is one byte
}

// NewCharView returns a view of a TYPE_STR value's characters.
func NewCharView(v Value) CharView {
	s := v.Str()
	if v.StrIsSingleByte() {
		return CharView{s: s}
	}
	return CharView{s: s, off: CharOffsets(s)}
}

// CharViewOf returns a view of a Go string's characters.
func CharViewOf(s string) CharView {
	if CharCount(s) == len(s) {
		return CharView{s: s}
	}
	return CharView{s: s, off: CharOffsets(s)}
}

// Len returns the number of characters in the view.
func (c CharView) Len() int {
	if c.off == nil {
		return len(c.s)
	}
	return len(c.off) - 1
}

// At returns character i (0-based) as its raw bytes.
func (c CharView) At(i int) string {
	if c.off == nil {
		return c.s[i : i+1]
	}
	return c.s[c.off[i]:c.off[i+1]]
}

// Slice returns characters [from, to) as raw bytes.
func (c CharView) Slice(from, to int) string {
	if c.off == nil {
		return c.s[from:to]
	}
	return c.s[c.off[from]:c.off[to]]
}

// ByteOffset returns the byte offset of character i; i may equal Len().
func (c CharView) ByteOffset(i int) int {
	if c.off == nil {
		return i
	}
	return c.off[i]
}

// CharIndexOfByte maps a byte offset that starts a character (or equals the
// string length) to its character index.
func (c CharView) CharIndexOfByte(b int) int {
	if c.off == nil {
		return b
	}
	lo, hi := 0, len(c.off)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if c.off[mid] < b {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

// CharEqual compares two characters. With fold set, valid code points compare
// case-insensitively; an invalid byte only ever equals the identical byte.
func CharEqual(a, b string, fold bool) bool {
	if a == b {
		return true
	}
	if !fold {
		return false
	}
	ra, sa := utf8.DecodeRuneInString(a)
	rb, sb := utf8.DecodeRuneInString(b)
	if (ra == utf8.RuneError && sa <= 1) || (rb == utf8.RuneError && sb <= 1) {
		return false
	}
	return unicode.ToLower(ra) == unicode.ToLower(rb)
}
