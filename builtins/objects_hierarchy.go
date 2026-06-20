package builtins

import (
	"sort"
	"strings"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

// builtinParent implements parent(object)
// Returns the first parent of an object
func builtinParent(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	objID, ok := args[0].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Check for invalid object references (E_INVARG for $nothing, etc.)
	if objID < 0 {
		return types.Err(types.E_INVARG)
	}

	parentID, errCode := store.Parent(objID)
	if errCode != types.E_NONE {
		// Check if recycled (E_INVARG) vs never existed (E_INVIND)
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	return types.Ok(types.NewObj(parentID))
}

// builtinParents implements parents(object)
// Returns list of all direct parents
// Waifs have no parents (E_INVARG)
func builtinParents(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Waifs have no parents
	if args[0].IsWaif() {
		return types.Err(types.E_INVARG)
	}

	objID, ok := args[0].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Check for invalid object references
	if objID < 0 {
		return types.Err(types.E_INVARG)
	}

	parentIDs, errCode := store.Parents(objID)
	if errCode != types.E_NONE {
		// Check if recycled (E_INVARG) vs never existed (E_INVIND)
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	return types.Ok(types.NewList(objIDsToValues(parentIDs)))
}

// builtinChildren implements children(object)
// Returns list of direct children
// Waifs have no children (E_INVARG)
func builtinChildren(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Waifs have no children
	if args[0].IsWaif() {
		return types.Err(types.E_INVARG)
	}

	objID, ok := args[0].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Check for invalid object references
	if objID < 0 {
		return types.Err(types.E_INVARG)
	}

	childIDs, errCode := store.Children(objID)
	if errCode != types.E_NONE {
		// Check if recycled (E_INVARG) vs never existed (E_INVIND)
		if store.IsRecycled(objID) {
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
func builtinChparent(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	// ToastStunt's chparent takes exactly two arguments (function_info reports
	// {"chparent", 2, 2, ...}); a third argument is E_ARGS.
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objID, ok := args[0].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	newParentID, ok := args[1].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Check for invalid object references
	if objID < 0 {
		return types.Err(types.E_INVARG)
	}

	if !store.Valid(objID) {
		return types.Err(types.E_INVIND)
	}

	// Check for cycles BEFORE validating new parent existence
	// This ensures self-parenting returns E_RECMOVE, not E_INVARG
	if objID == newParentID {
		return types.Err(types.E_RECMOVE)
	}

	// Check for invalid new parent
	// $nothing (-1) is valid and means no parent
	if newParentID < -1 {
		return types.Err(types.E_INVARG)
	}

	if newParentID != types.ObjNothing {
		if !store.Valid(newParentID) {
			return types.Err(types.E_INVARG)
		}
	}

	// Check if new parent is a descendant of object (would create cycle)
	if newParentID != types.ObjNothing && store.HasDescendant(objID, newParentID) {
		return types.Err(types.E_RECMOVE)
	}

	// Check for direct property conflicts between obj and new parent
	// If obj defines a property that new_parent or its ancestors also define, that's E_INVARG
	// (This is different from inherited properties, which can be shadowed)
	if newParentID != types.ObjNothing {
		conflict, errCode := store.HasDefinedPropertyConflictWithAncestry(objID, []types.ObjID{newParentID})
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		if conflict {
			return types.Err(types.E_INVARG)
		}
	}

	// Check for property conflicts: only chparent-added descendants of obj
	// cannot define properties that are also defined on new_parent or its ancestors.
	if newParentID != types.ObjNothing {
		newParentProps, errCode := store.DefinedPropertyNamesInAncestry(newParentID)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		conflict, errCode := store.HasChparentDescendantPropertyConflict(objID, newParentProps)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		if conflict {
			return types.Err(types.E_INVARG)
		}
	}

	if !ctx.IsWizard && newParentID != types.ObjNothing {
		ownerID, errCode := store.ObjectOwner(newParentID)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		hasFertile, errCode := store.HasObjectFlag(newParentID, dbstore.FlagFertile)
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
	if newParentID == types.ObjNothing {
		newParents = []types.ObjID{}
	} else {
		newParents = []types.ObjID{newParentID}
	}
	if errCode := store.ChangeParents(objID, newParents); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinChparents implements chparents(object, parents_list)
// Changes object's parents (multiple inheritance)
func builtinChparents(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objID, ok := args[0].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	parentsList, ok := args[1].AsList()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	if !store.Valid(objID) {
		return types.Err(types.E_INVIND)
	}

	// Convert list to ObjIDs - check cycles and duplicates BEFORE validation
	elements := parentsList.Elements()
	newParents := make([]types.ObjID, len(elements))
	seenParents := make(map[types.ObjID]bool)

	for i, elem := range elements {
		parentID, ok := elem.AsObjID()
		if !ok {
			return types.Err(types.E_TYPE)
		}

		// Check for self-parenting FIRST (before validating parent exists)
		if parentID == objID {
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
		if store.HasDescendant(objID, parentID) {
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

	conflict, errCode := store.HasDefinedPropertyConflictWithAncestry(objID, newParents)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if conflict {
		return types.Err(types.E_INVARG)
	}

	// Check for property conflicts: only chparent-added descendants of obj
	// cannot define properties that are also defined on new parents or their ancestors.
	conflict, errCode = store.HasChparentDescendantPropertyConflict(objID, allNewParentProps)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if conflict {
		return types.Err(types.E_INVARG)
	}

	// TODO: Check permissions and fertile flags (Layer 8.5)

	// Note: ToastStunt does NOT invalidate anonymous descendants when the parent
	// hierarchy changes; they remain valid.

	if errCode := store.ChangeParents(objID, newParents); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinAncestors implements ancestors(object [, include_self])
// Returns list of all ancestors in inheritance order
func builtinAncestors(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	objID, ok := args[0].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	includeSelf := false
	if len(args) == 2 {
		includeSelf = args[1].Truthy()
	}

	ancestorIDs, errCode := store.Ancestors(objID, includeSelf)
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewList(objIDsToValues(ancestorIDs)))
}

// builtinDescendants implements descendants(object [, include_self])
// Returns list of all descendants in inheritance order
func builtinDescendants(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	objID, ok := args[0].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	includeSelf := false
	if len(args) == 2 {
		includeSelf = args[1].Truthy()
	}

	descendantIDs, errCode := store.Descendants(objID, includeSelf)
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewList(objIDsToValues(descendantIDs)))
}

// builtinIsa implements isa(object, ancestor[, return_object])
// Returns true if object inherits from ancestor, or the matching ancestor object
// when return_object is truthy.
func builtinIsa(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	objID, ok := args[0].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	var ancestors []types.ObjID
	switch args[1].Kind() {
	case types.KindObj, types.KindAnon:
		ancestors = append(ancestors, args[1].ObjNum())
	case types.KindList:
		ancestorVal := args[1].List()
		for i := 1; i <= ancestorVal.Len(); i++ {
			parentID, ok := ancestorVal.Get(i).AsObjID()
			if !ok {
				return types.Err(types.E_TYPE)
			}
			ancestors = append(ancestors, parentID)
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

	if !store.Valid(objID) {
		return noMatch()
	}

	for _, ancestorID := range ancestors {
		if !store.Valid(ancestorID) {
			continue
		}

		if store.HasAncestor(objID, ancestorID) {
			if returnObject {
				return types.Ok(types.NewObj(ancestorID))
			}
			return types.Ok(types.NewInt(1))
		}
	}

	return noMatch()
}

func builtinLocateByName(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	needle, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	needleStr := strings.TrimSpace(needle)
	if needleStr == "" {
		return types.Ok(types.NewList([]types.Value{}))
	}

	caseSensitive := false
	if len(args) == 2 {
		cs, ok := args[1].AsInt()
		if !ok {
			return types.Err(types.E_TYPE)
		}
		caseSensitive = cs != 0
	}

	matchingIDs := store.ObjectIDsByNameSubstring(needleStr, caseSensitive)
	matches := make([]types.Value, 0, len(matchingIDs))
	for _, id := range matchingIDs {
		matches = append(matches, types.NewObj(id))
	}
	return types.Ok(types.NewList(matches))
}

func builtinLocations(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	objID, ok := args[0].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if !store.Valid(objID) {
		return types.Err(types.E_INVIND)
	}

	var (
		baseID      types.ObjID
		hasBase     bool
		checkParent bool
	)
	if len(args) >= 2 {
		base, ok := args[1].AsObjID()
		if !ok {
			return types.Err(types.E_TYPE)
		}
		baseID = base
		hasBase = true
	}
	if len(args) == 3 {
		flag, ok := args[2].AsInt()
		if !ok {
			return types.Err(types.E_TYPE)
		}
		checkParent = flag != 0
	}

	out := make([]types.Value, 0)
	currentID := objID
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

func builtinOwnedObjects(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	ownerID, ok := args[0].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if !store.Valid(ownerID) {
		return types.Err(types.E_INVIND)
	}
	ownedIDs := store.ObjectsOwnedBy(ownerID)
	out := make([]types.Value, 0, len(ownedIDs))
	for _, id := range ownedIDs {
		out = append(out, types.NewObj(id))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ObjNum() < out[j].ObjNum()
	})
	return types.Ok(types.NewList(out))
}

func builtinRecycledObjects(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

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

func builtinNextRecycledObject(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	start := types.ObjID(-1)
	if len(args) == 1 {
		switch args[0].Kind() {
		case types.KindObj, types.KindAnon:
			start = args[0].ObjNum()
		case types.KindInt:
			start = types.ObjID(args[0].Int())
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

func builtinRecreate(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store
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
	objID, ok := args[0].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	parent := types.ObjNothing
	owner := ctx.Programmer
	if len(args) >= 2 {
		p, ok := args[1].AsObjID()
		if !ok {
			return types.Err(types.E_TYPE)
		}
		parent = p
	}
	if len(args) == 3 {
		o, ok := args[2].AsObjID()
		if !ok {
			return types.Err(types.E_TYPE)
		}
		owner = o
	}
	if err := store.Recreate(objID, parent, owner); err != nil {
		return types.Err(types.E_INVARG)
	}

	result := types.Ok(types.NewObj(objID))
	if !store.Valid(objID) {
		return result
	}

	initResult := registry.CallVerb(objID, "initialize", []types.Value{}, ctx)
	if initResult.Flow == types.FlowException && initResult.Error != types.E_VERBNF {
		return initResult
	}
	return result
}

func builtinWaifStats(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

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
