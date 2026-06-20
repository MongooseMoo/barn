package builtins

import (
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

// isPlayerWizard checks if a player object has wizard permissions
func isPlayerWizard(store *dbstore.Store, objID types.ObjID) bool {
	hasWizard, errCode := store.HasObjectFlag(objID, dbstore.FlagWizard)
	return errCode == types.E_NONE && hasWizard
}

// builtinPlayers implements players()
// Returns a list of all player objects
func builtinPlayers(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

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
func builtinIsPlayer(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Waifs can't be players
	if args[0].IsWaif() {
		return types.Err(types.E_TYPE)
	}

	objID, ok := args[0].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if objID == types.ObjNothing {
		return types.Err(types.E_INVARG)
	}

	if !store.Valid(objID) {
		return types.Err(types.E_INVARG)
	}

	// Anonymous objects cannot be players - E_TYPE per MOO spec
	isAnonymous, errCode := store.ObjectIsAnonymous(objID)
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}
	if isAnonymous {
		return types.Err(types.E_TYPE)
	}

	hasPlayerFlag, errCode := store.HasObjectFlag(objID, dbstore.FlagUser)
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
func builtinSetPlayerFlag(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	// Waifs can't have player flag set
	if args[0].IsWaif() {
		return types.Err(types.E_TYPE)
	}

	objID, ok := args[0].AsObjID()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if objID == types.ObjNothing {
		return types.Err(types.E_INVARG)
	}

	if !store.Valid(objID) {
		return types.Err(types.E_INVARG)
	}

	// Anonymous objects cannot have player flag set - E_TYPE per MOO spec
	isAnonymous, errCode := store.ObjectIsAnonymous(objID)
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
		if errCode := store.SetObjectFlag(objID, dbstore.FlagUser, true); errCode != types.E_NONE {
			return types.Err(errCode)
		}
	} else {
		if errCode := store.SetObjectFlag(objID, dbstore.FlagUser, false); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		// Clearing the player flag on a currently-connected player terminates
		// its live connection (matching Toast).
		if globalConnManager != nil && resolveConnection(ctx, objID) != nil {
			_ = globalConnManager.BootPlayer(objID)
		}
	}

	return types.Ok(types.NewInt(0))
}
