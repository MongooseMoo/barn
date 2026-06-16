package vm

import (
	"barn/builtins"
	"barn/types"
)

func setAtIndex(coll types.Value, index types.Value, value types.Value) (types.Value, types.ErrorCode) {
	switch c := coll.(type) {
	case types.ListValue:
		idx, ok := index.(types.IntValue)
		if !ok {
			return nil, types.E_TYPE
		}
		i := int(idx.Val)
		if i < 1 || i > c.Len() {
			return nil, types.E_RANGE
		}
		result := c.Set(i, value)
		if err := builtins.CheckListLimit(result); err != types.E_NONE {
			return nil, err
		}
		return result, types.E_NONE

	case types.StrValue:
		idx, ok := index.(types.IntValue)
		if !ok {
			return nil, types.E_TYPE
		}
		i := int(idx.Val)
		s := c.Value()
		if i < 1 || i > len(s) {
			return nil, types.E_RANGE
		}
		newChar, ok := value.(types.StrValue)
		if !ok || len(newChar.Value()) != 1 {
			return nil, types.E_INVARG
		}
		newStr := s[:i-1] + newChar.Value() + s[i:]
		if err := builtins.CheckStringLimit(newStr); err != types.E_NONE {
			return nil, err
		}
		return types.NewStr(newStr), types.E_NONE

	case types.MapValue:
		if !types.IsValidMapKey(index) {
			return nil, types.E_TYPE
		}
		result := c.Set(index, value)
		if err := builtins.CheckMapLimit(result); err != types.E_NONE {
			return nil, err
		}
		return result, types.E_NONE

	default:
		return nil, types.E_TYPE
	}
}

func containsWaif(val types.Value, waif types.WaifValue) bool {
	switch v := val.(type) {
	case types.WaifValue:
		return v.Class() == waif.Class() && v.Owner() == waif.Owner()
	case types.ListValue:
		for i := 1; i <= v.Len(); i++ {
			if containsWaif(v.Get(i), waif) {
				return true
			}
		}
	case types.MapValue:
		for _, pair := range v.Pairs() {
			if containsWaif(pair[0], waif) || containsWaif(pair[1], waif) {
				return true
			}
		}
	}
	return false
}
