package builtins

import (
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// builtinMove implements move(what, where[, position])
// Moves object to new location
func builtinMove(ctx *Execution, args []types.Value) types.Result {
	store := ctx.Store
	session := ctx.Session
	if session == nil {
		return types.Err(types.E_INVARG)
	}

	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	whatVal := args[0]
	if !isObjectRef(whatVal) {
		return types.Err(types.E_TYPE)
	}

	whereVal := args[1]
	if !isObjectRef(whereVal) {
		return types.Err(types.E_TYPE)
	}

	position := int64(0)
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		position = args[2].Int()
		if position < 0 {
			return types.Err(types.E_INVARG)
		}
	}

	if !validForRead(ctx, whatVal.ID()) {
		return types.Err(types.E_INVARG)
	}
	if whereVal.ID() != types.ObjNothing && !validForRead(ctx, whereVal.ID()) {
		return types.Err(types.E_INVARG)
	}

	tx := readTxn(ctx)

	// Decentralize the move ONLY when the task has not already mutated the live store
	// directly (via a coarse builtin like create/recycle/renumber earlier in the same
	// task). A decentralized move only STAGES its writes in the transaction; a coarse
	// builtin reads and mutates the LIVE store, so mixing the two in one task would let
	// the coarse builtin see stale live state. Pure-move tasks (the hot path) stay
	// decentralized; mixed tasks fall back to the coarse move so live stays consistent.
	decentralized := !ctx.LiveStoreMutated

	// Check for recursive move (moving into self or descendant). Through the txn when
	// decentralized (records read deps so two concurrent moves cannot each create a
	// cycle); against the live store on the coarse path (which mutates live in place).
	var recursive bool
	if decentralized {
		recursive = tx.HasContentDescendant(whatVal.ID(), whereVal.ID())
	} else {
		recursive = store.DirectTxn().HasContentDescendant(whatVal.ID(), whereVal.ID())
	}
	if recursive {
		return types.Err(types.E_RECMOVE)
	}

	if whereVal.ID() != types.ObjNothing {
		result := session.CallVerb(whereVal.ID(), "accept", []types.Value{whatVal}, ctx)
		if result.Flow == types.FlowException {
			if result.Error != types.E_VERBNF {
				return result
			}
		} else if !result.Val.Truthy() && !ctx.IsWizard {
			return types.Err(types.E_NACC)
		}
	}

	if decentralized {
		// Stage the move; it commits on the decentralized MVCC path (disjoint-room
		// moves in parallel; same-room moves conflict and retry).
		if errCode := tx.MoveObject(whatVal.ID(), whereVal.ID(), position); errCode != types.E_NONE {
			return types.Err(errCode)
		}
	} else {
		// Coarse path: mutate the live store, flag the task live-mutated (so commit
		// uses the coarse path and does not retry), and adopt the changed relationship
		// facets into the transaction's cache.
		oldLocation, oldLocationErr := locationForRead(ctx, whatVal.ID())
		if errCode := store.DirectTxn().MoveObject(whatVal.ID(), whereVal.ID(), position); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		markLiveStoreMutated(ctx)
		adoptIDs := []types.ObjID{whatVal.ID(), whereVal.ID()}
		if oldLocationErr == types.E_NONE {
			adoptIDs = append(adoptIDs, oldLocation)
		}
		if errCode := tx.AdoptLiveRelationships(adoptIDs...); errCode != types.E_NONE {
			return types.Err(errCode)
		}
	}

	// TODO: Call exitfunc and enterfunc verbs (Phase 9) — unimplemented in Toast-Barn
	// parity terms; no conformance coverage exists for them.

	return types.Ok(types.NewInt(0))
}

// builtinOccupants implements occupants(objects [, parent [, player_flag [, inverse]]])
// Filters a list of objects by parent inheritance and optionally player flag.
// - objects: LIST of objects to filter
// - parent: OBJ or LIST of OBJs - only return objects that isa() one of these parents
// - player_flag: INT - if true, only return objects with player flag set
// - inverse: INT - if true, return objects that are NOT isa() the parent(s)
//
// With 1 arg: returns all valid objects from the list
// With 2+ args: filters by parent (isa check)
// With 3+ args: also filters by player flag
// With 4 args: inverts the parent check
func builtinOccupants(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	// First arg must be a list of objects
	objectList := args[0]
	if objectList.Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}

	// Validate all items are valid objects.
	for i := 1; i <= objectList.Len(); i++ {
		item := objectList.Get(i)
		if !isObjectRef(item) {
			return types.Err(types.E_INVARG)
		}
		if !validForRead(ctx, item.ID()) {
			return types.Err(types.E_INVARG)
		}
	}

	// Parse optional args
	checkParent := len(args) >= 2
	var parents []types.ObjID
	if checkParent {
		// Second arg can be OBJ or LIST of OBJs
		switch args[1].Type() {
		case types.TYPE_OBJ, types.TYPE_ANON:
			parents = []types.ObjID{args[1].ID()}
		case types.TYPE_LIST:
			for i := 1; i <= args[1].Len(); i++ {
				item := args[1].Get(i)
				if !isObjectRef(item) {
					return types.Err(types.E_TYPE)
				}
				parents = append(parents, item.ID())
			}
		default:
			return types.Err(types.E_TYPE)
		}
	}

	// Player flag filter (default: true if only 1 arg, otherwise use arg)
	checkPlayerFlag := len(args) == 1 || (len(args) > 2 && args[2].Truthy())

	// Inverse match (default: false)
	inverseMatch := len(args) > 3 && args[3].Truthy()

	// Helper to check if object isa any of the parents
	isaAnyParent := func(objID types.ObjID) bool {
		if !validForRead(ctx, objID) {
			return false
		}

		for _, parentID := range parents {
			hasAncestor := readTxn(ctx).HasAncestor(objID, parentID)
			if hasAncestor {
				return true
			}
		}
		return false
	}

	// Filter objects
	var result []types.Value
	for i := 1; i <= objectList.Len(); i++ {
		item := objectList.Get(i)
		objID := item.ID() // Already validated

		if !validForRead(ctx, objID) {
			continue
		}

		// Check parent filter
		parentMatches := true
		if checkParent {
			matches := isaAnyParent(objID)
			if inverseMatch {
				parentMatches = !matches
			} else {
				parentMatches = matches
			}
		}

		// Check player flag filter
		playerMatches := true
		if checkPlayerFlag {
			hasPlayerFlag, errCode := hasObjectFlagForRead(ctx, objID, dbstore.FlagUser)
			playerMatches = errCode == types.E_NONE && hasPlayerFlag
		}

		// Add to results if both conditions pass
		if parentMatches && playerMatches {
			result = append(result, types.NewObj(objID))
		}
	}

	return types.Ok(types.NewList(result))
}
