package vm

import (
	"barn/builtins"
	"barn/types"
)

// isObjLike reports whether v is an object reference: either a regular object
// (TYPE_OBJ) or an anonymous object (TYPE_ANON). The pre-struct code expressed
// this with a single `v.(types.ObjValue)` assertion, which matched both because
// an anonymous object was an ObjValue carrying an anonymous flag.
func isObjLike(v types.Value) bool {
	return v.Type() == types.TYPE_OBJ || v.Type() == types.TYPE_ANON
}

func setAtIndex(coll types.Value, index types.Value, value types.Value) (types.Value, types.ErrorCode) {
	switch coll.Type() {
	case types.TYPE_LIST:
		if index.Type() != types.TYPE_INT {
			return types.None, types.E_TYPE
		}
		i := int(index.Int())
		if i < 1 || i > coll.Len() {
			return types.None, types.E_RANGE
		}
		result := coll.Set(i, value)
		if err := builtins.CheckListLimit(result); err != types.E_NONE {
			return types.None, err
		}
		return result, types.E_NONE

	case types.TYPE_STR:
		if index.Type() != types.TYPE_INT {
			return types.None, types.E_TYPE
		}
		i := int(index.Int())
		s := coll.Str()
		if i < 1 || i > len(s) {
			return types.None, types.E_RANGE
		}
		if value.Type() != types.TYPE_STR || len(value.Str()) != 1 {
			return types.None, types.E_INVARG
		}
		newStr := s[:i-1] + value.Str() + s[i:]
		if err := builtins.CheckStringLimit(newStr); err != types.E_NONE {
			return types.None, err
		}
		return types.NewStr(newStr), types.E_NONE

	case types.TYPE_MAP:
		if !types.IsValidMapKey(index) {
			return types.None, types.E_TYPE
		}
		result := coll.MapSet(index, value)
		if err := builtins.CheckMapLimit(result); err != types.E_NONE {
			return types.None, err
		}
		return result, types.E_NONE

	default:
		return types.None, types.E_TYPE
	}
}

func containsWaif(val types.Value, waif types.Value) bool {
	switch val.Type() {
	case types.TYPE_WAIF:
		return val.Class() == waif.Class() && val.Owner() == waif.Owner()
	case types.TYPE_LIST:
		for i := 1; i <= val.Len(); i++ {
			if containsWaif(val.Get(i), waif) {
				return true
			}
		}
	case types.TYPE_MAP:
		for _, pair := range val.Pairs() {
			if containsWaif(pair[0], waif) || containsWaif(pair[1], waif) {
				return true
			}
		}
	}
	return false
}
