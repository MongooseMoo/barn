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
		size := listVarOverhead // map Var + overhead
		for _, pair := range v.Pairs() {
			size += ValueBytes(pair[0]) + ValueBytes(pair[1])
		}
		return size
	case TYPE_WAIF:
		// Waif Var + class ref (waif properties not included, matches Toast).
		return listVarOverhead
	default:
		return valueVarSize
	}
}
