package builtins

import (
	"barn/db"
	"barn/types"
)

// RegisterObjectBuiltins registers object management builtins
func (r *Registry) RegisterObjectBuiltins(store *db.Store) {
	// Object creation and lifecycle
	r.Register("create", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinCreate(ctx, args, store)
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
}

// builtinCreate implements create(parent [, owner [, anonymous]])
// Creates a new object with the given parent(s)
func builtinCreate(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) < 1 {
		return types.Err(types.E_ARGS)
	}

	// Get parent(s) - can be single object or list
	var parents []types.ObjID
	switch p := args[0].(type) {
	case types.ObjValue:
		parents = []types.ObjID{p.ID()}
	case types.ListValue:
		// Multiple parents
		elements := p.Elements()
		parents = make([]types.ObjID, len(elements))
		for i, elem := range elements {
			objVal, ok := elem.(types.ObjValue)
			if !ok {
				return types.Err(types.E_TYPE)
			}
			parents[i] = objVal.ID()
		}
	default:
		return types.Err(types.E_TYPE)
	}

	// Check that all parents exist and are fertile
	// Filter out $nothing (-1) which means "no parent"
	validParents := []types.ObjID{}
	for _, parentID := range parents {
		if parentID == types.ObjNothing {
			// $nothing is a valid "no parent" value - skip it
			continue
		}
		parent := store.Get(parentID)
		if parent == nil {
			return types.Err(types.E_INVARG)
		}
		// TODO: Check fertile flag and permissions (Layer 8.5)
		validParents = append(validParents, parentID)
	}
	parents = validParents

	// Get owner (default: caller)
	owner := ctx.Programmer
	if len(args) >= 2 {
		ownerVal, ok := args[1].(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		owner = ownerVal.ID()
	}

	// Get anonymous flag (default: false)
	anonymous := false
	if len(args) >= 3 {
		anonVal, ok := args[2].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		anonymous = anonVal.Val != 0
	}

	// Allocate new object ID
	newID := store.NextID()

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

	// Add to parents' children lists
	for _, parentID := range parents {
		parent := store.Get(parentID)
		if parent != nil {
			parent.Children = append(parent.Children, newID)
		}
	}

	// TODO: Call :initialize verb if it exists (Phase 9)

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

	// Move to $nothing (clear location)
	obj.Location = types.ObjNothing
	obj.Contents = []types.ObjID{}

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
func builtinValid(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	isValid := store.Valid(objVal.ID())
	return types.Ok(types.NewBool(isValid))
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

	obj := store.Get(objVal.ID())
	if obj == nil {
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

	obj := store.Get(objVal.ID())
	if obj == nil {
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

	obj := store.Get(objVal.ID())
	if obj == nil {
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

	obj := store.Get(objVal.ID())
	if obj == nil {
		return types.Err(types.E_INVIND)
	}

	newParent := store.Get(newParentVal.ID())
	if newParent == nil {
		return types.Err(types.E_INVARG)
	}

	// TODO: Check permissions and fertile flag (Layer 8.5)
	// TODO: Check for cycles

	// Remove from old parents' children lists
	for _, oldParentID := range obj.Parents {
		oldParent := store.Get(oldParentID)
		if oldParent != nil {
			oldParent.Children = removeObjID(oldParent.Children, objVal.ID())
		}
	}

	// Set new parent
	obj.Parents = []types.ObjID{newParentVal.ID()}

	// Add to new parent's children
	newParent.Children = append(newParent.Children, objVal.ID())

	return types.Ok(types.NewInt(0))
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

	// Convert list to ObjIDs
	elements := parentsList.Elements()
	newParents := make([]types.ObjID, len(elements))
	for i, elem := range elements {
		parentVal, ok := elem.(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}

		parent := store.Get(parentVal.ID())
		if parent == nil {
			return types.Err(types.E_INVARG)
		}

		newParents[i] = parentVal.ID()
	}

	// TODO: Check permissions and fertile flags (Layer 8.5)
	// TODO: Check for cycles

	// Remove from old parents' children lists
	for _, oldParentID := range obj.Parents {
		oldParent := store.Get(oldParentID)
		if oldParent != nil {
			oldParent.Children = removeObjID(oldParent.Children, objVal.ID())
		}
	}

	// Set new parents
	obj.Parents = newParents

	// Add to new parents' children lists
	for _, newParentID := range newParents {
		newParent := store.Get(newParentID)
		if newParent != nil {
			newParent.Children = append(newParent.Children, objVal.ID())
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
		return types.Err(types.E_INVIND)
	}

	// Check if ancestor is valid
	if !store.Valid(ancestorVal.ID()) {
		return types.Err(types.E_INVARG)
	}

	// Object is always its own ancestor
	if objVal.ID() == ancestorVal.ID() {
		return types.Ok(types.NewBool(true))
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
			return types.Ok(types.NewBool(true))
		}

		current := store.Get(currentID)
		if current != nil {
			queue = append(queue, current.Parents...)
		}
	}

	return types.Ok(types.NewBool(false))
}

// Helper functions

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
