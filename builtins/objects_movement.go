package builtins

import (
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

// builtinMove implements move(what, where[, position])
// Moves object to new location
func builtinMove(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store
	registry, ok := ctx.Registry.(*Registry)
	if !ok {
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

	if !store.Valid(whatVal.ID()) || (whereVal.ID() != types.ObjNothing && !store.Valid(whereVal.ID())) {
		return types.Err(types.E_INVARG)
	}

	// Check for recursive move (moving into self or descendant)
	if store.HasContentDescendant(whatVal.ID(), whereVal.ID()) {
		return types.Err(types.E_RECMOVE)
	}

	if whereVal.ID() != types.ObjNothing {
		result := registry.CallVerb(whereVal.ID(), "accept", []types.Value{whatVal}, ctx)
		if result.Flow == types.FlowException {
			if result.Error != types.E_VERBNF {
				return result
			}
		} else if !result.Val.Truthy() && !ctx.IsWizard {
			return types.Err(types.E_NACC)
		}
	}

	if errCode := store.MoveObject(whatVal.ID(), whereVal.ID(), position); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	// TODO: Call exitfunc and enterfunc verbs (Phase 9)

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
func builtinOccupants(ctx *kernel.TaskContext, args []types.Value) types.Result {
	store := ctx.Store

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
		if !store.Valid(item.ID()) {
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
		if !store.Valid(objID) {
			return false
		}

		for _, parentID := range parents {
			if store.HasAncestor(objID, parentID) {
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

		if !store.Valid(objID) {
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
			hasPlayerFlag, errCode := store.HasObjectFlag(objID, dbstore.FlagUser)
			playerMatches = errCode == types.E_NONE && hasPlayerFlag
		}

		// Add to results if both conditions pass
		if parentMatches && playerMatches {
			result = append(result, types.NewObj(objID))
		}
	}

	return types.Ok(types.NewList(result))
}
