package builtins

import (
	"barn/db"
	"barn/types"
	"sort"
	"strings"
)

func builtinLocateByName(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	needle, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	needleStr := strings.ToLower(strings.TrimSpace(needle.Value()))
	if needleStr == "" {
		return types.Ok(types.NewList([]types.Value{}))
	}

	var ids []types.ObjID
	if len(args) == 2 {
		list, ok := args[1].(types.ListValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		for i := 1; i <= list.Len(); i++ {
			obj, ok := list.Get(i).(types.ObjValue)
			if !ok {
				return types.Err(types.E_TYPE)
			}
			ids = append(ids, obj.ID())
		}
	} else {
		for _, obj := range store.All() {
			ids = append(ids, obj.ID)
		}
	}

	matches := make([]types.Value, 0)
	for _, id := range ids {
		obj := store.Get(id)
		if obj == nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(obj.Name))
		if strings.HasPrefix(name, needleStr) {
			matches = append(matches, types.NewObj(id))
		}
	}
	return types.Ok(types.NewList(matches))
}

func builtinLocations(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	switch v := args[0].(type) {
	case types.ObjValue:
		obj := store.Get(v.ID())
		if obj == nil {
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.NewObj(obj.Location))
	case types.ListValue:
		out := make([]types.Value, 0, v.Len())
		for i := 1; i <= v.Len(); i++ {
			objVal, ok := v.Get(i).(types.ObjValue)
			if !ok {
				return types.Err(types.E_TYPE)
			}
			obj := store.Get(objVal.ID())
			if obj == nil {
				out = append(out, types.NewObj(types.ObjNothing))
				continue
			}
			out = append(out, types.NewObj(obj.Location))
		}
		return types.Ok(types.NewList(out))
	default:
		return types.Err(types.E_TYPE)
	}
}

func builtinOwnedObjects(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	owner, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	out := make([]types.Value, 0)
	for _, obj := range store.All() {
		if obj.Owner == owner.ID() {
			out = append(out, types.NewObj(obj.ID))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].(types.ObjValue).ID() < out[j].(types.ObjValue).ID()
	})
	return types.Ok(types.NewList(out))
}

func builtinRecycledObjects(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	out := make([]types.Value, 0)
	upper := store.NextID()
	for id := types.ObjID(0); id < upper; id++ {
		if store.IsRecycled(id) {
			out = append(out, types.NewObj(id))
		}
	}
	return types.Ok(types.NewList(out))
}

func builtinNextRecycledObject(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	upper := store.NextID()
	for id := types.ObjID(0); id < upper; id++ {
		if store.IsRecycled(id) {
			return types.Ok(types.NewObj(id))
		}
	}
	return types.Ok(types.NewObj(types.ObjNothing))
}

func builtinRecreate(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	obj, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	parent := types.ObjNothing
	owner := ctx.Programmer
	if len(args) >= 2 {
		p, ok := args[1].(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		parent = p.ID()
	}
	if len(args) == 3 {
		o, ok := args[2].(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		owner = o.ID()
	}
	if err := store.Recreate(obj.ID(), parent, owner); err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewObj(obj.ID()))
}

func builtinWaifStats(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	byClass := store.WaifCountByClass()
	entries := make([]types.Value, 0, len(byClass))
	for classID, count := range byClass {
		entries = append(entries, types.NewMap([][2]types.Value{
			{types.NewStr("class"), types.NewObj(classID)},
			{types.NewStr("count"), types.NewInt(int64(count))},
		}))
	}
	result := types.NewMap([][2]types.Value{
		{types.NewStr("total"), types.NewInt(int64(store.WaifCount()))},
		{types.NewStr("classes"), types.NewList(entries)},
	})
	return types.Ok(result)
}
