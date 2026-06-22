package types

// Byte-size accounting for MOO values, used to enforce max_list_value_bytes /
// max_map_value_bytes and to implement the value_bytes() builtin. The numbers
// mirror ToastStunt's value_bytes (sizeof(Var) base plus payload).
//
// This lives in the types package (not builtins) so list values can cache their
// own size incrementally: appending to a list is O(1) for size accounting
// instead of re-walking the whole list on every append.

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
	switch val := v.(type) {
	case IntValue:
		return valueVarSize
	case FloatValue:
		return valueVarSize + 8
	case StrValue:
		return valueVarSize + val.Len() + 1
	case ObjValue:
		return valueVarSize
	case ErrValue:
		return valueVarSize
	case ListValue:
		return val.ByteSize()
	case MapValue:
		size := listVarOverhead // map Var + overhead
		for _, pair := range val.Pairs() {
			size += ValueBytes(pair[0]) + ValueBytes(pair[1])
		}
		return size
	case WaifValue:
		// Waif Var + class ref (waif properties not included, matches Toast)
		return listVarOverhead
	default:
		return valueVarSize
	}
}
