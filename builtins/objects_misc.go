package builtins

import (
	"barn/kernel"
	"barn/types"
)

// builtinRenumber implements renumber(obj) - wizard only
// Reassigns object to lowest available object ID
// Returns the new object ID
func builtinRenumber(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// TODO: Check caller is wizard
	// if !isWizard(ctx.Programmer) {
	// 	return types.Err(types.E_PERM)
	// }

	// Get object to renumber
	objVal := args[0]
	if !isObjectRef(objVal) {
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

// builtinNewWaif implements new_waif() - creates a new waif instance
// The waif's class is the caller (the object whose verb called new_waif)
// The waif's owner is the programmer (task permissions)
func builtinNewWaif(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Get caller (the object whose verb called new_waif)
	// In barn, ctx.ThisObj is the object whose verb is currently executing
	callerID := ctx.ThisObj

	// Caller must be a valid object (not $nothing or invalid)
	if callerID < 0 {
		return types.Err(types.E_INVARG)
	}

	// Check if class object is valid
	if !store.Valid(callerID) {
		return types.Err(types.E_INVIND)
	}

	isAnonymous, errCode := store.ObjectIsAnonymous(callerID)
	if errCode != types.E_NONE {
		return types.Err(types.E_INVIND)
	}
	if isAnonymous {
		return types.Err(types.E_INVARG)
	}

	// Owner is the programmer (task permissions)
	owner := ctx.Programmer

	// Create the waif
	waif := types.NewWaif(callerID, owner)
	return types.Ok(waif)
}

// builtinObjectBytes implements object_bytes(object)
// Returns the approximate memory size of an object in bytes
// Requires wizard permissions
func builtinObjectBytes(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Check argument type
	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}

	// Check if object is valid (not recycled)
	objID := objVal.ID()
	if objID == types.ObjNothing {
		return types.Err(types.E_INVIND)
	}
	if !store.Valid(objID) {
		// Check if recycled vs never existed
		if store.IsRecycled(objID) {
			return types.Err(types.E_INVIND)
		}
		return types.Err(types.E_INVARG)
	}

	// Check wizard permissions
	playerIsWizard := ctx.IsWizard || isPlayerWizard(store, ctx.Player)
	if !playerIsWizard {
		return types.Err(types.E_PERM)
	}

	bytes, errCode := store.ObjectByteEstimate(objID)
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(int64(bytes)))
}
