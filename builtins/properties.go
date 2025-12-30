package builtins

import (
	"barn/db"
	"barn/types"
)

// RegisterPropertyBuiltins registers property management builtins
func (r *Registry) RegisterPropertyBuiltins(store *db.Store) {
	r.Register("properties", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinProperties(ctx, args, store)
	})

	r.Register("property_info", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinPropertyInfo(ctx, args, store)
	})

	r.Register("set_property_info", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinSetPropertyInfo(ctx, args, store)
	})

	r.Register("add_property", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinAddProperty(ctx, args, store)
	})

	r.Register("delete_property", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinDeleteProperty(ctx, args, store)
	})

	r.Register("clear_property", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinClearProperty(ctx, args, store)
	})

	r.Register("is_clear_property", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return builtinIsClearProperty(ctx, args, store)
	})
}

// builtinProperties implements properties(object)
// Returns list of property names defined on object (not inherited)
func builtinProperties(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
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
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	// TODO: Check read permission (currently allows all)

	// Return list of property names that are DEFINED on this object
	// (not just local value overrides of inherited properties)
	names := make([]types.Value, 0, len(obj.Properties))
	for name, prop := range obj.Properties {
		if prop.Defined {
			names = append(names, types.NewStr(name))
		}
	}

	return types.Ok(types.NewList(names))
}

// builtinPropertyInfo implements property_info(object, name)
// Returns {owner, perms} where perms is a string like "rw"
func builtinPropertyInfo(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	// Find property (with inheritance)
	prop, err := findPropertyInChain(objID, nameVal.Value(), store)
	if err != types.E_NONE {
		return types.Err(err)
	}

	// Check read permission (unless wizard or owner)
	wizObj := store.Get(ctx.Programmer)
	isWizard := wizObj != nil && wizObj.Flags.Has(db.FlagWizard)
	isOwner := ctx.Programmer == prop.Owner
	if !isWizard && !isOwner && !prop.Perms.Has(db.PropRead) {
		return types.Err(types.E_PERM)
	}

	// Build permissions string
	perms := prop.Perms.String()

	// Return {owner, perms}
	result := []types.Value{
		types.NewObj(prop.Owner),
		types.NewStr(perms),
	}

	return types.Ok(types.NewList(result))
}

// builtinSetPropertyInfo implements set_property_info(object, name, info)
// info can be {owner, perms}, just perms string, or just owner ObjValue
func builtinSetPropertyInfo(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	propName := nameVal.Value()
	prop, ok := obj.Properties[propName]
	if !ok {
		return types.Err(types.E_PROPNF)
	}

	// TODO: Check permissions (owner or wizard)

	// Parse info argument
	switch info := args[2].(type) {
	case types.StrValue:
		// Just permissions string
		perms, err := parsePerms(info.Value())
		if err != types.E_NONE {
			return types.Err(err)
		}
		prop.Perms = perms

	case types.ObjValue:
		// Just owner (leave perms unchanged)
		prop.Owner = info.ID()

	case types.ListValue:
		// {owner, perms}
		elements := info.Elements()
		if len(elements) != 2 {
			return types.Err(types.E_INVARG)
		}

		ownerVal, ok := elements[0].(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}

		permsVal, ok := elements[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}

		prop.Owner = ownerVal.ID()
		perms, err := parsePerms(permsVal.Value())
		if err != types.E_NONE {
			return types.Err(err)
		}
		prop.Perms = perms

	default:
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewInt(0))
}

// builtinAddProperty implements add_property(object, name, value, info)
// Adds a new property to object
func builtinAddProperty(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 4 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	value := args[2]

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	propName := nameVal.Value()


	// Check if property name is built-in
	if isBuiltinProperty(propName) {
		return types.Err(types.E_INVARG)
	}

	// Check if property already exists on this object
	if _, exists := obj.Properties[propName]; exists {
		return types.Err(types.E_INVARG)
	}

	// Check if property exists in ancestor chain
	_, ancestorErr := findPropertyInChain(objID, propName, store)
	if ancestorErr == types.E_NONE {
		// Property exists in ancestor
		return types.Err(types.E_INVARG)
	}

	// Check if property exists in any descendant
	if hasPropertyInDescendants(objID, propName, store) {
		return types.Err(types.E_INVARG)
	}

	// Parse info argument (same as set_property_info)
	var owner types.ObjID
	var perms db.PropertyPerms

	switch info := args[3].(type) {
	case types.StrValue:
		// Just permissions string
		owner = ctx.Programmer // Default to caller
		var errCode types.ErrorCode
		perms, errCode = parsePerms(info.Value())
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}

	case types.ListValue:
		// {owner, perms}
		elements := info.Elements()
		if len(elements) != 2 {
			return types.Err(types.E_INVARG)
		}

		ownerVal, ok := elements[0].(types.ObjValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}

		permsVal, ok := elements[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}

		owner = ownerVal.ID()
		var errCode2 types.ErrorCode
		perms, errCode2 = parsePerms(permsVal.Value())
		if errCode2 != types.E_NONE {
			return types.Err(errCode2)
		}

	default:
		return types.Err(types.E_TYPE)
	}

	// Validate owner is a valid object
	ownerObj := store.Get(owner)
	if ownerObj == nil || store.IsRecycled(owner) {
		return types.Err(types.E_INVARG)
	}

	// Check permissions: only wizard can set owner to someone else
	wizObj := store.Get(ctx.Programmer)
	isWizard := wizObj != nil && wizObj.Flags.Has(db.FlagWizard)
	if !isWizard && owner != ctx.Programmer {
		return types.Err(types.E_PERM)
	}

	// Create property (defined on this object via add_property)
	obj.Properties[propName] = &db.Property{
		Name:    propName,
		Value:   value,
		Owner:   owner,
		Perms:   perms,
		Clear:   false,
		Defined: true, // This property is defined on this object
	}

	// Invalidate anonymous children (parent schema changed)
	for _, childID := range obj.AnonymousChildren {
		child := store.Get(childID)
		if child != nil && child.Anonymous {
			child.Flags = child.Flags.Set(db.FlagInvalid)
		}
	}
	obj.AnonymousChildren = nil

	return types.Ok(types.NewInt(0))
}

// builtinDeleteProperty implements delete_property(object, name)
// Removes property from object
func builtinDeleteProperty(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	propName := nameVal.Value()

	// Check if property exists on this object
	if _, exists := obj.Properties[propName]; !exists {
		return types.Err(types.E_PROPNF)
	}

	// TODO: Check permissions (owner or wizard)

	// Delete property
	delete(obj.Properties, propName)

	// Invalidate anonymous children (parent schema changed)
	for _, childID := range obj.AnonymousChildren {
		child := store.Get(childID)
		if child != nil && child.Anonymous {
			child.Flags = child.Flags.Set(db.FlagInvalid)
		}
	}
	obj.AnonymousChildren = nil

	return types.Ok(types.NewInt(0))
}

// builtinClearProperty implements clear_property(object, name)
// Clears property to inherit from parent
func builtinClearProperty(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	propName := nameVal.Value()

	// Check if it's a built-in property - return E_PERM
	if isBuiltinProperty(propName) {
		return types.Err(types.E_PERM)
	}

	// Find property in chain
	foundProp, err := findPropertyInChain(objID, propName, store)
	if err != types.E_NONE {
		return types.Err(err)
	}

	// Check if property is defined on this object - E_INVARG if so
	if prop, exists := obj.Properties[propName]; exists && prop.Defined {
		return types.Err(types.E_INVARG)
	}

	// Check write permission (unless wizard or owner)
	wizObj := store.Get(ctx.Programmer)
	isWizard := wizObj != nil && wizObj.Flags.Has(db.FlagWizard)
	isOwner := ctx.Programmer == foundProp.Owner
	if !isWizard && !isOwner && !foundProp.Perms.Has(db.PropWrite) {
		return types.Err(types.E_PERM)
	}

	// Clear property by removing local entry
	// This causes the property to inherit from parent
	delete(obj.Properties, propName)

	return types.Ok(types.NewInt(0))
}

// builtinIsClearProperty implements is_clear_property(object, name)
// Tests if property is cleared (inheriting)
// Returns 1 if property is clear or only inherited, 0 if has local value
func builtinIsClearProperty(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	nameVal, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	obj := store.Get(objID)
	if obj == nil {
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_INVIND)
	}

	propName := nameVal.Value()

	// Check if it's a built-in property - return 0
	if isBuiltinProperty(propName) {
		return types.Ok(types.NewInt(0))
	}

	// Find where property is defined in chain
	definingProp, err := findPropertyInChain(objID, propName, store)
	if err != types.E_NONE {
		return types.Err(err)
	}

	// Check if property exists directly on this object and determine clear state FIRST
	prop, exists := obj.Properties[propName]
	var isClear bool
	if exists {
		// Property is defined on this object (via add_property)
		if prop.Defined {
			isClear = false
		} else if prop.Clear {
			// Property exists as cleared local value
			isClear = true
		} else {
			// Property exists as local value override
			isClear = false
		}
	} else {
		// Property not on this object - it's inherited (counts as "clear")
		isClear = true
	}

	// NOW check read permission (unless wizard or owner)
	wizObj := store.Get(ctx.Programmer)
	isWizard := wizObj != nil && wizObj.Flags.Has(db.FlagWizard)
	isOwner := ctx.Programmer == definingProp.Owner
	hasReadPerm := definingProp.Perms.Has(db.PropRead)
	if !isWizard && !isOwner && !hasReadPerm {
		return types.Err(types.E_PERM)
	}

	// Return clear state
	if isClear {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// Helper functions

// isBuiltinProperty checks if a property name is a built-in property
// Built-in properties: name, owner, location, contents, parents, parent, children, programmer, wizard, player, r, w, f, a
func isBuiltinProperty(name string) bool {
	switch name {
	case "name", "owner", "location", "contents", "parents", "parent", "children",
		"programmer", "wizard", "player", "r", "w", "f", "a":
		return true
	default:
		return false
	}
}

// parsePerms converts a permission string like "rw" to PropertyPerms flags
// Returns error code if invalid characters found
func parsePerms(s string) (db.PropertyPerms, types.ErrorCode) {
	var perms db.PropertyPerms
	for _, c := range s {
		switch c {
		case 'r', 'R':
			perms |= db.PropRead
		case 'w', 'W':
			perms |= db.PropWrite
		case 'c', 'C':
			perms |= db.PropChown
		default:
			return 0, types.E_INVARG
		}
	}
	return perms, types.E_NONE
}

// findPropertyInChain finds a property anywhere in the inheritance chain
// Returns the property and E_NONE if found, or E_PROPNF if not found
func findPropertyInChain(objID types.ObjID, name string, store *db.Store) (*db.Property, types.ErrorCode) {
	// Breadth-first search
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)

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

		// Check if property exists on this object
		if prop, ok := current.Properties[name]; ok {
			return prop, types.E_NONE
		}

		// Add parents to queue
		queue = append(queue, current.Parents...)
	}

	return nil, types.E_PROPNF
}

// hasPropertyInDescendants checks if any descendant has a defined property with the given name
// Returns true if found, false otherwise
func hasPropertyInDescendants(objID types.ObjID, name string, store *db.Store) bool {
	// Breadth-first search through descendants
	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)

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

		// Check children for the property
		for _, childID := range current.Children {
			child := store.Get(childID)
			if child == nil {
				continue
			}

			// Check if property is defined on this child
			if prop, ok := child.Properties[name]; ok && prop.Defined {
				return true
			}

			// Add child to queue to check its descendants
			queue = append(queue, childID)
		}
	}

	return false
}
