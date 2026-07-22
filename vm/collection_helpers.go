package vm

import (
	"unsafe"

	"barn/builtins"
	"barn/kernel"
	"barn/types"
)

// isObjLike reports whether v is an object reference: either a regular object
// (TYPE_OBJ) or an anonymous object (TYPE_ANON). The pre-struct code expressed
// this with a single `v.(types.ObjValue)` assertion, which matched both because
// an anonymous object was an ObjValue carrying an anonymous flag.
func isObjLike(v types.Value) bool {
	return v.Type() == types.TYPE_OBJ || v.Type() == types.TYPE_ANON
}

func setAtIndex(ctx *kernel.TaskContext, coll types.Value, index types.Value, value types.Value) (types.Value, types.ErrorCode) {
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
		if err := builtins.CheckListLimitForTask(ctx, result); err != types.E_NONE {
			return types.None, err
		}
		return result, types.E_NONE

	default:
		return types.None, types.E_TYPE
	}
}

// containsWaif reports whether val transitively refers to the target waif,
// used to reject recursive containment (E_RECMOVE) on waif property assignment.
//
// Matches ToastStunt's refers_to (waif.cc:236-268): the leaf "is this the
// target?" test is WAIF INSTANCE IDENTITY — Toast compares the underlying
// `Waif *` pointer (waif.cc:250 `target.v.waif == key.v.waif`), NOT class/owner.
// Under the de-boxed Value, identity is Value.Equal (waifRep-pointer equality,
// F14). Two independently created waifs that happen to share class+owner are
// DISTINCT instances and must not collide.
//
// Like Toast (waif.cc:252-256) it also recurses into the waif's own property
// values, plus nested lists/maps. A visited set keyed on waif identity
// (WaifIdentity, the GC-traced ref pointer) guards against cycles formed by waif
// aliasing so traversal always terminates.
func containsWaif(val types.Value, waif types.Value) bool {
	return containsWaifVisited(val, waif, nil)
}

func containsWaifVisited(val types.Value, waif types.Value, visited map[unsafe.Pointer]bool) bool {
	switch val.Type() {
	case types.TYPE_WAIF:
		// Leaf: same underlying waif instance (pointer identity), not class+owner.
		if val.Equal(waif) {
			return true
		}
		// Recurse into the waif's own property values (Toast waif.cc:252-256),
		// guarding against aliasing cycles.
		id := val.WaifIdentity()
		if visited[id] {
			return false
		}
		if visited == nil {
			visited = make(map[unsafe.Pointer]bool)
		}
		visited[id] = true
		for _, name := range val.PropertyNames() {
			if prop, ok := val.GetProperty(name); ok {
				if containsWaifVisited(prop, waif, visited) {
					return true
				}
			}
		}
	case types.TYPE_LIST:
		for i := 1; i <= val.Len(); i++ {
			if containsWaifVisited(val.Get(i), waif, visited) {
				return true
			}
		}
	case types.TYPE_MAP:
		for _, pair := range val.Pairs() {
			if containsWaifVisited(pair[0], waif, visited) || containsWaifVisited(pair[1], waif, visited) {
				return true
			}
		}
	}
	return false
}
