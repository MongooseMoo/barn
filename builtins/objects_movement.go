package builtins

import (
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// MoveLifecycleRequest is the persistent data the owning VM needs to run
// move() lifecycle verbs on its resumable call stack.
type MoveLifecycleRequest struct {
	What          types.Value
	Where         types.Value
	Position      int64
	Decentralized bool
}

// builtinMove implements move(what, where[, position]).
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
	initialLocation, errCode := locationForRead(ctx, whatVal.ID())
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

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

	// MOO treats a move within the current location as a no-op/reorder: it
	// neither asks the container to accept the object nor runs lifecycle hooks.
	sameLocation := whereVal.ID() == initialLocation
	if sameLocation && position == 0 {
		return types.Ok(types.NewInt(0))
	}
	if !sameLocation && ctx.PushMoveLifecycle != nil {
		return ctx.PushMoveLifecycle(MoveLifecycleRequest{
			What:          whatVal,
			Where:         whereVal,
			Position:      position,
			Decentralized: decentralized,
		})
	}

	if !sameLocation && whereVal.ID() != types.ObjNothing {
		result := session.CallVerb(whereVal.ID(), "accept", []types.Value{whatVal}, ctx)
		if result.Flow == types.FlowException {
			if result.Error != types.E_VERBNF {
				return result
			}
		} else if !result.Val.Truthy() && !ctx.IsWizard {
			return types.Err(types.E_NACC)
		}
	}
	oldLocation, errCode := ApplyMoveLifecycle(ctx, whatVal, whereVal, position, decentralized)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	if oldLocation.ID() == whereVal.ID() {
		return types.Ok(types.NewInt(0))
	}

	// The object has already moved when these callbacks run. Missing hooks are
	// ignored; any other exception aborts the move task, as required by MOO semantics.
	if oldLocation.ID() != types.ObjNothing && validForRead(ctx, oldLocation.ID()) {
		result := session.CallVerb(oldLocation.ID(), "exitfunc", []types.Value{whatVal}, ctx)
		if result.Flow == types.FlowException && result.Error != types.E_VERBNF {
			return result
		}
	}

	// exitfunc is allowed to move or recycle the object. Only call the original
	// destination's enterfunc if both objects remain valid and containment still
	// points at that destination.
	if MoveLifecycleAtDestination(ctx, whatVal, whereVal) {
		result := session.CallVerb(whereVal.ID(), "enterfunc", []types.Value{whatVal}, ctx)
		if result.Flow == types.FlowException && result.Error != types.E_VERBNF {
			return result
		}
	}

	return types.Ok(types.NewInt(0))
}

// ApplyMoveLifecycle captures the source location immediately before the raw
// move and applies the transactional or coarse topology mutation.
func ApplyMoveLifecycle(ctx *Execution, what, where types.Value, position int64, decentralized bool) (types.Value, types.ErrorCode) {
	oldLocation, errCode := locationForRead(ctx, what.ID())
	if errCode != types.E_NONE {
		return types.None, errCode
	}
	oldLocationValue := moveObjectReferenceForRead(ctx, oldLocation)
	if oldLocation == where.ID() && position == 0 {
		return oldLocationValue, types.E_NONE
	}

	tx := readTxn(ctx)
	if decentralized {
		if errCode := tx.MoveObject(what.ID(), where.ID(), position); errCode != types.E_NONE {
			return types.None, errCode
		}
		return oldLocationValue, types.E_NONE
	}

	if errCode := ctx.Store.DirectTxn().MoveObject(what.ID(), where.ID(), position); errCode != types.E_NONE {
		return types.None, errCode
	}
	markLiveStoreMutated(ctx)
	if errCode := tx.AdoptLiveRelationships(what.ID(), where.ID(), oldLocation); errCode != types.E_NONE {
		return types.None, errCode
	}
	return oldLocationValue, types.E_NONE
}

// MoveLifecycleAtDestination reports whether exitfunc left both objects valid
// and the moved object still contained by the original destination.
func MoveLifecycleAtDestination(ctx *Execution, what, where types.Value) bool {
	if where.ID() == types.ObjNothing || !validForRead(ctx, what.ID()) || !validForRead(ctx, where.ID()) {
		return false
	}
	currentLocation, errCode := locationForRead(ctx, what.ID())
	return errCode == types.E_NONE && currentLocation == where.ID()
}

func moveObjectReferenceForRead(ctx *Execution, object types.ObjID) types.Value {
	isAnonymous, errCode := objectIsAnonymousForRead(ctx, object)
	if errCode == types.E_NONE && isAnonymous {
		return types.NewAnon(object)
	}
	return types.NewObj(object)
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
