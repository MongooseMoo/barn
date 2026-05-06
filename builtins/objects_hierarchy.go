package builtins

import (
	"barn/db"
	"barn/types"
)

// builtinParent implements parent(object)
// Returns the first parent of an object
func builtinParent(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
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

	obj := store.Get(objVal.ID())
	if obj == nil {
		// Check if recycled (E_INVARG) vs never existed (E_INVIND)
		if store.IsRecycled(objVal.ID()) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	if len(obj.Parents) == 0 {
		return types.Ok(types.NewObj(types.ObjNothing))
	}

	return types.Ok(types.NewObj(obj.Parents[0]))
}

// builtinParents implements parents(object)
// Returns list of all direct parents
// Waifs have no parents (E_INVARG)
func builtinParents(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
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

	obj := store.Get(objVal.ID())
	if obj == nil {
		// Check if recycled (E_INVARG) vs never existed (E_INVIND)
		if store.IsRecycled(objVal.ID()) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	// Convert []ObjID to []Value
	parents := make([]types.Value, len(obj.Parents))
	for i, parentID := range obj.Parents {
		parents[i] = types.NewObj(parentID)
	}

	return types.Ok(types.NewList(parents))
}

// builtinChildren implements children(object)
// Returns list of direct children
// Waifs have no children (E_INVARG)
func builtinChildren(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
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

	obj := store.Get(objVal.ID())
	if obj == nil {
		// Check if recycled (E_INVARG) vs never existed (E_INVIND)
		if store.IsRecycled(objVal.ID()) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	// Convert []ObjID to []Value
	children := make([]types.Value, len(obj.Children))
	for i, childID := range obj.Children {
		children[i] = types.NewObj(childID)
	}

	return types.Ok(types.NewList(children))
}

// builtinChparent implements chparent(object, new_parent)
// Changes object's parent (single inheritance)
func builtinChparent(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
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

	newParentVal, ok := args[1].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Validate 3rd arg type if present (must be a list of parents)
	if len(args) == 3 {
		if _, ok := args[2].(types.ListValue); !ok {
			return types.Err(types.E_TYPE)
		}
	}

	// Check for invalid object references
	if objVal.ID() < 0 {
		return types.Err(types.E_INVARG)
	}

	obj := store.Get(objVal.ID())
	if obj == nil {
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

	var newParent *db.Object
	if newParentVal.ID() != types.ObjNothing {
		newParent = store.Get(newParentVal.ID())
		if newParent == nil {
			return types.Err(types.E_INVARG)
		}
	}

	// Check if new parent is a descendant of object (would create cycle)
	if newParentVal.ID() != types.ObjNothing && isChildOf(store, newParentVal.ID(), objVal.ID()) {
		return types.Err(types.E_RECMOVE)
	}

	// Check for direct property conflicts between obj and new parent
	// If obj defines a property that new_parent or its ancestors also define, that's E_INVARG
	// (This is different from inherited properties, which can be shadowed)
	if newParentVal.ID() != types.ObjNothing {
		newParentProps := collectAncestorProperties(store, newParentVal.ID())

		// Check properties DEFINED on obj (Defined=true)
		for name, prop := range obj.Properties {
			if prop.Defined && newParentProps[name] {
				return types.Err(types.E_INVARG)
			}
		}
	}

	// Check for property conflicts: only chparent-added descendants of obj
	// cannot define properties that are also defined on new_parent or its ancestors.
	if newParentVal.ID() != types.ObjNothing {
		newParentProps := collectAncestorProperties(store, newParentVal.ID())
		if hasChparentDescendantConflict(store, obj, newParentProps) {
			return types.Err(types.E_INVARG)
		}
	}

	// TODO: Check permissions and fertile flag (Layer 8.5)

	// Invalidate anonymous children in descendant hierarchy.
	store.InvalidateAnonymousChildren(objVal.ID())

	// Remove from old parents' children lists and ChparentChildren tracking
	for _, oldParentID := range obj.Parents {
		oldParent := store.Get(oldParentID)
		if oldParent != nil {
			oldParent.Children = removeObjID(oldParent.Children, objVal.ID())
			// Remove from ChparentChildren tracking
			if oldParent.ChparentChildren != nil {
				delete(oldParent.ChparentChildren, objVal.ID())
			}
		}
	}

	// Set new parent(s)
	if newParentVal.ID() == types.ObjNothing {
		obj.Parents = []types.ObjID{}
	} else {
		obj.Parents = []types.ObjID{newParentVal.ID()}
		// Add to new parent's children
		newParent.Children = append(newParent.Children, objVal.ID())
		// Track that this child was added via chparent (not create)
		if newParent.ChparentChildren == nil {
			newParent.ChparentChildren = make(map[types.ObjID]bool)
		}
		newParent.ChparentChildren[objVal.ID()] = true
	}

	// Reset inherited property overrides when parent changes
	// Properties that are propdefs (Defined=true) are kept
	// Properties that are local overrides (Defined=false) are removed and re-inherited
	resetInheritedProperties(obj)
	// Re-inherit properties from new parent chain
	newProps := copyInheritedProperties(obj, store)
	// Merge with existing defined properties
	for name, prop := range obj.Properties {
		if prop.Defined {
			newProps[name] = prop
		}
	}
	obj.Properties = newProps

	return types.Ok(types.NewInt(0))
}

// builtinChparents implements chparents(object, parents_list)
// Changes object's parents (multiple inheritance)
func builtinChparents(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
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

	obj := store.Get(objVal.ID())
	if obj == nil {
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
		parent := store.Get(parentID)
		if parent == nil {
			return types.Err(types.E_INVARG)
		}

		// Check if parent is a descendant of object (would create cycle)
		if isChildOf(store, parentID, objVal.ID()) {
			return types.Err(types.E_RECMOVE)
		}

		newParents[i] = parentID
	}

	// Check for duplicate property definitions among new parents
	// Each parent's defined properties must not conflict with any other parent
	allPropNames := make(map[string]bool)
	for _, parentID := range newParents {
		parent := store.Get(parentID)
		if parent == nil {
			continue
		}
		// Get properties DEFINED on this parent (Defined=true)
		for name, prop := range parent.Properties {
			if prop.Defined {
				if allPropNames[name] {
					return types.Err(types.E_INVARG)
				}
				allPropNames[name] = true
			}
		}
	}

	// Check for direct property conflicts between obj and new parents
	// If obj defines a property that any new parent or their ancestors also define, that's E_INVARG
	allNewParentProps := make(map[string]bool)
	for _, parentID := range newParents {
		props := collectAncestorProperties(store, parentID)
		for name := range props {
			allNewParentProps[name] = true
		}
	}

	// Check properties DEFINED on obj (Defined=true)
	for name, prop := range obj.Properties {
		if prop.Defined && allNewParentProps[name] {
			return types.Err(types.E_INVARG)
		}
	}

	// Check for property conflicts: only chparent-added descendants of obj
	// cannot define properties that are also defined on new parents or their ancestors.
	if hasChparentDescendantConflict(store, obj, allNewParentProps) {
		return types.Err(types.E_INVARG)
	}

	// TODO: Check permissions and fertile flags (Layer 8.5)

	// Invalidate anonymous children in descendant hierarchy.
	store.InvalidateAnonymousChildren(objVal.ID())

	// Remove from old parents' children lists and ChparentChildren tracking
	for _, oldParentID := range obj.Parents {
		oldParent := store.Get(oldParentID)
		if oldParent != nil {
			oldParent.Children = removeObjID(oldParent.Children, objVal.ID())
			// Remove from ChparentChildren tracking
			if oldParent.ChparentChildren != nil {
				delete(oldParent.ChparentChildren, objVal.ID())
			}
		}
	}

	// Set new parents
	obj.Parents = newParents

	// Add to new parents' children lists and track as chparent-added
	for _, newParentID := range newParents {
		newParent := store.Get(newParentID)
		if newParent != nil {
			newParent.Children = append(newParent.Children, objVal.ID())
			// Track that this child was added via chparent (not create)
			if newParent.ChparentChildren == nil {
				newParent.ChparentChildren = make(map[types.ObjID]bool)
			}
			newParent.ChparentChildren[objVal.ID()] = true
		}
	}

	// Reset inherited property overrides when parents change
	// Properties that are propdefs (Defined=true) are kept
	// Properties that are local overrides (Defined=false) are removed and re-inherited
	resetInheritedProperties(obj)
	// Re-inherit properties from new parent chain
	newProps := copyInheritedProperties(obj, store)
	// Merge with existing defined properties
	for name, prop := range obj.Properties {
		if prop.Defined {
			newProps[name] = prop
		}
	}
	obj.Properties = newProps

	return types.Ok(types.NewInt(0))
}

// builtinAncestors implements ancestors(object [, include_self])
// Returns list of all ancestors in inheritance order
func builtinAncestors(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
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

	obj := store.Get(objVal.ID())
	if obj == nil {
		return types.Err(types.E_INVARG)
	}

	includeSelf := false
	if len(args) == 2 {
		includeSelf = args[1].Truthy()
	}

	// Collect ancestors in BFS order, maintaining insertion order
	var result []types.Value
	seen := make(map[types.ObjID]bool)
	queue := make([]types.ObjID, 0)

	// Optionally include self first
	if includeSelf {
		result = append(result, types.NewObj(objVal.ID()))
		seen[objVal.ID()] = true
	}

	// Start with direct parents
	queue = append(queue, obj.Parents...)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if seen[currentID] {
			continue
		}
		seen[currentID] = true

		result = append(result, types.NewObj(currentID))

		// Add this ancestor's parents
		current := store.Get(currentID)
		if current != nil {
			queue = append(queue, current.Parents...)
		}
	}

	return types.Ok(types.NewList(result))
}

// builtinDescendants implements descendants(object [, include_self])
// Returns list of all descendants in inheritance order
func builtinDescendants(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
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

	obj := store.Get(objVal.ID())
	if obj == nil {
		return types.Err(types.E_INVARG)
	}

	includeSelf := false
	if len(args) == 2 {
		includeSelf = args[1].Truthy()
	}

	// Collect descendants in BFS order
	var result []types.Value
	seen := make(map[types.ObjID]bool)
	queue := make([]types.ObjID, 0)

	// Optionally include self first
	if includeSelf {
		result = append(result, types.NewObj(objVal.ID()))
		seen[objVal.ID()] = true
	}

	// Start with direct children
	queue = append(queue, obj.Children...)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if seen[currentID] {
			continue
		}
		seen[currentID] = true

		result = append(result, types.NewObj(currentID))

		// Add this descendant's children
		current := store.Get(currentID)
		if current != nil {
			queue = append(queue, current.Children...)
		}
	}

	return types.Ok(types.NewList(result))
}

// builtinIsa implements isa(object, ancestor[, return_object])
// Returns true if object inherits from ancestor, or the matching ancestor object
// when return_object is truthy.
func builtinIsa(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
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

	obj := store.Get(objVal.ID())
	if obj == nil {
		return noMatch()
	}

	for _, ancestorID := range ancestors {
		if !store.Valid(ancestorID) {
			continue
		}

		// Object is always its own ancestor.
		if objVal.ID() == ancestorID {
			if returnObject {
				return types.Ok(types.NewObj(ancestorID))
			}
			return types.Ok(types.NewInt(1))
		}

		// BFS through ancestry chain.
		seen := make(map[types.ObjID]bool)
		queue := obj.Parents[:]

		for len(queue) > 0 {
			currentID := queue[0]
			queue = queue[1:]

			if seen[currentID] {
				continue
			}
			seen[currentID] = true

			if currentID == ancestorID {
				if returnObject {
					return types.Ok(types.NewObj(ancestorID))
				}
				return types.Ok(types.NewInt(1))
			}

			current := store.Get(currentID)
			if current != nil {
				queue = append(queue, current.Parents...)
			}
		}
	}

	return noMatch()
}

// removeObjID removes an ObjID from a slice
func removeObjID(slice []types.ObjID, id types.ObjID) []types.ObjID {
	result := make([]types.ObjID, 0, len(slice))
	for _, item := range slice {
		if item != id {
			result = append(result, item)
		}
	}
	return result
}

// insertObjIDAtMOOPosition inserts id using 1-based MOO positions; 0 appends.
func insertObjIDAtMOOPosition(slice []types.ObjID, id types.ObjID, position int64) []types.ObjID {
	if position == 0 || position > int64(len(slice)+1) {
		return append(slice, id)
	}

	index := int(position - 1)
	result := make([]types.ObjID, len(slice)+1)
	copy(result[:index], slice[:index])
	result[index] = id
	copy(result[index+1:], slice[index:])
	return result
}

// isChildOf checks if descendant is in the children tree of ancestor
// Used for cycle detection in parent relationships
func isChildOf(store *db.Store, descendant, ancestor types.ObjID) bool {
	obj := store.Get(ancestor)
	if obj == nil {
		return false
	}

	// Check direct children
	for _, childID := range obj.Children {
		if childID == descendant {
			return true
		}
		// Recursively check children's children
		if isChildOf(store, descendant, childID) {
			return true
		}
	}

	return false
}

// collectAncestorProperties collects all defined property names from an object
// and its entire ancestor chain (BFS traversal)
func collectAncestorProperties(store *db.Store, objID types.ObjID) map[string]bool {
	props := make(map[string]bool)
	visited := make(map[types.ObjID]bool)
	queue := []types.ObjID{objID}

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] || currentID == types.ObjNothing {
			continue
		}
		visited[currentID] = true

		current := store.Get(currentID)
		if current == nil {
			continue
		}

		// Collect defined properties
		for name, prop := range current.Properties {
			if prop.Defined {
				props[name] = true
			}
		}

		// Add parents to queue
		queue = append(queue, current.Parents...)
	}

	return props
}

// hasChparentDescendantConflict checks if any chparent-added descendant has
// a defined property that conflicts with the given property set.
// ONLY checks descendants that were added via chparent(), not via create().
// The object being reparented itself is NOT checked - it can shadow parent properties.
func hasChparentDescendantConflict(store *db.Store, obj *db.Object, ancestorProps map[string]bool) bool {
	visited := make(map[types.ObjID]bool)

	var checkChparentDescendants func(o *db.Object) bool
	checkChparentDescendants = func(o *db.Object) bool {
		if o == nil || visited[o.ID] {
			return false
		}
		visited[o.ID] = true

		// Check only chparent-added children of this object
		if o.ChparentChildren == nil {
			return false
		}

		for childID := range o.ChparentChildren {
			child := store.Get(childID)
			if child == nil {
				continue
			}

			// Check this chparent-added child's defined properties for conflicts
			for name, prop := range child.Properties {
				if prop.Defined && ancestorProps[name] {
					return true // Conflict found
				}
			}

			// Recursively check this child's chparent-added descendants
			if checkChparentDescendants(child) {
				return true
			}
		}

		return false
	}

	return checkChparentDescendants(obj)
}

// resetInheritedProperties clears non-defined properties when parent changes
// Properties added via add_property (Defined=true) are kept
// Properties that are local overrides (Defined=false) are cleared
func resetInheritedProperties(obj *db.Object) {
	toDelete := []string{}
	for name, prop := range obj.Properties {
		if !prop.Defined {
			toDelete = append(toDelete, name)
		}
	}
	for _, name := range toDelete {
		delete(obj.Properties, name)
	}
}

// isDescendant checks if target is a descendant of ancestor
func isDescendant(ancestor, target types.ObjID, store *db.Store) bool {
	if ancestor == target {
		return true
	}

	// Breadth-first search through location chain
	queue := []types.ObjID{ancestor}
	visited := make(map[types.ObjID]bool)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		if currentID == target {
			return true
		}

		current := store.Get(currentID)
		if current != nil {
			// Add all contents to queue
			queue = append(queue, current.Contents...)
		}
	}

	return false
}
