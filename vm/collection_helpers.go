package vm

import (
	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

// isObjLike reports whether v is an object reference: either a regular object
// (TYPE_OBJ) or an anonymous object (TYPE_ANON). The pre-struct code expressed
// this with a single `v.(types.ObjValue)` assertion, which matched both because
// an anonymous object was an ObjValue carrying an anonymous flag.
func isObjLike(v types.Value) bool {
	return v.Type() == types.TYPE_OBJ || v.Type() == types.TYPE_ANON
}

func setAtIndex(session *builtins.Session, ctx *kernel.TaskContext, coll types.Value, index types.Value, value types.Value) (types.Value, types.ErrorCode) {
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
		if err := session.CheckListLimitForTask(ctx, result); err != types.E_NONE {
			return types.None, err
		}
		return result, types.E_NONE

	case types.TYPE_STR:
		if index.Type() != types.TYPE_INT {
			return types.None, types.E_TYPE
		}
		i := int(index.Int())
		s := coll.Str()
		if i < 1 || i > coll.StrCharLen() {
			return types.None, types.E_RANGE
		}
		if value.Type() != types.TYPE_STR || value.StrCharLen() != 1 {
			return types.None, types.E_INVARG
		}
		start, end := i-1, i
		if !coll.StrIsSingleByte() {
			start = types.CharByteOffset(s, start)
			end = start + types.CharByteOffset(s[start:], 1)
		}
		newStr := s[:start] + value.Str() + s[end:]
		if err := session.CheckStringLimitForTask(ctx, newStr); err != types.E_NONE {
			return types.None, err
		}
		return types.NewStr(newStr), types.E_NONE

	case types.TYPE_MAP:
		if !types.IsValidMapKey(index) {
			return types.None, types.E_TYPE
		}
		result := coll.MapSet(index, value)
		if err := session.CheckListLimitForTask(ctx, result); err != types.E_NONE {
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
// (WaifIdentity, an opaque GC-traced token) guards against cycles formed by waif
// aliasing so traversal always terminates.
func containsWaif(tx *dbstore.StoreTxn, val types.Value, waif types.Value) (bool, types.ErrorCode) {
	return containsWaifVisited(tx, val, waif, make(map[types.WaifIdentity]bool))
}

func containsWaifVisited(tx *dbstore.StoreTxn, val types.Value, waif types.Value, visited map[types.WaifIdentity]bool) (bool, types.ErrorCode) {
	var children []types.Value
	switch val.Type() {
	case types.TYPE_WAIF:
		if val.Equal(waif) {
			return true, types.E_NONE
		}
		id := val.WaifIdentity()
		if visited[id] {
			return false, types.E_NONE
		}
		visited[id] = true
		properties, ec := tx.WaifProperties(val)
		if ec != types.E_NONE {
			return false, ec
		}
		for _, property := range properties {
			children = append(children, property)
		}
	case types.TYPE_LIST:
		children = val.Elements()
	case types.TYPE_MAP:
		for _, pair := range val.Pairs() {
			children = append(children, pair[0], pair[1])
		}
	}
	for _, child := range children {
		found, ec := containsWaifVisited(tx, child, waif, visited)
		if found || ec != types.E_NONE {
			return found, ec
		}
	}
	return false, types.E_NONE
}
