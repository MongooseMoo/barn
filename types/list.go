package types

import "strings"

// MooList abstracts list storage - allows swapping implementation later
type MooList interface {
	Len() int
	Get(index int) Value            // 1-based MOO index
	Set(index int, v Value) MooList // Returns new list (COW)
	Append(v Value) MooList
	Slice(start, end int) MooList
	Elements() []Value // For iteration
	ByteSize() int     // ValueBytes of the list (cached)
}

// sliceList is the concrete implementation (private).
// byteSize caches ValueBytes(list); a negative value means "not yet computed".
type sliceList struct {
	elements []Value
	byteSize int
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

func (s *sliceList) Len() int {
	return len(s.elements)
}

// ByteSize returns the cached ValueBytes of the list, computing it once on first
// use. Lists are immutable, so the cached value never goes stale.
func (s *sliceList) ByteSize() int {
	if s.byteSize < 0 {
		size := listVarOverhead
		for _, e := range s.elements {
			size += ValueBytes(e)
		}
		s.byteSize = size
	}
	return s.byteSize
}

func (s *sliceList) Get(i int) Value {
	if i < 1 || i > len(s.elements) {
		return nil
	}
	return s.elements[i-1] // 1-based to 0-based
}

func (s *sliceList) Set(i int, v Value) MooList {
	if i < 1 || i > len(s.elements) {
		return s // Out of bounds - return unchanged
	}
	newElems := make([]Value, len(s.elements))
	copy(newElems, s.elements)
	newElems[i-1] = v
	// Size changes by the delta of the replaced element; compute incrementally
	// when the current size is already known, else leave it lazy.
	if s.byteSize >= 0 {
		return newSliceListSized(newElems, s.byteSize-ValueBytes(s.elements[i-1])+ValueBytes(v))
	}
	return newSliceList(newElems)
}

func (s *sliceList) Append(v Value) MooList {
	newElems := make([]Value, len(s.elements)+1)
	copy(newElems, s.elements)
	newElems[len(s.elements)] = v
	return newSliceListSized(newElems, s.ByteSize()+ValueBytes(v))
}

func (s *sliceList) Slice(start, end int) MooList {
	// 1-based inclusive range
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

func (s *sliceList) Elements() []Value {
	return s.elements
}

// ListValue represents a MOO list
type ListValue struct {
	data MooList
}

// NewList creates a new list value
func NewList(elements []Value) ListValue {
	return ListValue{data: newSliceList(elements)}
}

// NewEmptyList creates an empty list
func NewEmptyList() ListValue {
	return ListValue{data: newSliceListSized([]Value{}, listVarOverhead)}
}

// String returns the MOO string representation
func (l ListValue) String() string {
	elements := l.data.Elements()
	if len(elements) == 0 {
		return "{}"
	}

	var parts []string
	for _, elem := range elements {
		if elem == nil {
			parts = append(parts, "0") // nil becomes 0 in MOO
		} else {
			parts = append(parts, elem.String())
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// Type returns the MOO type
func (l ListValue) Type() TypeCode {
	return TYPE_LIST
}

// Truthy returns whether the value is truthy
// In MOO, non-empty lists are truthy, empty lists are falsy
func (l ListValue) Truthy() bool {
	return l.Len() > 0
}

// Equal compares two values for equality (deep comparison)
func (l ListValue) Equal(other Value) bool {
	if otherList, ok := other.(ListValue); ok {
		if l.data.Len() != otherList.data.Len() {
			return false
		}

		// Deep comparison
		elems1 := l.data.Elements()
		elems2 := otherList.data.Elements()
		for i := 0; i < len(elems1); i++ {
			if !elems1[i].Equal(elems2[i]) {
				return false
			}
		}
		return true
	}
	return false
}

// Len returns the length of the list
func (l ListValue) Len() int {
	return l.data.Len()
}

// Get returns the element at index (1-based)
func (l ListValue) Get(index int) Value {
	return l.data.Get(index)
}

// Set returns a new list with the element at index set to value (1-based, COW)
func (l ListValue) Set(index int, value Value) ListValue {
	return ListValue{data: l.data.Set(index, value)}
}

// Append returns a new list with the value appended (COW)
func (l ListValue) Append(value Value) ListValue {
	return ListValue{data: l.data.Append(value)}
}

// ByteSize returns the cached ValueBytes of the list (O(1) after first use).
func (l ListValue) ByteSize() int {
	return l.data.ByteSize()
}

// Concat returns a new list with all elements of other appended (COW). Size
// accounting is incremental: other's elements contribute its size minus the
// fixed list overhead.
func (l ListValue) Concat(other ListValue) ListValue {
	a := l.data.Elements()
	b := other.data.Elements()
	newElems := make([]Value, len(a)+len(b))
	copy(newElems, a)
	copy(newElems[len(a):], b)
	return ListValue{data: newSliceListSized(newElems, l.ByteSize()+other.ByteSize()-listVarOverhead)}
}

// Elements returns the internal slice for iteration
func (l ListValue) Elements() []Value {
	return l.data.Elements()
}

// InsertAt returns a new list with value inserted at index (1-based, COW)
func (l ListValue) InsertAt(index int, value Value) ListValue {
	elements := l.data.Elements()

	// Clamp index to valid range [1, len+1]
	if index < 1 {
		index = 1
	}
	if index > len(elements)+1 {
		index = len(elements) + 1
	}

	// Create new slice with space for inserted element
	newElems := make([]Value, len(elements)+1)

	// Convert to 0-based
	idx0 := index - 1

	// Copy elements before insertion point
	copy(newElems[:idx0], elements[:idx0])

	// Insert new value
	newElems[idx0] = value

	// Copy elements after insertion point
	copy(newElems[idx0+1:], elements[idx0:])

	return ListValue{data: newSliceList(newElems)}
}

// DeleteAt returns a new list with element at index removed (1-based, COW)
func (l ListValue) DeleteAt(index int) ListValue {
	elements := l.data.Elements()

	// Check bounds
	if index < 1 || index > len(elements) {
		return l // Out of bounds - return unchanged
	}

	// Create new slice without the element
	newElems := make([]Value, len(elements)-1)

	// Convert to 0-based
	idx0 := index - 1

	// Copy elements before deletion point
	copy(newElems[:idx0], elements[:idx0])

	// Copy elements after deletion point
	copy(newElems[idx0:], elements[idx0+1:])

	return ListValue{data: newSliceList(newElems)}
}

// Slice returns a new list containing elements from start to end (1-based, inclusive)
func (l ListValue) Slice(start, end int) ListValue {
	return ListValue{data: l.data.Slice(start, end)}
}
