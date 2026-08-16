package builtins

import (
	"strings"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// builtinProperties implements properties(object)
// Returns list of property names defined on object (not inherited)
func builtinProperties(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() == types.TYPE_WAIF {
		return types.Err(types.E_INVARG)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := objectExistsForRead(ctx, objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	allowed, errCode := objectAllowsForRead(ctx, objID, dbstore.FlagRead)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if !allowed {
		return types.Err(types.E_PERM)
	}

	names, err := definedPropertyNamesForRead(ctx, objID)
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
func builtinPropertyInfo(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() == types.TYPE_WAIF {
		return types.Err(types.E_INVARG)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	nameVal := args[1]
	if nameVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := objectExistsForRead(ctx, objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	// Find property (with inheritance)
	prop, err := findPropertyForRead(ctx, objID, nameVal.Str())
	if err != types.E_NONE {
		return types.Err(err)
	}

	// Check read permission (unless wizard or owner)
	hasWizard, errCode := hasObjectFlagForRead(ctx, ctx.Programmer, dbstore.FlagWizard)
	isWizard := errCode == types.E_NONE && hasWizard
	isOwner := ctx.Programmer == prop.Owner
	if !isWizard && !isOwner && !prop.Perms.Has(dbstore.PropRead) {
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
func builtinSetPropertyInfo(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() == types.TYPE_WAIF {
		return types.Err(types.E_INVARG)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	nameVal := args[1]
	if nameVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := objectExistsForRead(ctx, objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	propName := nameVal.Str()
	prop, ok, err := localPropertyForRead(ctx, objID, propName)
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
	switch args[2].Type() {
	case types.TYPE_STR:
		// Just permissions string
		perms, err := parsePerms(args[2].Str())
		if err != types.E_NONE {
			return types.Err(err)
		}
		errCode := readTxn(ctx).SetPropertyInfo(objID, propName, nil, &perms)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}

	case types.TYPE_OBJ, types.TYPE_ANON:
		// Just owner (leave perms unchanged)
		owner := args[2].ID()
		errCode := readTxn(ctx).SetPropertyInfo(objID, propName, &owner, nil)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}

	case types.TYPE_LIST:
		// {owner, perms}
		elements := args[2].Elements()
		if len(elements) != 2 {
			return types.Err(types.E_INVARG)
		}

		ownerVal := elements[0]
		if !isObjectRef(ownerVal) {
			return types.Err(types.E_TYPE)
		}

		permsVal := elements[1]
		if permsVal.Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}

		perms, err := parsePerms(permsVal.Str())
		if err != types.E_NONE {
			return types.Err(err)
		}
		owner := ownerVal.ID()
		errCode := readTxn(ctx).SetPropertyInfo(objID, propName, &owner, &perms)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}

	default:
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewInt(0))
}

// builtinAddProperty implements add_property(object, name, value, info)
// Adds a new property to object
func builtinAddProperty(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 4 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	nameVal := args[1]
	if nameVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	value := args[2]

	objID := objVal.ID()
	if errCode := objectExistsForRead(ctx, objID); errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}

	// Anonymous objects are instances, not classes: their property structure
	// cannot be modified. ToastStunt raises E_TYPE for add_property on an
	// anonymous object.
	isAnonymous, errCode := objectIsAnonymousForRead(ctx, objID)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if isAnonymous {
		return types.Err(types.E_TYPE)
	}

	propName := nameVal.Str()

	// Check if property name is built-in
	if IsBuiltinProperty(propName) {
		return types.Err(types.E_INVARG)
	}

	// Check if property already exists on this object
	_, exists, err := localPropertyForRead(ctx, objID, propName)
	if err != types.E_NONE {
		return types.Err(err)
	}
	if exists {
		return types.Err(types.E_INVARG)
	}

	// Check if property exists in ancestor chain
	_, ancestorErr := findPropertyForRead(ctx, objID, propName)
	if ancestorErr == types.E_NONE {
		// Property exists in ancestor
		return types.Err(types.E_INVARG)
	}

	// Check if property exists in any descendant
	if readTxn(ctx).HasDefinedPropertyInDescendants(objID, propName) {
		return types.Err(types.E_INVARG)
	}

	// Parse info argument (same as set_property_info)
	var owner types.ObjID
	var perms dbstore.PropertyPerms

	switch args[3].Type() {
	case types.TYPE_STR:
		// Just permissions string
		owner = ctx.Programmer // Default to caller
		var errCode types.ErrorCode
		perms, errCode = parsePerms(args[3].Str())
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}

	case types.TYPE_LIST:
		// {owner, perms}
		elements := args[3].Elements()
		if len(elements) != 2 {
			return types.Err(types.E_INVARG)
		}

		ownerVal := elements[0]
		if !isObjectRef(ownerVal) {
			return types.Err(types.E_TYPE)
		}

		permsVal := elements[1]
		if permsVal.Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}

		owner = ownerVal.ID()
		var errCode2 types.ErrorCode
		perms, errCode2 = parsePerms(permsVal.Str())
		if errCode2 != types.E_NONE {
			return types.Err(errCode2)
		}

	default:
		return types.Err(types.E_TYPE)
	}

	// Validate owner is a valid object
	if !validForRead(ctx, owner) {
		return types.Err(types.E_INVARG)
	}

	// Check permissions: only wizard can set owner to someone else
	hasWizard, errCode := hasObjectFlagForRead(ctx, ctx.Programmer, dbstore.FlagWizard)
	isWizard := errCode == types.E_NONE && hasWizard
	if !isWizard && owner != ctx.Programmer {
		return types.Err(types.E_PERM)
	}
	allowed, errCode := objectAllowsForRead(ctx, objID, dbstore.FlagWrite)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if !allowed {
		return types.Err(types.E_PERM)
	}

	prop := dbstore.NewProperty(value, owner, perms, false, true)
	defineErr := readTxn(ctx).DefineProperty(objID, propName, prop)
	if defineErr != types.E_NONE {
		return types.Err(defineErr)
	}

	// Note: ToastStunt does NOT invalidate anonymous descendants when a parent's
	// property schema changes; they remain valid.

	return types.Ok(types.NewInt(0))
}

// builtinDeleteProperty implements delete_property(object, name)
// Removes property from object
func builtinDeleteProperty(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	nameVal := args[1]
	if nameVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := objectExistsForRead(ctx, objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	propName := nameVal.Str()

	prop, ok, err := localPropertyForRead(ctx, objID, propName)
	if err != types.E_NONE {
		return types.Err(err)
	}
	if !ok || !prop.Defined {
		return types.Err(types.E_PROPNF)
	}
	allowed, errCode := objectAllowsForRead(ctx, objID, dbstore.FlagWrite)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if !allowed {
		return types.Err(types.E_PERM)
	}

	deleteErr := readTxn(ctx).DeleteDefinedProperty(objID, propName)
	if deleteErr != types.E_NONE {
		return types.Err(deleteErr)
	}

	// Note: ToastStunt does NOT invalidate anonymous descendants when a parent's
	// property schema changes; they remain valid.

	return types.Ok(types.NewInt(0))
}

// builtinClearProperty implements clear_property(object, name)
// Clears property to inherit from parent
func builtinClearProperty(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	nameVal := args[1]
	if nameVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := objectExistsForRead(ctx, objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	propName := nameVal.Str()

	// Check if it's a built-in property - return E_PERM
	if IsBuiltinProperty(propName) {
		return types.Err(types.E_PERM)
	}

	// Find property in chain
	foundProp, err := findPropertyForRead(ctx, objID, propName)
	if err != types.E_NONE {
		hasWizard, errCode := hasObjectFlagForRead(ctx, ctx.Programmer, dbstore.FlagWizard)
		isWizard := errCode == types.E_NONE && hasWizard
		objectOwner, ownerErr := objectOwnerForRead(ctx, objID)
		isObjectOwner := ownerErr == types.E_NONE && ctx.Programmer == objectOwner
		if !isWizard && !isObjectOwner {
			return types.Err(types.E_PERM)
		}
		return types.Err(err)
	}

	// Check write permission (unless wizard or owner)
	hasWizard, errCode := hasObjectFlagForRead(ctx, ctx.Programmer, dbstore.FlagWizard)
	isWizard := errCode == types.E_NONE && hasWizard
	isOwner := ctx.Programmer == foundProp.Owner
	if !isWizard && !isOwner && !foundProp.Perms.Has(dbstore.PropWrite) {
		return types.Err(types.E_PERM)
	}

	// Check if property is defined on this object - E_INVARG if so
	prop, defined, defErr := localPropertyForRead(ctx, objID, propName)
	if defErr != types.E_NONE {
		return types.Err(defErr)
	}
	if defined && prop.Defined {
		return types.Err(types.E_INVARG)
	}

	clearErr := readTxn(ctx).ClearPropertyOverride(objID, propName)
	if clearErr != types.E_NONE {
		return types.Err(clearErr)
	}

	return types.Ok(types.NewInt(0))
}

// builtinIsClearProperty implements is_clear_property(object, name)
// Tests if property is cleared (inheriting)
// Returns 1 if property is clear or only inherited, 0 if has local value
func builtinIsClearProperty(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() == types.TYPE_WAIF {
		return types.Err(types.E_INVARG)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_INVARG)
	}

	nameVal := args[1]
	if nameVal.Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if errCode := objectExistsForRead(ctx, objID); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	propName := nameVal.Str()

	// Check if it's a built-in property - return 0
	if IsBuiltinProperty(propName) {
		return types.Ok(types.NewInt(0))
	}

	// Find where property is defined in chain
	definingProp, err := findPropertyForRead(ctx, objID, propName)
	if err != types.E_NONE {
		return types.Err(err)
	}

	isClear, clearErr := propertyClearStateForRead(ctx, objID, propName)
	if clearErr != types.E_NONE {
		return types.Err(clearErr)
	}

	// NOW check read permission (unless wizard or owner)
	hasWizard, errCode := hasObjectFlagForRead(ctx, ctx.Programmer, dbstore.FlagWizard)
	isWizard := errCode == types.E_NONE && hasWizard
	isOwner := ctx.Programmer == definingProp.Owner
	hasReadPerm := definingProp.Perms.Has(dbstore.PropRead)
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

// IsBuiltinProperty checks if a property name is a built-in property
// Built-in properties: name, owner, location, contents, last_move, programmer, wizard, r, w, f, a.
// parent/parents/children/player are NOT properties -- they have dedicated accessor built-in
// functions (parent(), parents(), children(), is_player()), so `.parent` etc. raise E_PROPNF.
func IsBuiltinProperty(name string) bool {
	switch strings.ToLower(name) {
	case "name", "owner", "location", "contents", "last_move",
		"programmer", "wizard", "r", "w", "f", "a":
		return true
	default:
		return false
	}
}

// parsePerms converts a permission string like "rw" to PropertyPerms flags
// Returns error code if invalid characters found
func parsePerms(s string) (dbstore.PropertyPerms, types.ErrorCode) {
	var perms dbstore.PropertyPerms
	for _, c := range s {
		switch c {
		case 'r', 'R':
			perms |= dbstore.PropRead
		case 'w', 'W':
			perms |= dbstore.PropWrite
		case 'c', 'C':
			perms |= dbstore.PropChown
		default:
			return 0, types.E_INVARG
		}
	}
	return perms, types.E_NONE
}
