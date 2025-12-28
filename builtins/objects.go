package builtins

import (
	"barn/db"
	"barn/types"
)

// RegisterObjectBuiltins registers object management builtins
func (r *Registry) RegisterObjectBuiltins(store *db.Store) {
	// Object creation and lifecycle
	r.Register("create", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinCreate(ctx, args, store, r)
	})

	r.Register("recycle", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinRecycle(ctx, args, store)
	})

	r.Register("valid", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinValid(ctx, args, store)
	})

	r.Register("max_object", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinMaxObject(ctx, args, store)
	})

	// Inheritance
	r.Register("parent", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinParent(ctx, args, store)
	})

	r.Register("parents", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinParents(ctx, args, store)
	})

	r.Register("children", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinChildren(ctx, args, store)
	})

	r.Register("ancestors", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinAncestors(ctx, args, store)
	})

	r.Register("descendants", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinDescendants(ctx, args, store)
	})

	r.Register("isa", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinIsa(ctx, args, store)
	})

	r.Register("chparent", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinChparent(ctx, args, store)
	})

	r.Register("chparents", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinChparents(ctx, args, store)
	})

	// Location and movement
	r.Register("move", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinMove(ctx, args, store)
	})

	// Player management
	r.Register("is_player", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinIsPlayer(ctx, args, store)
	})

	r.Register("set_player_flag", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinSetPlayerFlag(ctx, args, store)
	})

	r.Register("players", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinPlayers(ctx, args, store)
	})

	r.Register("renumber", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinRenumber(ctx, args, store)
	})
}

// builtinCreate implements create(parent [, owner] [, anonymous] [, args])
// Creates a new object with the given parent(s)
// Per cow_py semantics:
// - First arg: OBJ, negative INT (as object reference), or list of same
// - Optional args (in order):
//   - OBJ or negative INT → owner (must come before anonymous flag)
//   - Non-negative INT → anonymous flag (0 or 1)
//   - LIST → init args for :initialize verb (must come last)
// - Float or Map is always E_TYPE
// - Owner values < -1 (like -2, -3, -4) are E_INVARG
func builtinCreate(ctx *types.TaskContext, args []types.Value, store *db.Store, registry *Registry) types.Result {
	if len(args) < 1 {
		return types.Err(types.E_ARGS)
	}

	// Get parent(s) - OBJ or negative INT, or list of same
	// Positive integers are NOT valid as parent references (E_TYPE)
	var parents []types.ObjID
	parentsFromList := false
	switch p := args[0].(type) {
	case types.ObjValue:
		parents = []types.ObjID{p.ID()}
	case types.IntValue:
		// Only negative integers are valid as object references
		if p.Val >= 0 {
			return types.Err(types.E_TYPE)
		}
		parents = []types.ObjID{types.ObjID(p.Val)}
	case types.ListValue:
		// Multiple parents
		parentsFromList = true
		elements := p.Elements()
		parents = make([]types.ObjID, len(elements))
		for i, elem := range elements {
			switch e := elem.(type) {
			case types.ObjValue:
				parents[i] = e.ID()
			case types.IntValue:
				// Only negative integers are valid as object references
				if e.Val >= 0 {
					return types.Err(types.E_TYPE)
				}
				parents[i] = types.ObjID(e.Val)
			default:
				return types.Err(types.E_TYPE)
			}
		}
	default:
		return types.Err(types.E_TYPE)
	}

	// Validate parents
	// -1 ($nothing) is valid as a solo parent (means no parent)
	// -1 ($nothing) in a list is E_INVARG
	// -2, -3, -4 (special invalid object numbers) are always E_INVARG
	// Other negative IDs and non-existent objects are E_INVARG
	validParents := []types.ObjID{}
	seenParents := make(map[types.ObjID]bool)
	for _, parentID := range parents {
		if parentID < -1 {
			// Special invalid object numbers: -2, -3, -4
			return types.Err(types.E_INVARG)
		}
		if parentID == types.ObjNothing {
			if parentsFromList {
				// $nothing in a parent list is invalid
				return types.Err(types.E_INVARG)
			}
			// $nothing as solo parent means "no parent" - skip it
			continue
		}
		// Check for duplicate parents
		if seenParents[parentID] {
			return types.Err(types.E_INVARG)
		}
		seenParents[parentID] = true
		parent := store.Get(parentID)
		if parent == nil {
			return types.Err(types.E_INVARG)
		}
		// Permission check deferred until after anonymous flag is parsed
		validParents = append(validParents, parentID)
	}
	parents = validParents

	// Check for duplicate property definitions among parents
	// Each parent's defined properties must not conflict with any other parent
	allPropNames := make(map[string]bool)
	for _, parentID := range parents {
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

	// Parse optional arguments
	// Per cow_py semantics:
	// - OBJ or negative INT → owner (must come before anonymous flag, only once)
	// - Non-negative INT → anonymous flag (0 or 1, only once)
	// - LIST → init args (only once, must be last)
	// - Float or Map is always E_TYPE
	owner := ctx.Programmer
	ownerSpecified := false
	anonymous := false
	anonymousSeen := false
	var initArgs []types.Value

	initArgsSeen := false
	for i := 1; i < len(args); i++ {
		switch v := args[i].(type) {
		case types.ObjValue:
			// ObjNum is owner - only valid before anonymous flag and initArgs
			if anonymousSeen {
				return types.Err(types.E_TYPE)
			}
			if ownerSpecified {
				return types.Err(types.E_TYPE)
			}
			if initArgsSeen {
				return types.Err(types.E_TYPE)
			}
			owner = v.ID()
			ownerSpecified = true
		case types.IntValue:
			if v.Val < 0 {
				// Negative int is owner (object reference)
				if anonymousSeen {
					return types.Err(types.E_TYPE)
				}
				if ownerSpecified {
					return types.Err(types.E_TYPE)
				}
				if initArgsSeen {
					return types.Err(types.E_TYPE)
				}
				owner = types.ObjID(v.Val)
				ownerSpecified = true
			} else {
				// Non-negative int is anonymous flag (0 or 1)
				if anonymousSeen {
					return types.Err(types.E_TYPE)
				}
				anonymous = v.Val != 0
				anonymousSeen = true
			}
		case types.ListValue:
			// LIST is initialization arguments (only once)
			if initArgsSeen {
				return types.Err(types.E_TYPE)
			}
			initArgs = v.Elements()
			initArgsSeen = true
		case types.FloatValue:
			// Float is always an error
			return types.Err(types.E_TYPE)
		case types.MapValue:
			// Map is always an error
			return types.Err(types.E_TYPE)
		default:
			return types.Err(types.E_TYPE)
		}
	}

	// Check permissions for creating from each parent
	// - Wizards can create from any object
	// - For anonymous objects: non-wizards need to own parent OR parent has FlagAnonymous
	// - For regular objects: non-wizards need to own parent OR parent has FlagFertile
	playerIsWizard := ctx.IsWizard || isPlayerWizard(store, ctx.Player)
	if !playerIsWizard {
		for _, parentID := range parents {
			parent := store.Get(parentID)
			if parent == nil {
				continue
			}
			isOwner := parent.Owner == ctx.Programmer
			if anonymous {
				hasAnonFlag := parent.Flags.Has(db.FlagAnonymous)
				if !isOwner && !hasAnonFlag {
					return types.Err(types.E_PERM)
				}
			} else {
				hasFertile := parent.Flags.Has(db.FlagFertile)
				if !isOwner && !hasFertile {
					return types.Err(types.E_PERM)
				}
			}
		}
	}

	// Validate owner if explicitly specified
	if ownerSpecified {
		if owner < -1 {
			// Special invalid object numbers: -2, -3, -4
			return types.Err(types.E_INVARG)
		}
		if owner != types.ObjNothing && store.Get(owner) == nil {
			return types.Err(types.E_INVARG)
		}
		// Only wizards can specify $nothing as owner (makes object own itself)
		if owner == types.ObjNothing && !playerIsWizard {
			return types.Err(types.E_PERM)
		}
	}

	// Anonymous objects cannot have $nothing as owner
	if anonymous && owner == types.ObjNothing {
		return types.Err(types.E_INVARG)
	}

	// Allocate new object ID
	newID := store.NextID()

	// If owner is $nothing, the new object owns itself (only for regular objects)
	if owner == types.ObjNothing {
		owner = newID
	}

	// Create object
	obj := db.NewObject(newID, owner)
	obj.Parents = parents
	obj.Anonymous = anonymous
	if anonymous {
		obj.Flags = obj.Flags.Set(db.FlagAnonymous)
	}

	// Copy properties from parent chain
	// Per spec: "All properties from the entire inheritance chain are copied"
	copied := copyInheritedProperties(obj, store)
	obj.Properties = copied

	// Add object to store
	if err := store.Add(obj); err != nil {
		return types.Err(types.E_QUOTA)
	}

	// Add to parents' children lists (only for non-anonymous objects)
	// Anonymous objects do not appear in children() results
	if !anonymous {
		for _, parentID := range parents {
			parent := store.Get(parentID)
			if parent != nil {
				parent.Children = append(parent.Children, newID)
			}
		}
	}

	// Call :initialize verb if it exists
	// The :initialize verb receives the init args and can set up the new object
	// If verb not found (E_VERBNF), that's fine - just means no initialize
	// Other errors should be propagated
	result := registry.CallVerb(newID, "initialize", initArgs, ctx)
	if result.Flow == types.FlowException {
		if result.Error != types.E_VERBNF {
			// Real error - propagate it
			return result
		}
		// E_VERBNF is fine - no initialize verb
	}

	// Return AnonValue for anonymous objects, ObjValue for regular
	if anonymous {
		return types.Ok(types.NewAnon(newID))
	}
	return types.Ok(types.NewObj(newID))
}

// copyInheritedProperties copies properties from parent chain
// Clear properties remain clear (inherit dynamically)
// Non-clear properties are copied as independent values
func copyInheritedProperties(obj *db.Object, store *db.Store) map[string]*db.Property {
	result := make(map[string]*db.Property)
	visited := make(map[types.ObjID]bool)

	// Breadth-first traversal of inheritance chain
	queue := obj.Parents[:]
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		current := store.Get(currentID)
		if current == nil {
			continue
		}

		// Copy properties not already seen
		for name, prop := range current.Properties {
			if _, exists := result[name]; !exists {
				// Copy property
				newProp := &db.Property{
					Name:  prop.Name,
					Value: prop.Value,
					Owner: prop.Owner,
					Perms: prop.Perms,
					Clear: prop.Clear,
				}
				result[name] = newProp
			}
		}

		// Add parents to queue
		queue = append(queue, current.Parents...)
	}

	return result
}

// builtinRecycle implements recycle(object)
// Destroys an object
func builtinRecycle(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		return types.Err(types.E_INVARG)
	}

	// TODO: Check permissions (Layer 8.5)
	// TODO: Call :recycle verb if it exists (Phase 9)

	// Reparent children to this object's parent(s)
	// Per MOO semantics: when an object is recycled, its children
	// are reparented to the recycled object's parent
	objParents := obj.Parents
	for _, childID := range obj.Children {
		child := store.Get(childID)
		if child == nil {
			continue
		}

		// Replace this object with its parents in the child's parent list
		// Avoid duplicates when merging parents
		newChildParents := []types.ObjID{}
		seen := make(map[types.ObjID]bool)
		for _, pid := range child.Parents {
			if pid == objID {
				// Replace with recycled object's parents (avoiding duplicates)
				for _, op := range objParents {
					if !seen[op] {
						seen[op] = true
						newChildParents = append(newChildParents, op)
					}
				}
			} else {
				if !seen[pid] {
					seen[pid] = true
					newChildParents = append(newChildParents, pid)
				}
			}
		}
		child.Parents = newChildParents

		// Add child to new parents' children lists
		for _, newParentID := range objParents {
			newParent := store.Get(newParentID)
			if newParent != nil {
				// Avoid duplicates
				hasChild := false
				for _, cid := range newParent.Children {
					if cid == childID {
						hasChild = true
						break
					}
				}
				if !hasChild {
					newParent.Children = append(newParent.Children, childID)
				}
			}
		}
	}

	// Move contents to $nothing (update their location)
	for _, contentID := range obj.Contents {
		content := store.Get(contentID)
		if content != nil {
			content.Location = types.ObjNothing
		}
	}
	obj.Contents = []types.ObjID{}

	// Remove from old location's contents
	if obj.Location != types.ObjNothing {
		oldLoc := store.Get(obj.Location)
		if oldLoc != nil {
			oldLoc.Contents = removeObjID(oldLoc.Contents, objID)
		}
	}

	// Move to $nothing
	obj.Location = types.ObjNothing

	// Clear properties and verbs
	obj.Properties = make(map[string]*db.Property)
	obj.Verbs = make(map[string]*db.Verb)

	// Remove from parent's children
	for _, parentID := range obj.Parents {
		parent := store.Get(parentID)
		if parent != nil {
			parent.Children = removeObjID(parent.Children, objID)
		}
	}

	// Mark as recycled
	if err := store.Recycle(objID); err != nil {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewInt(0))
}

// builtinValid implements valid(object)
// Tests if an object exists and is not recycled
// Accepts both ObjValue and IntValue (integers are implicitly converted to object IDs)
func builtinValid(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	var objID types.ObjID
	switch v := args[0].(type) {
	case types.ObjValue:
		objID = v.ID()
	case types.IntValue:
		objID = types.ObjID(v.Val)
	default:
		return types.Err(types.E_TYPE)
	}

	isValid := store.Valid(objID)
	if isValid {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// builtinMaxObject implements max_object()
// Returns the highest allocated object ID
func builtinMaxObject(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	maxID := store.MaxObject()
	return types.Ok(types.NewObj(maxID))
}

// builtinParent implements parent(object)
// Returns the first parent of an object
func builtinParent(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
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
func builtinParents(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
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
func builtinChildren(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
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
func builtinChparent(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
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

	// Check for property conflicts: only chparent-added descendants of obj
	// cannot define properties that are also defined on new_parent or its ancestors.
	// The object being reparented itself is NOT checked - it can shadow parent properties.
	if newParentVal.ID() != types.ObjNothing {
		newParentProps := collectAncestorProperties(store, newParentVal.ID())
		if hasChparentDescendantConflict(store, obj, newParentProps) {
			return types.Err(types.E_INVARG)
		}
	}

	// TODO: Check permissions and fertile flag (Layer 8.5)

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
	// Properties that are local overrides (Defined=false) are cleared
	resetInheritedProperties(obj)

	return types.Ok(types.NewInt(1))
}

// builtinChparents implements chparents(object, parents_list)
// Changes object's parents (multiple inheritance)
func builtinChparents(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
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

	// Check for property conflicts: only chparent-added descendants of obj
	// cannot define properties that are also defined on new parents or their ancestors.
	// The object being reparented itself is NOT checked - it can shadow parent properties.
	allNewParentProps := make(map[string]bool)
	for _, parentID := range newParents {
		props := collectAncestorProperties(store, parentID)
		for name := range props {
			allNewParentProps[name] = true
		}
	}
	if hasChparentDescendantConflict(store, obj, allNewParentProps) {
		return types.Err(types.E_INVARG)
	}

	// TODO: Check permissions and fertile flags (Layer 8.5)

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

	return types.Ok(types.NewInt(0))
}

// builtinMove implements move(what, where)
// Moves object to new location
func builtinMove(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	whatVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	whereVal, ok := args[1].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	what := store.Get(whatVal.ID())
	if what == nil {
		return types.Err(types.E_INVIND)
	}

	// Check for recursive move (moving into self or descendant)
	if isDescendant(whatVal.ID(), whereVal.ID(), store) {
		return types.Err(types.E_RECMOVE)
	}

	// Remove from old location's contents
	if what.Location != types.ObjNothing {
		oldLoc := store.Get(what.Location)
		if oldLoc != nil {
			oldLoc.Contents = removeObjID(oldLoc.Contents, whatVal.ID())
		}
	}

	// Set new location
	what.Location = whereVal.ID()

	// Add to new location's contents (if not moving to nothing)
	if whereVal.ID() != types.ObjNothing {
		where := store.Get(whereVal.ID())
		if where != nil {
			where.Contents = append(where.Contents, whatVal.ID())
		}
	}

	// TODO: Call exitfunc and enterfunc verbs (Phase 9)

	return types.Ok(types.NewInt(0))
}

// builtinAncestors implements ancestors(object [, include_self])
// Returns list of all ancestors in inheritance order
func builtinAncestors(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) < 1 || len(args) > 2 {
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
func builtinDescendants(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) < 1 || len(args) > 2 {
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

// builtinIsa implements isa(object, ancestor)
// Returns true if object inherits from ancestor
func builtinIsa(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	ancestorVal, ok := args[1].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	obj := store.Get(objVal.ID())
	if obj == nil {
		// Invalid child - return 0 (false)
		return types.Ok(types.NewInt(0))
	}

	// If ancestor is invalid, return 0 (false) not an error
	if !store.Valid(ancestorVal.ID()) {
		return types.Ok(types.NewInt(0))
	}

	// Object is always its own ancestor
	if objVal.ID() == ancestorVal.ID() {
		return types.Ok(types.NewInt(1))
	}

	// BFS through ancestry chain
	seen := make(map[types.ObjID]bool)
	queue := obj.Parents[:]

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if seen[currentID] {
			continue
		}
		seen[currentID] = true

		if currentID == ancestorVal.ID() {
			return types.Ok(types.NewInt(1))
		}

		current := store.Get(currentID)
		if current != nil {
			queue = append(queue, current.Parents...)
		}
	}

	return types.Ok(types.NewInt(0))
}

// Helper functions

// isPlayerWizard checks if a player object has wizard permissions
func isPlayerWizard(store *db.Store, objID types.ObjID) bool {
	obj := store.Get(objID)
	if obj == nil {
		return false
	}
	return obj.Flags.Has(db.FlagWizard)
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

// builtinPlayers implements players()
// Returns a list of all player objects
func builtinPlayers(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	playerIDs := store.Players()
	result := make([]types.Value, len(playerIDs))
	for i, id := range playerIDs {
		result[i] = types.NewObj(id)
	}

	return types.Ok(types.NewList(result))
}

// builtinIsPlayer implements is_player(object)
// Returns 1 if object is a player, 0 otherwise
func builtinIsPlayer(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 1 {
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

	// Anonymous objects cannot be players - E_TYPE per MOO spec
	if obj.Anonymous {
		return types.Err(types.E_TYPE)
	}

	if obj.Flags.Has(db.FlagUser) {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// builtinSetPlayerFlag implements set_player_flag(object, value)
// Sets or clears the player flag on an object
func builtinSetPlayerFlag(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 2 {
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

	// Anonymous objects cannot have player flag set - E_TYPE per MOO spec
	if obj.Anonymous {
		return types.Err(types.E_TYPE)
	}

	// TODO: Check wizard permissions (Layer 8.5)

	// Set or clear the player flag
	if args[1].Truthy() {
		obj.Flags = obj.Flags.Set(db.FlagUser)
	} else {
		obj.Flags = obj.Flags.Clear(db.FlagUser)
	}

	return types.Ok(types.NewInt(0))
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

// builtinRenumber implements renumber(obj) - wizard only
// Reassigns object to lowest available object ID
// Returns the new object ID
func builtinRenumber(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// TODO: Check caller is wizard
	// if !isWizard(ctx.Programmer) {
	// 	return types.Err(types.E_PERM)
	// }

	// Get object to renumber
	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	oldID := objVal.ID()

	// Check object is valid
	if !store.Valid(oldID) {
		return types.Err(types.E_INVARG)
	}

	// Find lowest available ID
	newID := store.LowestFreeID()

	// If lowest free ID is same or higher, nothing to do
	if newID >= oldID {
		return types.Ok(types.NewObj(oldID))
	}

	// Renumber the object
	err := store.Renumber(oldID, newID)
	if err != nil {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewObj(newID))
}
