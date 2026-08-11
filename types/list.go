package types

import (
	"strings"
	"sync/atomic"
	"unsafe"
)

// sliceList is the heap payload behind a TYPE_LIST Value.
//
// byteSize caches ValueBytes(list); a negative value means "not yet computed".
//
// watermark enables amortized-O(1) append without copying, safely under value
// sharing (MOO lists are immutable values, but several sliceList headers can
// share one backing array). It points to the exclusive append frontier in the
// backing array, shared by every header viewing that array. A header may append
// in place only when its own length equals the frontier (i.e. nobody has
// appended past it), the backing has spare capacity, and it atomically claims
// the next slot. It then writes the previously-uncommitted slot and returns a
// NEW header — never mutating its own (possibly aliased) header. A nil watermark
// always copies, so any list not produced by Append's growth path is append-safe.
type sliceList struct {
	elements  []Value
	byteSize  int
	watermark *atomic.Int64
}

// newSliceList wraps elements with an uncomputed size cache (filled lazily).
func newSliceList(elements []Value) *sliceList {
	return &sliceList{elements: elements, byteSize: -1}
}

// newSliceListSized wraps elements with a known, pre-computed size cache. Used
// on the append/concat hot path so size accounting stays O(1) per operation.
func newSliceListSized(elements []Value, byteSize int) *sliceList {
	return &sliceList{elements: elements, byteSize: byteSize}
}

// listValue boxes a sliceList into a Value.
func listValue(sl *sliceList) Value {
	return Value{tag: TYPE_LIST, ref: unsafe.Pointer(sl)}
}

func (s *sliceList) Len() int {
	return len(s.elements)
}

// ByteSize returns the cached ValueBytes of the list, computing it once on first
// use. Lists are immutable, so the cached value never goes stale.
func (s *sliceList) byteSizeOf() int {
	if s.byteSize < 0 {
		size := listVarOverhead
		for _, e := range s.elements {
			size += ValueBytes(e)
		}
		s.byteSize = size
	}
	return s.byteSize
}

// get returns the 1-based element, or None when out of bounds (the old
// representation returned a nil Value here).
func (s *sliceList) get(i int) Value {
	if i < 1 || i > len(s.elements) {
		return None
	}
	return s.elements[i-1]
}

func (s *sliceList) set(i int, v Value) *sliceList {
	if i < 1 || i > len(s.elements) {
		return s // Out of bounds - return unchanged
	}
	newElems := make([]Value, len(s.elements))
	copy(newElems, s.elements)
	newElems[i-1] = v
	if s.byteSize >= 0 {
		return newSliceListSized(newElems, s.byteSize-ValueBytes(s.elements[i-1])+ValueBytes(v))
	}
	return newSliceList(newElems)
}

func (s *sliceList) append(v Value) *sliceList {
	n := len(s.elements)
	bs := s.byteSizeOf() + ValueBytes(v)

	// In-place fast path: this header owns the frontier of the backing array
	// (its length equals the frontier) and there is spare capacity.
	if s.watermark != nil && cap(s.elements) > n &&
		s.watermark.CompareAndSwap(int64(n), int64(n+1)) {
		extended := s.elements[:n+1]
		extended[n] = v
		return &sliceList{elements: extended, byteSize: bs, watermark: s.watermark}
	}

	// Copy path: reallocate with amortized growth (the [:n:n] cap forces a copy
	// rather than touching s's backing). A fresh watermark tracks the new backing.
	newElems := append(s.elements[:n:n], v)
	wm := new(atomic.Int64)
	wm.Store(int64(n + 1))
	return &sliceList{elements: newElems, byteSize: bs, watermark: wm}
}

func (s *sliceList) slice(start, end int) *sliceList {
	if start < 1 {
		start = 1
	}
	if end > len(s.elements) {
		end = len(s.elements)
	}
	if start > end {
		return newSliceList([]Value{})
	}
	newElems := make([]Value, end-start+1)
	copy(newElems, s.elements[start-1:end])
	return newSliceList(newElems)
}

func (s *sliceList) equal(other *sliceList) bool {
	if len(s.elements) != len(other.elements) {
		return false
	}
	for i := range s.elements {
		if !s.elements[i].Equal(other.elements[i]) {
			return false
		}
	}
	return true
}

// literal returns the MOO literal representation of the list.
func (s *sliceList) literal() string {
	if len(s.elements) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(s.elements))
	for _, elem := range s.elements {
		if elem.IsNone() {
			parts = append(parts, "0") // none renders as 0 in MOO
		} else {
			parts = append(parts, elem.String())
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// ---- constructors ------------------------------------------------------

// NewList creates a list value.
func NewList(elements []Value) Value {
	return listValue(newSliceList(elements))
}

// NewEmptyList creates an empty list value.
func NewEmptyList() Value {
	return listValue(newSliceListSized([]Value{}, listVarOverhead))
}

// ---- Value-level list API (list-typed methods keep their natural names) --

// Get returns the 1-based list element, or None if out of bounds.
func (v Value) Get(index int) Value { return v.sliceList().get(index) }

// Set returns a new list with the 1-based element set (COW).
func (v Value) Set(index int, value Value) Value {
	return listValue(v.sliceList().set(index, value))
}

// Append returns a new list with value appended (COW).
func (v Value) Append(value Value) Value {
	return listValue(v.sliceList().append(value))
}

// Elements returns the backing element slice (for iteration; do not mutate).
func (v Value) Elements() []Value { return v.sliceList().elements }

// ByteSize returns the cached ValueBytes of the list (O(1) after first use).
func (v Value) ByteSize() int { return v.sliceList().byteSizeOf() }

// Concat returns a new list with all elements of other appended (COW).
func (v Value) Concat(other Value) Value {
	a := v.sliceList().elements
	b := other.sliceList().elements
	newElems := make([]Value, len(a)+len(b))
	copy(newElems, a)
	copy(newElems[len(a):], b)
	return listValue(newSliceListSized(newElems, v.ByteSize()+other.ByteSize()-listVarOverhead))
}

// InsertAt returns a new list with value inserted at the 1-based index (COW).
func (v Value) InsertAt(index int, value Value) Value {
	elements := v.sliceList().elements
	if index < 1 {
		index = 1
	}
	if index > len(elements)+1 {
		index = len(elements) + 1
	}
	newElems := make([]Value, len(elements)+1)
	idx0 := index - 1
	copy(newElems[:idx0], elements[:idx0])
	newElems[idx0] = value
	copy(newElems[idx0+1:], elements[idx0:])
	return listValue(newSliceList(newElems))
}

// DeleteAt returns a new list with the 1-based element removed (COW).
func (v Value) DeleteAt(index int) Value {
	elements := v.sliceList().elements
	if index < 1 || index > len(elements) {
		return v // Out of bounds - unchanged
	}
	newElems := make([]Value, len(elements)-1)
	idx0 := index - 1
	copy(newElems[:idx0], elements[:idx0])
	copy(newElems[idx0:], elements[idx0+1:])
	return listValue(newSliceList(newElems))
}

// Slice returns a new list of elements from start to end (1-based, inclusive).
func (v Value) Slice(start, end int) Value {
	return listValue(v.sliceList().slice(start, end))
}
