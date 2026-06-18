package builtins

import (
	"barn/db"
	"barn/types"
)

// isPlayerWizard checks if a player object has wizard permissions
func isPlayerWizard(store *db.Store, objID types.ObjID) bool {
	hasWizard, errCode := store.HasObjectFlag(objID, db.FlagWizard)
	return errCode == types.E_NONE && hasWizard
}

// builtinPlayers implements players()
// Returns a list of all player objects
func builtinPlayers(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

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
// Waifs can't be players (E_TYPE)
func builtinIsPlayer(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Waifs can't be players
	if _, ok := args[0].(types.WaifValue); ok {
		return types.Err(types.E_TYPE)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if objVal.ID() == types.ObjNothing {
		return types.Err(types.E_INVARG)
	}

	if !store.Valid(objVal.ID()) {
		return types.Err(types.E_INVARG)
	}

	// Anonymous objects cannot be players - E_TYPE per MOO spec
	isAnonymous, errCode := store.ObjectIsAnonymous(objVal.ID())
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}
	if isAnonymous {
		return types.Err(types.E_TYPE)
	}

	hasPlayerFlag, errCode := store.HasObjectFlag(objVal.ID(), db.FlagUser)
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}
	if hasPlayerFlag {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// builtinSetPlayerFlag implements set_player_flag(object, value)
// Sets or clears the player flag on an object
// Waifs can't have player flag set (E_TYPE)
func builtinSetPlayerFlag(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	// Waifs can't have player flag set
	if _, ok := args[0].(types.WaifValue); ok {
		return types.Err(types.E_TYPE)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if objVal.ID() == types.ObjNothing {
		return types.Err(types.E_INVARG)
	}

	if !store.Valid(objVal.ID()) {
		return types.Err(types.E_INVARG)
	}

	// Anonymous objects cannot have player flag set - E_TYPE per MOO spec
	isAnonymous, errCode := store.ObjectIsAnonymous(objVal.ID())
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}
	if isAnonymous {
		return types.Err(types.E_TYPE)
	}

	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Set or clear the player flag
	if args[1].Truthy() {
		if errCode := store.SetObjectFlag(objVal.ID(), db.FlagUser, true); errCode != types.E_NONE {
			return types.Err(errCode)
		}
	} else {
		if errCode := store.SetObjectFlag(objVal.ID(), db.FlagUser, false); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		// Clearing the player flag on a currently-connected player terminates
		// its live connection (matching Toast).
		if globalConnManager != nil && resolveConnection(ctx, objVal.ID()) != nil {
			_ = globalConnManager.BootPlayer(objVal.ID())
		}
	}

	return types.Ok(types.NewInt(0))
}
