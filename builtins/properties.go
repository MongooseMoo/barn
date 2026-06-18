package builtins

import (
	"barn/db"
	"barn/types"
)

// builtinProperties implements properties(object)
// Returns list of property names defined on object (not inherited)
func builtinProperties(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if _, ok := args[0].(types.WaifValue); ok {
		return types.Err(types.E_INVARG)
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

	names, err := store.DefinedPropertyNames(objID)
	if err != types.E_NONE {
		return types.Err(err)
	}

	values := make([]types.Value, 0, len(names))
	for _, name := range names {
		values = append(values, types.NewStr(name))
	}

	return types.Ok(types.NewList(values))
}

// builtinPropertyInfo implements property_info(object, name)
// Returns {owner, perms} where perms is a string like "rw"
func builtinPropertyInfo(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if _, ok := args[0].(types.WaifValue); ok {
		return types.Err(types.E_INVARG)
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
	prop, err := store.FindProperty(objID, nameVal.Value())
	if err != types.E_NONE {
		return types.Err(err)
	}

	// Check read permission (unless wizard or owner)
	hasWizard, errCode := store.HasObjectFlag(ctx.Programmer, db.FlagWizard)
	isWizard := errCode == types.E_NONE && hasWizard
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
func builtinSetPropertyInfo(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	if _, ok := args[0].(types.WaifValue); ok {
		return types.Err(types.E_INVARG)
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
	prop, ok, err := store.LocalProperty(objID, propName)
	if err != types.E_NONE {
		return types.Err(err)
	}
	if !ok {
		return types.Err(types.E_PROPNF)
	}

	// Only the property's owner or a wizard may change its {owner, perms}
	// metadata (Toast: E_PERM otherwise).
	if !ctx.IsWizard && ctx.Programmer != prop.Owner {
		return types.Err(types.E_PERM)
	}

	// Parse info argument
	switch info := args[2].(type) {
	case types.StrValue:
		// Just permissions string
		perms, err := parsePerms(info.Value())
		if err != types.E_NONE {
			return types.Err(err)
		}
		if err := store.SetPropertyInfo(objID, propName, nil, &perms); err != types.E_NONE {
			return types.Err(err)
		}

	case types.ObjValue:
		// Just owner (leave perms unchanged)
		owner := info.ID()
		if err := store.SetPropertyInfo(objID, propName, &owner, nil); err != types.E_NONE {
			return types.Err(err)
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

		perms, err := parsePerms(permsVal.Value())
		if err != types.E_NONE {
			return types.Err(err)
		}
		owner := ownerVal.ID()
		if err := store.SetPropertyInfo(objID, propName, &owner, &perms); err != types.E_NONE {
			return types.Err(err)
		}

	default:
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewInt(0))
}

// builtinAddProperty implements add_property(object, name, value, info)
// Adds a new property to object
func builtinAddProperty(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

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

	// Anonymous objects are instances, not classes: their property structure
	// cannot be modified. ToastStunt raises E_TYPE for add_property on an
	// anonymous object.
	isAnonymous, errCode := store.ObjectIsAnonymous(objID)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if isAnonymous {
		return types.Err(types.E_TYPE)
	}

	propName := nameVal.Value()

	// Check if property name is built-in
	if isBuiltinProperty(propName) {
		return types.Err(types.E_INVARG)
	}

	// Check if property already exists on this object
	exists, err := store.HasLocalProperty(objID, propName)
	if err != types.E_NONE {
		return types.Err(err)
	}
	if exists {
		return types.Err(types.E_INVARG)
	}

	// Check if property exists in ancestor chain
	_, ancestorErr := store.FindProperty(objID, propName)
	if ancestorErr == types.E_NONE {
		// Property exists in ancestor
		return types.Err(types.E_INVARG)
	}

	// Check if property exists in any descendant
	if store.HasDefinedPropertyInDescendants(objID, propName) {
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
	if !store.Valid(owner) {
		return types.Err(types.E_INVARG)
	}

	// Check permissions: only wizard can set owner to someone else
	hasWizard, errCode := store.HasObjectFlag(ctx.Programmer, db.FlagWizard)
	isWizard := errCode == types.E_NONE && hasWizard
	if !isWizard && owner != ctx.Programmer {
		return types.Err(types.E_PERM)
	}

	prop := db.Property{
		Name:    propName,
		Value:   value,
		Owner:   owner,
		Perms:   perms,
		Clear:   false,
		Defined: true,
	}
	if err := store.DefineProperty(objID, prop); err != types.E_NONE {
		return types.Err(err)
	}

	// Note: ToastStunt does NOT invalidate anonymous descendants when a parent's
	// property schema changes; they remain valid.

	return types.Ok(types.NewInt(0))
}

// builtinDeleteProperty implements delete_property(object, name)
// Removes property from object
func builtinDeleteProperty(ctx *types.TaskContext, args []types.Value) types.Result {
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

	defined, err := store.IsPropertyDefinedOnObject(objID, propName)
	if err != types.E_NONE {
		return types.Err(err)
	}
	if !defined {
		return types.Err(types.E_PROPNF)
	}

	// TODO: Check permissions (owner or wizard)

	if err := store.DeleteDefinedProperty(objID, propName); err != types.E_NONE {
		return types.Err(err)
	}

	// Note: ToastStunt does NOT invalidate anonymous descendants when a parent's
	// property schema changes; they remain valid.

	return types.Ok(types.NewInt(0))
}

// builtinClearProperty implements clear_property(object, name)
// Clears property to inherit from parent
func builtinClearProperty(ctx *types.TaskContext, args []types.Value) types.Result {
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
	foundProp, err := store.FindProperty(objID, propName)
	if err != types.E_NONE {
		return types.Err(err)
	}

	// Check if property is defined on this object - E_INVARG if so
	defined, defErr := store.IsPropertyDefinedOnObject(objID, propName)
	if defErr != types.E_NONE {
		return types.Err(defErr)
	}
	if defined {
		return types.Err(types.E_INVARG)
	}

	// Check write permission (unless wizard or owner)
	hasWizard, errCode := store.HasObjectFlag(ctx.Programmer, db.FlagWizard)
	isWizard := errCode == types.E_NONE && hasWizard
	isOwner := ctx.Programmer == foundProp.Owner
	if !isWizard && !isOwner && !foundProp.Perms.Has(db.PropWrite) {
		return types.Err(types.E_PERM)
	}

	if err := store.ClearPropertyOverride(objID, propName); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(types.NewInt(0))
}

// builtinIsClearProperty implements is_clear_property(object, name)
// Tests if property is cleared (inheriting)
// Returns 1 if property is clear or only inherited, 0 if has local value
func builtinIsClearProperty(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if _, ok := args[0].(types.WaifValue); ok {
		return types.Err(types.E_INVARG)
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
	definingProp, err := store.FindProperty(objID, propName)
	if err != types.E_NONE {
		return types.Err(err)
	}

	isClear, clearErr := store.PropertyClearState(objID, propName)
	if clearErr != types.E_NONE {
		return types.Err(clearErr)
	}

	// NOW check read permission (unless wizard or owner)
	hasWizard, errCode := store.HasObjectFlag(ctx.Programmer, db.FlagWizard)
	isWizard := errCode == types.E_NONE && hasWizard
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
