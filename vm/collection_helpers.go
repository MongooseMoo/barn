package vm

import (
	"barn/builtins"
	"barn/types"
)

func setAtIndex(coll types.Value, index types.Value, value types.Value) (types.Value, types.ErrorCode) {
	switch coll.Kind() {
	case types.KindList:
		c := coll.List()
		idx, ok := index.AsInt()
		if !ok {
			return types.Value{}, types.E_TYPE
		}
		i := int(idx)
		if i < 1 || i > c.Len() {
			return types.Value{}, types.E_RANGE
		}
		result := c.Set(i, value)
		if err := builtins.CheckListLimit(result); err != types.E_NONE {
			return types.Value{}, err
		}
		return result.AsValue(), types.E_NONE

	case types.KindStr:
		idx, ok := index.AsInt()
		if !ok {
			return types.Value{}, types.E_TYPE
		}
		i := int(idx)
		s := coll.Str()
		if i < 1 || i > len(s) {
			return types.Value{}, types.E_RANGE
		}
		newChar, ok := value.AsStr()
		if !ok || len(newChar) != 1 {
			return types.Value{}, types.E_INVARG
		}
		newStr := s[:i-1] + newChar + s[i:]
		if err := builtins.CheckStringLimit(newStr); err != types.E_NONE {
			return types.Value{}, err
		}
		return types.NewStr(newStr), types.E_NONE

	case types.KindMap:
		c := coll.Map()
		if !types.IsValidMapKey(index) {
			return types.Value{}, types.E_TYPE
		}
		result := c.Set(index, value)
		if err := builtins.CheckMapLimit(result); err != types.E_NONE {
			return types.Value{}, err
		}
		return result.AsValue(), types.E_NONE

	default:
		return types.Value{}, types.E_TYPE
	}
}

func containsWaif(val types.Value, waif types.WaifValue) bool {
	switch val.Kind() {
	case types.KindWaif:
		v := val.Waif()
		return v.Class() == waif.Class() && v.Owner() == waif.Owner()
	case types.KindList:
		v := val.List()
		for i := 1; i <= v.Len(); i++ {
			if containsWaif(v.Get(i), waif) {
				return true
			}
		}
	case types.KindMap:
		v := val.Map()
		for _, pair := range v.Pairs() {
			if containsWaif(pair[0], waif) || containsWaif(pair[1], waif) {
				return true
			}
		}
	}
	return false
}
