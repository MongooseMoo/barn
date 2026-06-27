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

// containsWaif reports whether val transitively refers to the target waif,
// used to reject recursive containment (E_RECMOVE) on waif property assignment.
//
// Matches ToastStunt's refers_to (waif.cc:236-268): the leaf "is this the
// target?" test is WAIF INSTANCE IDENTITY — Toast compares the underlying
// `Waif *` pointer (waif.cc:250 `target.v.waif == key.v.waif`), NOT class/owner.
// After F4 a WaifValue is a thin handle over a shared *waifData, so identity is
// WaifValue.Equal (data-pointer equality, F14). Two independently created waifs
// that happen to share class+owner are DISTINCT instances and must not collide.
//
// Like Toast (waif.cc:252-256) it also recurses into the waif's own property
// values, plus nested lists/maps. A visited set keyed on waif identity guards
// against cycles formed by waif aliasing so traversal always terminates.
func containsWaif(val types.Value, waif types.WaifValue) bool {
	return containsWaifVisited(val, waif, nil)
}

func containsWaifVisited(val types.Value, waif types.WaifValue, visited map[types.WaifValue]bool) bool {
	switch v := val.(type) {
	case types.WaifValue:
		// Leaf: same underlying waif instance (pointer identity), not class+owner.
		if v.Equal(waif) {
			return true
		}
		// Recurse into the waif's own property values (Toast waif.cc:252-256),
		// guarding against aliasing cycles.
		if visited[v] {
			return false
		}
		if visited == nil {
			visited = make(map[types.WaifValue]bool)
		}
		visited[v] = true
		for _, name := range v.PropertyNames() {
			if prop, ok := v.GetProperty(name); ok {
				if containsWaifVisited(prop, waif, visited) {
					return true
				}
			}
		}
	case types.ListValue:
		for i := 1; i <= v.Len(); i++ {
			if containsWaifVisited(v.Get(i), waif, visited) {
				return true
			}
		}
	case types.MapValue:
		for _, pair := range v.Pairs() {
			if containsWaifVisited(pair[0], waif, visited) || containsWaifVisited(pair[1], waif, visited) {
				return true
			}
		}
	}
	return false
}
