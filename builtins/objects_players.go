package builtins

import (
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

// isPlayerWizard checks if a player object has wizard permissions
func isPlayerWizard(ctx *Execution, objID types.ObjID) bool {
	hasWizard, errCode := hasObjectFlagForRead(ctx, objID, dbstore.FlagWizard)
	return errCode == types.E_NONE && hasWizard
}

// builtinPlayers implements players()
// Returns a list of all player objects
func builtinPlayers(ctx *Execution, args []types.Value) types.Result {
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
func builtinIsPlayer(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Waifs can't be players
	if args[0].Type() == types.TYPE_WAIF {
		return types.Err(types.E_TYPE)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}
	if objVal.ID() == types.ObjNothing {
		return types.Err(types.E_INVARG)
	}

	if !validForRead(ctx, objVal.ID()) {
		return types.Err(types.E_INVARG)
	}

	// Anonymous objects cannot be players - E_TYPE per MOO spec
	isAnonymous, errCode := objectIsAnonymousForRead(ctx, objVal.ID())
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}
	if isAnonymous {
		return types.Err(types.E_TYPE)
	}

	hasPlayerFlag, errCode := hasObjectFlagForRead(ctx, objVal.ID(), dbstore.FlagUser)
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
func builtinSetPlayerFlag(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	// Waifs can't have player flag set
	if args[0].Type() == types.TYPE_WAIF {
		return types.Err(types.E_TYPE)
	}

	objVal := args[0]
	if !isObjectRef(objVal) {
		return types.Err(types.E_TYPE)
	}
	if objVal.ID() == types.ObjNothing {
		return types.Err(types.E_INVARG)
	}

	if !validForRead(ctx, objVal.ID()) {
		return types.Err(types.E_INVARG)
	}

	// Anonymous objects cannot have player flag set - E_TYPE per MOO spec
	isAnonymous, errCode := objectIsAnonymousForRead(ctx, objVal.ID())
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
	clearingPlayerFlag := !args[1].Truthy()
	tx := readTxn(ctx)
	if errCode := tx.SetObjectFlag(objVal.ID(), dbstore.FlagUser, !clearingPlayerFlag); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if cm := hostOf(ctx).ConnManager; clearingPlayerFlag && cm != nil && resolveConnection(ctx, objVal.ID()) != nil {
		if tx.IsDirect() {
			_ = cm.BootPlayer(objVal.ID())
		} else {
			enqueuePendingEffect(ctx, kernel.PendingEffect{
				Kind:       kernel.PendingEffectBootPlayer,
				BootPlayer: objVal.ID(),
			})
		}
	}
	return types.Ok(types.NewInt(0))
}
