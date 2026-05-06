package builtins

import (
	"barn/db"
	"barn/types"
)

// builtinRenumber implements renumber(obj) - wizard only
// Reassigns object to lowest available object ID
// Returns the new object ID
func builtinRenumber(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

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

// builtinNewWaif implements new_waif() - creates a new waif instance
// The waif's class is the caller (the object whose verb called new_waif)
// The waif's owner is the programmer (task permissions)
func builtinNewWaif(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

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

	// Check if class object is anonymous (anonymous objects cannot be waif parents)
	classObj := store.Get(callerID)
	if classObj == nil {
		return types.Err(types.E_INVIND)
	}
	if classObj.Anonymous {
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
func builtinObjectBytes(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Check argument type
	objVal, ok := args[0].(types.ObjValue)
	if !ok {
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

	obj := store.Get(objID)
	if obj == nil {
		return types.Err(types.E_INVARG)
	}

	// Calculate approximate byte size
	bytes := calculateObjectBytes(obj, store)
	return types.Ok(types.NewInt(int64(bytes)))
}

// calculateObjectBytes calculates approximate memory usage of an object
// Based on ToastStunt's db_object_bytes implementation
func calculateObjectBytes(obj *db.Object, store *db.Store) int {
	// Start with object header size
	// sizeof(Object) + sizeof(Object*) in C
	count := 64 + 8 // Approximation for Go struct overhead

	// Object name
	count += len(obj.Name) + 1

	// Verbs
	for _, verb := range obj.Verbs {
		count += 32 // Verb struct overhead
		count += len(verb.Name) + 1
		// Program AST size (if compiled)
		if verb.Program != nil {
			count += len(verb.Program.Statements) * 64 // Approximate statement size
		}
	}

	// Property definitions (properties defined on this object)
	for _, prop := range obj.Properties {
		if prop.Defined {
			count += 32 // Propdef struct overhead
			count += len(prop.Name) + 1
		}
	}

	// Property values (all properties including inherited)
	for _, prop := range obj.Properties {
		count += 24 // Pval struct overhead (minus Var size)
		count += calculateValueBytes(prop.Value)
	}

	return count
}

// calculateValueBytes calculates approximate memory usage of a value
// Based on ToastStunt's value_bytes function
func calculateValueBytes(v types.Value) int {
	size := 16 // Base Var struct size

	switch val := v.(type) {
	case types.StrValue:
		size += len(val.Value()) + 1
	case types.FloatValue:
		size += 8 // sizeof(double)
	case types.ListValue:
		elements := val.Elements()
		size += len(elements) * 16 // List overhead
		for _, elem := range elements {
			size += calculateValueBytes(elem)
		}
	case types.MapValue:
		// Approximate map overhead
		pairs := val.Pairs()
		size += len(pairs) * 32 // Map node overhead
		for _, pair := range pairs {
			size += calculateValueBytes(pair[0]) // Key
			size += calculateValueBytes(pair[1]) // Value
		}
	case types.WaifValue:
		// Waif overhead - basic struct size
		size += 64
		// Note: Waif properties are stored on the class object, not the waif instance
		// So we just count the waif struct overhead
	}

	return size
}
