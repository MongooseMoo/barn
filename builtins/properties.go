package builtins

import (
	"barn/db"
	"barn/types"
	"strings"
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

	obj := store.Get(objVal.ID())
	if obj == nil {
		return types.Err(types.E_INVIND)
	}

	// TODO: Check read permission (currently allows all)

	// Return list of property names
	names := make([]types.Value, 0, len(obj.Properties))
	for name := range obj.Properties {
		names = append(names, types.NewStr(name))
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

	// Find property (with inheritance)
	prop, err := findPropertyInChain(objVal.ID(), nameVal.String()[1:len(nameVal.String())-1], store)
	if err != types.E_NONE {
		return types.Err(err)
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
// info can be {owner, perms} or just perms string
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

	obj := store.Get(objVal.ID())
	if obj == nil {
		return types.Err(types.E_INVIND)
	}

	propName := nameVal.String()[1 : len(nameVal.String())-1] // Strip quotes
	prop, ok := obj.Properties[propName]
	if !ok {
		return types.Err(types.E_PROPNF)
	}

	// TODO: Check permissions (owner or wizard)

	// Parse info argument
	switch info := args[2].(type) {
	case types.StrValue:
		// Just permissions string
		perms := parsePerms(info.String()[1 : len(info.String())-1])
		prop.Perms = perms

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
		prop.Perms = parsePerms(permsVal.String()[1 : len(permsVal.String())-1])

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

	obj := store.Get(objVal.ID())
	if obj == nil {
		return types.Err(types.E_INVIND)
	}

	propName := nameVal.String()[1 : len(nameVal.String())-1] // Strip quotes

	// Check if property already exists
	if _, exists := obj.Properties[propName]; exists {
		return types.Err(types.E_INVARG)
	}

	// TODO: Check permissions (owner or wizard)

	// Parse info argument (same as set_property_info)
	var owner types.ObjID
	var perms db.PropertyPerms

	switch info := args[3].(type) {
	case types.StrValue:
		// Just permissions string
		owner = ctx.Programmer // Default to caller
		perms = parsePerms(info.String()[1 : len(info.String())-1])

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
		perms = parsePerms(permsVal.String()[1 : len(permsVal.String())-1])

	default:
		return types.Err(types.E_TYPE)
	}

	// Create property
	obj.Properties[propName] = &db.Property{
		Name:  propName,
		Value: value,
		Owner: owner,
		Perms: perms,
		Clear: false,
	}

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

	obj := store.Get(objVal.ID())
	if obj == nil {
		return types.Err(types.E_INVIND)
	}

	propName := nameVal.String()[1 : len(nameVal.String())-1] // Strip quotes

	// Check if property exists on this object
	if _, exists := obj.Properties[propName]; !exists {
		return types.Err(types.E_PROPNF)
	}

	// TODO: Check permissions (owner or wizard)

	// Delete property
	delete(obj.Properties, propName)

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

	obj := store.Get(objVal.ID())
	if obj == nil {
		return types.Err(types.E_INVIND)
	}

	propName := nameVal.String()[1 : len(nameVal.String())-1] // Strip quotes

	// Check if property exists (anywhere in chain)
	_, err := findPropertyInChain(objVal.ID(), propName, store)
	if err != types.E_NONE {
		return types.Err(err)
	}

	// TODO: Check permissions (owner or wizard)

	// Get or create property entry
	prop, exists := obj.Properties[propName]
	if !exists {
		// Create a clear property
		obj.Properties[propName] = &db.Property{
			Name:  propName,
			Value: nil,
			Owner: ctx.Programmer,
			Perms: db.PropRead | db.PropWrite,
			Clear: true,
		}
	} else {
		// Clear existing property
		prop.Clear = true
		prop.Value = nil
	}

	return types.Ok(types.NewInt(0))
}

// builtinIsClearProperty implements is_clear_property(object, name)
// Tests if property is cleared (inheriting)
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

	obj := store.Get(objVal.ID())
	if obj == nil {
		return types.Err(types.E_INVIND)
	}

	propName := nameVal.String()[1 : len(nameVal.String())-1] // Strip quotes

	// Check if property exists
	prop, exists := obj.Properties[propName]
	if !exists {
		return types.Err(types.E_PROPNF)
	}

	return types.Ok(types.NewBool(prop.Clear))
}

// Helper functions

// parsePerms converts a permission string like "rw" to PropertyPerms flags
func parsePerms(s string) db.PropertyPerms {
	var perms db.PropertyPerms
	if strings.Contains(s, "r") {
		perms |= db.PropRead
	}
	if strings.Contains(s, "w") {
		perms |= db.PropWrite
	}
	if strings.Contains(s, "c") {
		perms |= db.PropChown
	}
	return perms
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
