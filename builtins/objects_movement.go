package builtins

import (
	"barn/db"
	"barn/types"
)

// builtinMove implements move(what, where[, position])
// Moves object to new location
func builtinMove(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}
	registry, ok := ctx.Registry.(*Registry)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	whatVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	whereVal, ok := args[1].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	position := int64(0)
	if len(args) == 3 {
		positionVal, ok := args[2].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		position = positionVal.Val
		if position < 0 {
			return types.Err(types.E_INVARG)
		}
	}

	if store.Get(whatVal.ID()) == nil {
		return types.Err(types.E_INVIND)
	}

	// Check for recursive move (moving into self or descendant)
	if isDescendant(whatVal.ID(), whereVal.ID(), store) {
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
func builtinOccupants(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) < 1 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	// First arg must be a list of objects
	objectList, ok := args[0].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Validate all items are valid objects.
	for i := 1; i <= objectList.Len(); i++ {
		item := objectList.Get(i)
		objVal, ok := item.(types.ObjValue)
		if !ok {
			return types.Err(types.E_INVARG)
		}
		obj := store.Get(objVal.ID())
		if obj == nil || obj.Recycled {
			return types.Err(types.E_INVARG)
		}
	}

	// Parse optional args
	checkParent := len(args) >= 2
	var parents []types.ObjID
	if checkParent {
		// Second arg can be OBJ or LIST of OBJs
		switch v := args[1].(type) {
		case types.ObjValue:
			parents = []types.ObjID{v.ID()}
		case types.ListValue:
			for i := 1; i <= v.Len(); i++ {
				item := v.Get(i)
				objVal, ok := item.(types.ObjValue)
				if !ok {
					return types.Err(types.E_TYPE)
				}
				parents = append(parents, objVal.ID())
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
		obj := store.Get(objID)
		if obj == nil {
			return false
		}

		for _, parentID := range parents {
			// Object is always its own ancestor
			if objID == parentID {
				return true
			}

			// BFS through ancestry chain
			seen := make(map[types.ObjID]bool)
			queue := obj.Parents[:]

			for len(queue) > 0 {
				currentID := queue[0]
				queue = queue[1:]

				if seen[currentID] {
					continue
				}
				seen[currentID] = true

				if currentID == parentID {
					return true
				}

				current := store.Get(currentID)
				if current != nil {
					queue = append(queue, current.Parents...)
				}
			}
		}
		return false
	}

	// Filter objects
	var result []types.Value
	for i := 1; i <= objectList.Len(); i++ {
		item := objectList.Get(i)
		objVal := item.(types.ObjValue) // Already validated
		objID := objVal.ID()

		// Skip objects that became invalid during this call.
		obj := store.Get(objID)
		if obj == nil || obj.Recycled {
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
		playerMatches := !checkPlayerFlag || obj.Flags.Has(db.FlagUser)

		// Add to results if both conditions pass
		if parentMatches && playerMatches {
			result = append(result, types.NewObj(objID))
		}
	}

	return types.Ok(types.NewList(result))
}
