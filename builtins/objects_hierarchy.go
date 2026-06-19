package builtins

import (
	dbstore "barn/db/store"
	"barn/types"
	"sort"
	"strings"
)

// builtinParent implements parent(object)
// Returns the first parent of an object
func builtinParent(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Check for invalid object references (E_INVARG for $nothing, etc.)
	if objVal.ID() < 0 {
		return types.Err(types.E_INVARG)
	}

	parentID, errCode := store.Parent(objVal.ID())
	if errCode != types.E_NONE {
		// Check if recycled (E_INVARG) vs never existed (E_INVIND)
		if store.IsRecycled(objVal.ID()) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	return types.Ok(types.NewObj(parentID))
}

// builtinParents implements parents(object)
// Returns list of all direct parents
// Waifs have no parents (E_INVARG)
func builtinParents(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Waifs have no parents
	if _, ok := args[0].(types.WaifValue); ok {
		return types.Err(types.E_INVARG)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Check for invalid object references
	if objVal.ID() < 0 {
		return types.Err(types.E_INVARG)
	}

	parentIDs, errCode := store.Parents(objVal.ID())
	if errCode != types.E_NONE {
		// Check if recycled (E_INVARG) vs never existed (E_INVIND)
		if store.IsRecycled(objVal.ID()) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	return types.Ok(types.NewList(objIDsToValues(parentIDs)))
}

// builtinChildren implements children(object)
// Returns list of direct children
// Waifs have no children (E_INVARG)
func builtinChildren(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Waifs have no children
	if _, ok := args[0].(types.WaifValue); ok {
		return types.Err(types.E_INVARG)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Check for invalid object references
	if objVal.ID() < 0 {
		return types.Err(types.E_INVARG)
	}

	childIDs, errCode := store.Children(objVal.ID())
	if errCode != types.E_NONE {
		// Check if recycled (E_INVARG) vs never existed (E_INVIND)
		if store.IsRecycled(objVal.ID()) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	return types.Ok(types.NewList(objIDsToValues(childIDs)))
}

func objIDsToValues(ids []types.ObjID) []types.Value {
	values := make([]types.Value, len(ids))
	for i, id := range ids {
		values[i] = types.NewObj(id)
	}
	return values
}

// builtinChparent implements chparent(object, new_parent)
// Changes object's parent (single inheritance)
func builtinChparent(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	// ToastStunt's chparent takes exactly two arguments (function_info reports
	// {"chparent", 2, 2, ...}); a third argument is E_ARGS.
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	newParentVal, ok := args[1].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Check for invalid object references
	if objVal.ID() < 0 {
		return types.Err(types.E_INVARG)
	}

	if !store.Valid(objVal.ID()) {
		return types.Err(types.E_INVIND)
	}

	// Check for cycles BEFORE validating new parent existence
	// This ensures self-parenting returns E_RECMOVE, not E_INVARG
	if objVal.ID() == newParentVal.ID() {
		return types.Err(types.E_RECMOVE)
	}

	// Check for invalid new parent
	// $nothing (-1) is valid and means no parent
	if newParentVal.ID() < -1 {
		return types.Err(types.E_INVARG)
	}

	if newParentVal.ID() != types.ObjNothing {
		if !store.Valid(newParentVal.ID()) {
			return types.Err(types.E_INVARG)
		}
	}

	// Check if new parent is a descendant of object (would create cycle)
	if newParentVal.ID() != types.ObjNothing && store.HasDescendant(objVal.ID(), newParentVal.ID()) {
		return types.Err(types.E_RECMOVE)
	}

	// Check for direct property conflicts between obj and new parent
	// If obj defines a property that new_parent or its ancestors also define, that's E_INVARG
	// (This is different from inherited properties, which can be shadowed)
	if newParentVal.ID() != types.ObjNothing {
		conflict, errCode := store.HasDefinedPropertyConflictWithAncestry(objVal.ID(), []types.ObjID{newParentVal.ID()})
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		if conflict {
			return types.Err(types.E_INVARG)
		}
	}

	// Check for property conflicts: only chparent-added descendants of obj
	// cannot define properties that are also defined on new_parent or its ancestors.
	if newParentVal.ID() != types.ObjNothing {
		newParentProps, errCode := store.DefinedPropertyNamesInAncestry(newParentVal.ID())
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		conflict, errCode := store.HasChparentDescendantPropertyConflict(objVal.ID(), newParentProps)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		if conflict {
			return types.Err(types.E_INVARG)
		}
	}

	if !ctx.IsWizard && newParentVal.ID() != types.ObjNothing {
		ownerID, errCode := store.ObjectOwner(newParentVal.ID())
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		hasFertile, errCode := store.HasObjectFlag(newParentVal.ID(), dbstore.FlagFertile)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		isOwner := ownerID == ctx.Programmer
		if !isOwner && !hasFertile {
			return types.Err(types.E_PERM)
		}
	}

	// Note: ToastStunt does NOT invalidate anonymous descendants when the parent
	// hierarchy changes; they remain valid.

	var newParents []types.ObjID
	if newParentVal.ID() == types.ObjNothing {
		newParents = []types.ObjID{}
	} else {
		newParents = []types.ObjID{newParentVal.ID()}
	}
	if errCode := store.ChangeParents(objVal.ID(), newParents); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinChparents implements chparents(object, parents_list)
// Changes object's parents (multiple inheritance)
func builtinChparents(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	parentsList, ok := args[1].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	if !store.Valid(objVal.ID()) {
		return types.Err(types.E_INVIND)
	}

	// Convert list to ObjIDs - check cycles and duplicates BEFORE validation
	elements := parentsList.Elements()
	newParents := make([]types.ObjID, len(elements))
	seenParents := make(map[types.ObjID]bool)

	for i, elem := range elements {
		parentVal, ok := elem.(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}

		parentID := parentVal.ID()

		// Check for self-parenting FIRST (before validating parent exists)
		if parentID == objVal.ID() {
			return types.Err(types.E_RECMOVE)
		}

		// Check for duplicate parents in list
		if seenParents[parentID] {
			return types.Err(types.E_INVARG)
		}
		seenParents[parentID] = true

		// Now validate parent exists
		if !store.Valid(parentID) {
			return types.Err(types.E_INVARG)
		}

		// Check if parent is a descendant of object (would create cycle)
		if store.HasDescendant(objVal.ID(), parentID) {
			return types.Err(types.E_RECMOVE)
		}

		newParents[i] = parentID
	}

	duplicateProps, errCode := store.HasDuplicateDefinedPropertyAmong(newParents)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if duplicateProps {
		return types.Err(types.E_INVARG)
	}

	// Check for direct property conflicts between obj and new parents
	// If obj defines a property that any new parent or their ancestors also define, that's E_INVARG
	allNewParentProps := make(map[string]bool)
	for _, parentID := range newParents {
		props, errCode := store.DefinedPropertyNamesInAncestry(parentID)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		for name := range props {
			allNewParentProps[name] = true
		}
	}

	conflict, errCode := store.HasDefinedPropertyConflictWithAncestry(objVal.ID(), newParents)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if conflict {
		return types.Err(types.E_INVARG)
	}

	// Check for property conflicts: only chparent-added descendants of obj
	// cannot define properties that are also defined on new parents or their ancestors.
	conflict, errCode = store.HasChparentDescendantPropertyConflict(objVal.ID(), allNewParentProps)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if conflict {
		return types.Err(types.E_INVARG)
	}

	// TODO: Check permissions and fertile flags (Layer 8.5)

	// Note: ToastStunt does NOT invalidate anonymous descendants when the parent
	// hierarchy changes; they remain valid.

	if errCode := store.ChangeParents(objVal.ID(), newParents); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinAncestors implements ancestors(object [, include_self])
// Returns list of all ancestors in inheritance order
func builtinAncestors(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	includeSelf := false
	if len(args) == 2 {
		includeSelf = args[1].Truthy()
	}

	ancestorIDs, errCode := store.Ancestors(objVal.ID(), includeSelf)
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewList(objIDsToValues(ancestorIDs)))
}

// builtinDescendants implements descendants(object [, include_self])
// Returns list of all descendants in inheritance order
func builtinDescendants(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	includeSelf := false
	if len(args) == 2 {
		includeSelf = args[1].Truthy()
	}

	descendantIDs, errCode := store.Descendants(objVal.ID(), includeSelf)
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewList(objIDsToValues(descendantIDs)))
}

// builtinIsa implements isa(object, ancestor[, return_object])
// Returns true if object inherits from ancestor, or the matching ancestor object
// when return_object is truthy.
func builtinIsa(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	var ancestors []types.ObjID
	switch ancestorVal := args[1].(type) {
	case types.ObjValue:
		ancestors = append(ancestors, ancestorVal.ID())
	case types.ListValue:
		for i := 1; i <= ancestorVal.Len(); i++ {
			parentVal, ok := ancestorVal.Get(i).(types.ObjValue)
			if !ok {
				return types.Err(types.E_TYPE)
			}
			ancestors = append(ancestors, parentVal.ID())
		}
	default:
		return types.Err(types.E_TYPE)
	}

	returnObject := len(args) == 3 && args[2].Truthy()
	noMatch := func() types.Result {
		if returnObject {
			return types.Ok(types.NewObj(types.NOTHING))
		}
		return types.Ok(types.NewInt(0))
	}

	if !store.Valid(objVal.ID()) {
		return noMatch()
	}

	for _, ancestorID := range ancestors {
		if !store.Valid(ancestorID) {
			continue
		}

		if store.HasAncestor(objVal.ID(), ancestorID) {
			if returnObject {
				return types.Ok(types.NewObj(ancestorID))
			}
			return types.Ok(types.NewInt(1))
		}
	}

	return noMatch()
}

func builtinLocateByName(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

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

	matchingIDs := store.ObjectIDsByNameSubstring(needleStr, caseSensitive)
	matches := make([]types.Value, 0, len(matchingIDs))
	for _, id := range matchingIDs {
		matches = append(matches, types.NewObj(id))
	}
	return types.Ok(types.NewList(matches))
}

func builtinLocations(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if !store.Valid(objVal.ID()) {
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
	currentID := objVal.ID()
	for {
		locID, errCode := store.Location(currentID)
		if errCode != types.E_NONE || locID == types.ObjNothing {
			break
		}

		if hasBase {
			if !checkParent && locID == baseID {
				break
			}
			if checkParent && (locID == baseID || store.HasAncestor(locID, baseID)) {
				break
			}
		}

		out = append(out, types.NewObj(locID))
		currentID = locID
	}

	return types.Ok(types.NewList(out))
}

func builtinOwnedObjects(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

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
	ownedIDs := store.ObjectsOwnedBy(owner.ID())
	out := make([]types.Value, 0, len(ownedIDs))
	for _, id := range ownedIDs {
		out = append(out, types.NewObj(id))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].(types.ObjValue).ID() < out[j].(types.ObjValue).ID()
	})
	return types.Ok(types.NewList(out))
}

func builtinRecycledObjects(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

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

func builtinNextRecycledObject(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

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

func builtinRecreate(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}
	registry, ok := ctx.Registry.(*Registry)
	if !ok {
		return types.Err(types.E_INVARG)
	}

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

	result := types.Ok(types.NewObj(obj.ID()))
	if !store.Valid(obj.ID()) {
		return result
	}

	initResult := registry.CallVerb(obj.ID(), "initialize", []types.Value{}, ctx)
	if initResult.Flow == types.FlowException && initResult.Error != types.E_VERBNF {
		return initResult
	}
	return result
}

func builtinWaifStats(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*dbstore.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

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
