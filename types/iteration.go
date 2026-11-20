package types

// IterationColumns preserves the container's iteration snapshot as separate
// value/key columns. Keys remain roots even when the loop binds only values.
// Non-map columns use integer zero as the positional-key sentinel.
func (container Value) IterationColumns() (values, keys Value, code ErrorCode) {
	keys = NewInt(0)
	switch container.Type() {
	case TYPE_LIST:
		values = container
	case TYPE_MAP:
		valueColumn, keyColumn := container.MapColumns()
		values, keys = NewList(valueColumn), NewList(keyColumn)
	case TYPE_STR:
		chars := SplitChars(container.Str())
		elements := make([]Value, len(chars))
		for index, char := range chars {
			elements[index] = NewStr(char)
		}
		values = NewList(elements)
	default:
		return None, None, E_TYPE
	}
	return values, keys, E_NONE
}
