package types

// Byte-size accounting for MOO values, used to enforce max_list_value_bytes /
// max_map_value_bytes and to implement the value_bytes() builtin. The numbers
// mirror ToastStunt's value_bytes (sizeof(Var) base plus payload).
//
// This lives in the types package (not builtins) so list values can cache their
// own size incrementally: appending to a list is O(1) for size accounting
// instead of re-walking the whole list on every append.

import "math"

const (
	// valueVarSize is sizeof(Var) in Toast - the base size for any value.
	valueVarSize = 16
	// listVarOverhead is the fixed overhead of a list value: the list Var plus
	// the length Var (matches Toast's sizeof(Var) + list_sizeof base).
	listVarOverhead = valueVarSize + valueVarSize
)

// ValueBytes returns the Toast-equivalent byte size of a value.
// For lists it reads the value's cached size (O(1)); other composite types are
// walked, matching the previous behaviour exactly.
func ValueBytes(v Value) int {
	switch v.Type() {
	case TYPE_INT:
		return valueVarSize
	case TYPE_FLOAT:
		return valueVarSize + 8
	case TYPE_STR:
		return valueVarSize + v.Len() + 1
	case TYPE_OBJ, TYPE_ANON:
		return valueVarSize
	case TYPE_ERR:
		return valueVarSize
	case TYPE_LIST:
		return v.ByteSize()
	case TYPE_MAP:
		return v.goMap().mapBytes()
	case TYPE_WAIF:
		// Waif Var + class ref (waif properties not included, matches Toast).
		return listVarOverhead
	default:
		return valueVarSize
	}
}

// RefusedArgListBytes is what ListBytes counts for None. A verb without the d
// flag marks a call's argument list that passed max_list_value_bytes by
// putting None in its first argument's slot (Toast pushes E_QUOTA in place of
// the list and keeps evaluating). The count is above every limit, so the
// call's own check, which runs only for a list of at least the smallest limit,
// always looks at the mark. None is never a MOO value, so nothing else counts
// it.
const RefusedArgListBytes = math.MaxInt32

// sizedTags holds the tags whose ValueBytes is more than valueVarSize. The
// internal tags are negative, so they never index it.
const sizedTags = 1<<TYPE_STR | 1<<TYPE_LIST | 1<<TYPE_MAP | 1<<TYPE_FLOAT | 1<<TYPE_WAIF

// ListBytes is ValueBytes of a list holding elements, without building it. It
// runs on every multi-argument call, so it reads cached collection sizes in
// place and calls ValueBytes only for an uncached one.
func ListBytes(elements []Value) int {
	size := listVarOverhead + valueVarSize*len(elements)
	for i := range elements {
		e := &elements[i]
		// Most arguments are values with no payload: one test skips them.
		if uint(e.tag) < 64 && sizedTags>>uint(e.tag)&1 == 0 {
			continue
		}
		switch e.tag {
		case tagNone:
			size += RefusedArgListBytes
			continue
		case TYPE_MAP:
			if cached := (*goMap)(e.ref).byteSize; cached > 0 {
				size += cached - valueVarSize
				continue
			}
		case TYPE_LIST:
			if cached := (*sliceList)(e.ref).byteSize.Load(); cached != 0 {
				size += int(cached) - valueVarSize
				continue
			}
		case TYPE_STR:
			size += e.Len() + 1
			continue
		case TYPE_FLOAT, TYPE_WAIF:
		default:
			continue
		}
		size += ValueBytes(*e) - valueVarSize
	}
	return size
}
