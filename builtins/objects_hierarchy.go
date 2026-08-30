package builtins

import (
	"sort"
	"strings"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// builtinParent implements parent(object)
// Returns the first parent of an object
func builtinParent(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	// Check for invalid object references (E_INVARG for $nothing, etc.)
	if objVal.ID() < 0 {
		return types.Err(types.E_INVARG)
	}

	parentID, errCode := parentForRead(ctx, objVal.ID())
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewObj(parentID))
}

// builtinParents implements parents(object)
// Returns list of all direct parents
// Waifs have no parents (E_INVARG)
func builtinParents(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Waifs have no parents
	if args[0].Type() == types.TYPE_WAIF {
		return types.Err(types.E_INVARG)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	// Check for invalid object references
	if objVal.ID() < 0 {
		return types.Err(types.E_INVARG)
	}

	parentIDs, errCode := parentsForRead(ctx, objVal.ID())
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewList(objIDsToValues(parentIDs)))
}

// builtinChildren implements children(object)
// Returns list of direct children
// Waifs have no children (E_INVARG)
func builtinChildren(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Waifs have no children
	if args[0].Type() == types.TYPE_WAIF {
		return types.Err(types.E_INVARG)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	// Check for invalid object references
	if objVal.ID() < 0 {
		return types.Err(types.E_INVARG)
	}

	childIDs, errCode := childrenForRead(ctx, objVal.ID())
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
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
func builtinChparent(ctx *Execution, args []types.Value) types.Result {
	if errCode := flushStagedBeforeCoarse(ctx); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	store := ctx.Store

	// ToastStunt's chparent takes exactly two arguments (function_info reports
	// {"chparent", 2, 2, ...}); a third argument is E_ARGS.
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	newParentVal := args[1]
	if !isObjectRef(newParentVal) {
		return types.Err(types.E_TYPE)
	}

	// Check for invalid object references
	if objVal.ID() < 0 {
		return types.Err(types.E_INVARG)
	}

	if !validForRead(ctx, objVal.ID()) {
		return types.Err(types.E_INVARG)
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
		if !validForRead(ctx, newParentVal.ID()) {
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
		conflict, errCode := readTxn(ctx).HasDefinedPropertyConflictWithAncestry(objVal.ID(), []types.ObjID{newParentVal.ID()})
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
		newParentProps, errCode := readTxn(ctx).DefinedPropertyNamesInAncestry(newParentVal.ID())
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		conflict, errCode := readTxn(ctx).HasChparentDescendantPropertyConflict(objVal.ID(), newParentProps)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		if conflict {
			return types.Err(types.E_INVARG)
		}
	}

	if !ctx.IsWizard && newParentVal.ID() != types.ObjNothing {
		ownerID, errCode := objectOwnerForRead(ctx, newParentVal.ID())
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		hasFertile, errCode := hasObjectFlagForRead(ctx, newParentVal.ID(), dbstore.FlagFertile)
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
	tx := readTxn(ctx)
	oldParents, errCode := tx.Parents(objVal.ID())
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if errCode := store.ChangeParents(objVal.ID(), newParents); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	markLiveStoreMutated(ctx)
	adoptIDs := append([]types.ObjID{objVal.ID()}, oldParents...)
	adoptIDs = append(adoptIDs, newParents...)
	if errCode := tx.AdoptLiveRelationships(adoptIDs...); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if errCode := tx.ReseedInheritedProperties(objVal.ID()); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinChparents implements chparents(object, parents_list)
// Changes object's parents (multiple inheritance)
func builtinChparents(ctx *Execution, args []types.Value) types.Result {
	if errCode := flushStagedBeforeCoarse(ctx); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	store := ctx.Store

	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	parentsList := args[1]
	if parentsList.Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}

	if !validForRead(ctx, objVal.ID()) {
		return types.Err(types.E_INVIND)
	}

	// Convert list to ObjIDs - check cycles and duplicates BEFORE validation
	elements := parentsList.Elements()
	newParents := make([]types.ObjID, len(elements))
	seenParents := make(map[types.ObjID]bool)

	for i, elem := range elements {
		if !isObjectRef(elem) {
			return types.Err(types.E_TYPE)
		}

		parentID := elem.ID()

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
		if !validForRead(ctx, parentID) {
			return types.Err(types.E_INVARG)
		}

		// Check if parent is a descendant of object (would create cycle)
		if store.HasDescendant(objVal.ID(), parentID) {
			return types.Err(types.E_RECMOVE)
		}

		newParents[i] = parentID
	}

	tx := readTxn(ctx)
	duplicateProps, errCode := tx.HasDuplicateDefinedPropertyAmong(newParents)
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
		props, errCode := tx.DefinedPropertyNamesInAncestry(parentID)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		for name := range props {
			allNewParentProps[name] = true
		}
	}

	conflict, errCode := tx.HasDefinedPropertyConflictWithAncestry(objVal.ID(), newParents)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if conflict {
		return types.Err(types.E_INVARG)
	}

	// Check for property conflicts: only chparent-added descendants of obj
	// cannot define properties that are also defined on new parents or their ancestors.
	conflict, errCode = tx.HasChparentDescendantPropertyConflict(objVal.ID(), allNewParentProps)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if conflict {
		return types.Err(types.E_INVARG)
	}

	// TODO: Check permissions and fertile flags (Layer 8.5)

	// Note: ToastStunt does NOT invalidate anonymous descendants when the parent
	// hierarchy changes; they remain valid.

	oldParents, errCode := tx.Parents(objVal.ID())
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if errCode := store.ChangeParents(objVal.ID(), newParents); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	markLiveStoreMutated(ctx)
	adoptIDs := append([]types.ObjID{objVal.ID()}, oldParents...)
	adoptIDs = append(adoptIDs, newParents...)
	if errCode := tx.AdoptLiveRelationships(adoptIDs...); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if errCode := tx.ReseedInheritedProperties(objVal.ID()); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewInt(0))
}

// builtinAncestors implements ancestors(object [, include_self])
// Returns list of all ancestors in inheritance order
func builtinAncestors(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	includeSelf := false
	if len(args) == 2 {
		includeSelf = args[1].Truthy()
	}

	ancestorIDs, errCode := readTxn(ctx).Ancestors(objVal.ID(), includeSelf)
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewList(objIDsToValues(ancestorIDs)))
}

// builtinDescendants implements descendants(object [, include_self])
// Returns list of all descendants in inheritance order
func builtinDescendants(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	includeSelf := false
	if len(args) == 2 {
		includeSelf = args[1].Truthy()
	}

	descendantIDs, errCode := readTxn(ctx).Descendants(objVal.ID(), includeSelf)
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewList(objIDsToValues(descendantIDs)))
}

// builtinIsa implements isa(object, ancestor[, return_object])
// Returns true if object inherits from ancestor, or the matching ancestor object
// when return_object is truthy.
func builtinIsa(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	var ancestors []types.ObjID
	switch args[1].Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		ancestors = append(ancestors, args[1].ID())
	case types.TYPE_LIST:
		for i := 1; i <= args[1].Len(); i++ {
			parentVal := args[1].Get(i)
			if !isObjectRef(parentVal) {
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

	if !validForRead(ctx, objVal.ID()) {
		return noMatch()
	}

	for _, ancestorID := range ancestors {
		if !validForRead(ctx, ancestorID) {
			continue
		}

		hasAncestor := readTxn(ctx).HasAncestor(objVal.ID(), ancestorID)
		if hasAncestor {
			if returnObject {
				return types.Ok(types.NewObj(ancestorID))
			}
			return types.Ok(types.NewInt(1))
		}
	}

	return noMatch()
}

func builtinLocateByName(ctx *Execution, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	needleStr := strings.TrimSpace(args[0].Str())
	if needleStr == "" {
		return types.Ok(types.NewList([]types.Value{}))
	}

	caseSensitive := false
	if len(args) == 2 {
		if args[1].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		caseSensitive = args[1].Int() != 0
	}

	matchingIDs := store.ObjectIDsByNameSubstring(needleStr, caseSensitive)
	matches := make([]types.Value, 0, len(matchingIDs))
	for _, id := range matchingIDs {
		matches = append(matches, types.NewObj(id))
	}
	return types.Ok(types.NewList(matches))
}

func builtinLocations(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}
	if !validForRead(ctx, objVal.ID()) {
		return types.Err(types.E_INVIND)
	}

	var (
		baseID      types.ObjID
		hasBase     bool
		checkParent bool
	)
	if len(args) >= 2 {
		if !isObjectRef(args[1]) {
			return types.Err(types.E_TYPE)
		}
		baseID = args[1].ID()
		hasBase = true
	}
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		checkParent = args[2].Int() != 0
	}

	out := make([]types.Value, 0)
	currentID := objVal.ID()
	for {
		locID, errCode := locationForRead(ctx, currentID)
		if errCode != types.E_NONE || locID == types.ObjNothing {
			break
		}

		if hasBase {
			if !checkParent && locID == baseID {
				break
			}
			if checkParent {
				hasAncestor := readTxn(ctx).HasAncestor(locID, baseID)
				if locID == baseID || hasAncestor {
					break
				}
			}
		}

		out = append(out, types.NewObj(locID))
		currentID = locID
	}

	return types.Ok(types.NewList(out))
}

func builtinOwnedObjects(ctx *Execution, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	owner := args[0]
	if !isObjectRef(owner) {
		return types.Err(types.E_TYPE)
	}
	if !validForRead(ctx, owner.ID()) {
		return types.Err(types.E_INVIND)
	}
	// Scans committed live state; flush staged decentralized creates so an object this task
	// just created is attributed to its owner. Rare introspection builtin, not a hot path.
	if errCode := flushStagedBeforeCoarse(ctx); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	ownedIDs := store.ObjectsOwnedBy(owner.ID())
	out := make([]types.Value, 0, len(ownedIDs))
	for _, id := range ownedIDs {
		out = append(out, types.NewObj(id))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID() < out[j].ID()
	})
	return types.Ok(types.NewList(out))
}

func builtinRecycledObjects(ctx *Execution, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	// This scans committed live state; flush any staged decentralized recycle/create so a
	// recycle this task just performed is reflected. Rare introspection builtin — the flush
	// (which makes the task coarse) is not on any hot path.
	if errCode := flushStagedBeforeCoarse(ctx); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	out := make([]types.Value, 0)
	upper := store.NextID()
	for id := types.ObjID(0); id < upper; id++ {
		if store.DirectTxn().IsRecycled(id) {
			out = append(out, types.NewObj(id))
		}
	}
	return types.Ok(types.NewList(out))
}

func builtinNextRecycledObject(ctx *Execution, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	start := types.ObjID(-1)
	if len(args) == 1 {
		switch args[0].Type() {
		case types.TYPE_OBJ, types.TYPE_ANON:
			start = args[0].ID()
		case types.TYPE_INT:
			start = types.ObjID(args[0].Int())
		default:
			return types.Err(types.E_TYPE)
		}
		if start == types.ObjNothing {
			return types.Err(types.E_INVARG)
		}
		if start > store.DirectTxn().MaxObject() {
			return types.Err(types.E_INVARG)
		}
	}

	// Scans committed live state; flush any staged decentralized recycle first. Rare.
	if errCode := flushStagedBeforeCoarse(ctx); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	upper := store.NextID()
	scanStart := start + 1
	if len(args) == 1 {
		scanStart = start
	}
	for id := scanStart; id < upper; id++ {
		if store.DirectTxn().IsRecycled(id) {
			return types.Ok(types.NewObj(id))
		}
	}
	return types.Ok(types.NewInt(0))
}

func builtinRecreate(ctx *Execution, args []types.Value) types.Result {
	if errCode := flushStagedBeforeCoarse(ctx); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	store := ctx.Store
	session := ctx.Session
	if session == nil {
		return types.Err(types.E_INVARG)
	}

	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	obj := args[0]
	if !isObjectRef(obj) {
		return types.Err(types.E_TYPE)
	}
	parent := types.ObjNothing
	owner := ctx.Programmer
	if len(args) >= 2 {
		if !isObjectRef(args[1]) {
			return types.Err(types.E_TYPE)
		}
		parent = args[1].ID()
	}
	if len(args) == 3 {
		if !isObjectRef(args[2]) {
			return types.Err(types.E_TYPE)
		}
		owner = args[2].ID()
	}
	if err := store.Recreate(obj.ID(), parent, owner); err != nil {
		return types.Err(types.E_INVARG)
	}
	tx := readTxn(ctx)
	if errCode := tx.AdoptLiveObject(obj.ID()); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	adoptIDs := []types.ObjID{obj.ID()}
	if parent != types.ObjNothing {
		adoptIDs = append(adoptIDs, parent)
	}
	if errCode := tx.AdoptLiveRelationships(adoptIDs...); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if errCode := tx.ReseedInheritedProperties(obj.ID()); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	result := types.Ok(types.NewObj(obj.ID()))
	if !validForRead(ctx, obj.ID()) {
		return result
	}

	initResult := session.CallVerb(obj.ID(), "initialize", []types.Value{}, ctx)
	if initResult.Flow == types.FlowException && initResult.Error != types.E_VERBNF {
		return initResult
	}
	return result
}

func builtinWaifStats(ctx *Execution, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	byClass := store.WaifCountByClass()
	total := 0
	for _, count := range byClass {
		total += count
	}
	result := types.NewMap([][2]types.Value{
		{types.NewStr("total"), types.NewInt(int64(total))},
		// Barn has no deferred WAIF-recycling queue: unreachable WAIFs are
		// removed by a Go cleanup, so none remain pending after removal.
		{types.NewStr("pending_recycle"), types.NewInt(0)},
	})
	for classID, count := range byClass {
		result = result.MapSet(types.NewObj(classID), types.NewInt(int64(count)))
	}
	return types.Ok(result)
}
