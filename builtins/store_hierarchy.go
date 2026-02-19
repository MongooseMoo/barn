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
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	needle, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	needleStr := strings.TrimSpace(needle.Value())
	if needleStr == "" {
		return types.Ok(types.NewList([]types.Value{}))
	}

	caseSensitive := false
	if len(args) == 2 {
		cs, ok := args[1].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		caseSensitive = cs.Val != 0
	}

	searchNeedle := needleStr
	if !caseSensitive {
		searchNeedle = strings.ToLower(searchNeedle)
	}

	matches := make([]types.Value, 0)
	for _, obj := range store.All() {
		name := strings.TrimSpace(obj.Name)
		if !caseSensitive {
			name = strings.ToLower(name)
		}
		if strings.Contains(name, searchNeedle) {
			matches = append(matches, types.NewObj(obj.ID))
		}
	}
	return types.Ok(types.NewList(matches))
}

func builtinLocations(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	obj := store.Get(objVal.ID())
	if obj == nil {
		return types.Err(types.E_INVIND)
	}

	var (
		baseID      types.ObjID
		hasBase     bool
		checkParent bool
	)
	if len(args) >= 2 {
		baseVal, ok := args[1].(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		baseID = baseVal.ID()
		hasBase = true
	}
	if len(args) == 3 {
		flag, ok := args[2].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		checkParent = flag.Val != 0
	}

	out := make([]types.Value, 0)
	current := obj
	for current != nil && current.Location != types.ObjNothing {
		locID := current.Location

		if hasBase {
			if !checkParent && locID == baseID {
				break
			}
			if checkParent && objectHasAncestor(store, locID, baseID) {
				break
			}
		}

		out = append(out, types.NewObj(locID))
		current = store.Get(locID)
	}

	return types.Ok(types.NewList(out))
}

func objectHasAncestor(store *db.Store, objID, ancestorID types.ObjID) bool {
	if objID == ancestorID {
		return true
	}
	obj := store.Get(objID)
	if obj == nil {
		return false
	}
	visited := map[types.ObjID]bool{}
	queue := append([]types.ObjID{}, obj.Parents...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if visited[id] {
			continue
		}
		visited[id] = true
		if id == ancestorID {
			return true
		}
		parent := store.Get(id)
		if parent != nil {
			queue = append(queue, parent.Parents...)
		}
	}
	return false
}

func builtinOwnedObjects(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	owner, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if !store.Valid(owner.ID()) {
		return types.Err(types.E_INVIND)
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
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	start := types.ObjID(-1)
	if len(args) == 1 {
		switch startArg := args[0].(type) {
		case types.ObjValue:
			start = startArg.ID()
		case types.IntValue:
			start = types.ObjID(startArg.Val)
		default:
			return types.Err(types.E_TYPE)
		}
		if start == types.ObjNothing {
			return types.Err(types.E_INVARG)
		}
		if start > store.MaxObject() {
			return types.Err(types.E_INVARG)
		}
	}

	upper := store.NextID()
	for id := start + 1; id < upper; id++ {
		if store.IsRecycled(id) {
			return types.Ok(types.NewObj(id))
		}
	}
	return types.Ok(types.NewInt(0))
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
