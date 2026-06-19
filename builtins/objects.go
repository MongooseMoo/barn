package builtins

import (
	"barn/db"
	"barn/types"
	"sort"
	"sync"
)

// builtinCreate implements create(parent [, owner] [, anonymous] [, args])
// Creates a new object with the given parent(s)
// Per cow_py semantics:
// - First arg: OBJ, negative INT (as object reference), or list of same
// - Optional args (in order):
//   - OBJ or negative INT → owner (must come before anonymous flag)
//   - Non-negative INT → anonymous flag (0 or 1)
//   - LIST → init args for :initialize verb (must come last)
//
// - Float or Map is always E_TYPE
// - Owner values < -1 (like -2, -3, -4) are E_INVARG
func builtinCreate(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}
	registry, ok := ctx.Registry.(*Registry)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) < 1 {
		return types.Err(types.E_ARGS)
	}

	// Get parent(s) - OBJ or negative INT, or list of same
	// Positive integers are NOT valid as parent references (E_TYPE)
	var parents []types.ObjID
	parentsFromList := false
	switch p := args[0].(type) {
	case types.ObjValue:
		parents = []types.ObjID{p.ID()}
	case types.IntValue:
		// Only negative integers are valid as object references
		if p.Val >= 0 {
			return types.Err(types.E_TYPE)
		}
		parents = []types.ObjID{types.ObjID(p.Val)}
	case types.ListValue:
		// Multiple parents
		parentsFromList = true
		elements := p.Elements()
		parents = make([]types.ObjID, len(elements))
		for i, elem := range elements {
			switch e := elem.(type) {
			case types.ObjValue:
				parents[i] = e.ID()
			case types.IntValue:
				// Only negative integers are valid as object references
				if e.Val >= 0 {
					return types.Err(types.E_TYPE)
				}
				parents[i] = types.ObjID(e.Val)
			default:
				return types.Err(types.E_TYPE)
			}
		}
	default:
		return types.Err(types.E_TYPE)
	}

	// Validate parents
	// -1 ($nothing) is valid as a solo parent (means no parent)
	// -1 ($nothing) in a list is E_INVARG
	// -2, -3, -4 (special invalid object numbers) are E_TYPE (not valid object types)
	// Other negative IDs and non-existent objects are E_INVARG
	validParents := []types.ObjID{}
	seenParents := make(map[types.ObjID]bool)
	for _, parentID := range parents {
		if parentID < -1 {
			// Special invalid object numbers like -2, -3, -4 ($ambiguous_match, $failed_match)
			// These are type errors because they're not valid object references
			return types.Err(types.E_TYPE)
		}
		if parentID == types.ObjNothing {
			if parentsFromList {
				// $nothing in a parent list is invalid
				return types.Err(types.E_INVARG)
			}
			// $nothing as solo parent means "no parent" - skip it
			continue
		}
		// Check for duplicate parents
		if seenParents[parentID] {
			return types.Err(types.E_INVARG)
		}
		seenParents[parentID] = true
		if !store.Valid(parentID) {
			return types.Err(types.E_INVARG)
		}
		// Permission check deferred until after anonymous flag is parsed
		validParents = append(validParents, parentID)
	}
	parents = validParents

	duplicateProps, errCode := store.HasDuplicateDefinedPropertyAmong(parents)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if duplicateProps {
		return types.Err(types.E_INVARG)
	}

	// Parse optional arguments
	// Per cow_py semantics:
	// - OBJ or negative INT → owner (must come before anonymous flag, only once)
	// - Non-negative INT → anonymous flag (0 or 1, only once)
	// - LIST → init args (only once, must be last)
	// - Float or Map is always E_TYPE
	owner := ctx.Programmer
	ownerSpecified := false
	anonymous := false
	anonymousSeen := false
	var initArgs []types.Value

	initArgsSeen := false
	for i := 1; i < len(args); i++ {
		switch v := args[i].(type) {
		case types.ObjValue:
			// ObjNum is owner - only valid before anonymous flag and initArgs
			if anonymousSeen {
				return types.Err(types.E_TYPE)
			}
			if ownerSpecified {
				return types.Err(types.E_TYPE)
			}
			if initArgsSeen {
				return types.Err(types.E_TYPE)
			}
			owner = v.ID()
			ownerSpecified = true
		case types.IntValue:
			if v.Val < 0 {
				// Negative int is owner (object reference)
				if anonymousSeen {
					return types.Err(types.E_TYPE)
				}
				if ownerSpecified {
					return types.Err(types.E_TYPE)
				}
				if initArgsSeen {
					return types.Err(types.E_TYPE)
				}
				owner = types.ObjID(v.Val)
				ownerSpecified = true
			} else {
				// Non-negative int is anonymous flag (0 or 1)
				if anonymousSeen {
					return types.Err(types.E_TYPE)
				}
				anonymous = v.Val != 0
				anonymousSeen = true
			}
		case types.ListValue:
			// LIST is initialization arguments (only once)
			if initArgsSeen {
				return types.Err(types.E_TYPE)
			}
			initArgs = v.Elements()
			initArgsSeen = true
		case types.FloatValue:
			// Float is always an error
			return types.Err(types.E_TYPE)
		case types.MapValue:
			// Map is always an error
			return types.Err(types.E_TYPE)
		default:
			return types.Err(types.E_TYPE)
		}
	}

	// Validate owner if explicitly specified
	// Special case: invalid object numbers like -2, -3, -4 automatically create anonymous objects
	playerIsWizard := ctx.IsWizard || isPlayerWizard(store, ctx.Player)
	if ownerSpecified {
		if owner < -1 {
			// Special invalid object numbers like -2, -3, -4 ($ambiguous_match, $failed_match)
			// These automatically create anonymous objects (force anonymous flag)
			anonymous = true
			owner = ctx.Programmer // Use programmer as owner
		} else if owner != types.ObjNothing && !store.Valid(owner) {
			return types.Err(types.E_INVARG)
		} else if owner == types.ObjNothing && !playerIsWizard {
			// Only wizards can specify $nothing as owner (makes object own itself)
			return types.Err(types.E_PERM)
		} else if owner != ctx.Programmer && !playerIsWizard {
			// Non-wizards can only specify themselves as owner or get E_PERM
			return types.Err(types.E_PERM)
		}
	}

	// Check permissions for creating from each parent
	// - Wizards can create from any object
	// - For anonymous objects: non-wizards need to own parent OR parent has FlagAnonymous
	// - For regular objects: non-wizards need to own parent OR parent has FlagFertile
	if !playerIsWizard {
		for _, parentID := range parents {
			parentOwner, errCode := store.ObjectOwner(parentID)
			if errCode != types.E_NONE {
				continue
			}
			isOwner := parentOwner == ctx.Programmer
			if anonymous {
				hasAnonFlag, errCode := store.HasObjectFlag(parentID, db.FlagAnonymous)
				if errCode != types.E_NONE {
					continue
				}
				if !isOwner && !hasAnonFlag {
					return types.Err(types.E_PERM)
				}
			} else {
				hasFertile, errCode := store.HasObjectFlag(parentID, db.FlagFertile)
				if errCode != types.E_NONE {
					continue
				}
				if !isOwner && !hasFertile {
					return types.Err(types.E_PERM)
				}
			}
		}
	}

	// Anonymous objects cannot have $nothing as owner
	if anonymous && owner == types.ObjNothing {
		return types.Err(types.E_INVARG)
	}

	newID, errCode := store.CreateObject(parents, owner, anonymous)
	if errCode != types.E_NONE {
		return types.Err(types.E_QUOTA)
	}

	// Call :initialize verb if it exists
	// The :initialize verb receives the init args and can set up the new object
	// If verb not found (E_VERBNF), that's fine - just means no initialize
	// Other errors should be propagated
	result := registry.CallVerb(newID, "initialize", initArgs, ctx)
	if result.Flow == types.FlowException {
		if result.Error != types.E_VERBNF {
			// Real error - propagate it
			return result
		}
		// E_VERBNF is fine - no initialize verb
	}

	// Return AnonValue for anonymous objects, ObjValue for regular
	if anonymous {
		return types.Ok(types.NewAnon(newID))
	}
	return types.Ok(types.NewObj(newID))
}

var recycleState struct {
	mu  sync.Mutex
	ids map[types.ObjID]int
}

func init() {
	recycleState.ids = make(map[types.ObjID]int)
}

func beginRecycle(id types.ObjID) bool {
	recycleState.mu.Lock()
	defer recycleState.mu.Unlock()
	if recycleState.ids[id] > 0 {
		return false
	}
	recycleState.ids[id] = 1
	return true
}

func endRecycle(id types.ObjID) {
	recycleState.mu.Lock()
	defer recycleState.mu.Unlock()
	delete(recycleState.ids, id)
}

func collectAnonymousRefs(v types.Value, out map[types.ObjID]types.ObjValue) {
	switch val := v.(type) {
	case types.ObjValue:
		if val.IsAnonymous() {
			out[val.ID()] = val
		}
	case types.ListValue:
		for _, elem := range val.Elements() {
			collectAnonymousRefs(elem, out)
		}
	case types.MapValue:
		for _, pair := range val.Pairs() {
			collectAnonymousRefs(pair[0], out)
			collectAnonymousRefs(pair[1], out)
		}
	}
}

// builtinRecycle implements recycle(object)
// Destroys an object and invokes :recycle lifecycle hooks.
func builtinRecycle(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}
	registry, ok := ctx.Registry.(*Registry)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	objVal, ok := args[0].(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()
	if !beginRecycle(objID) {
		// Recursive recycle(this) on the same target fails.
		return types.Err(types.E_INVARG)
	}
	defer endRecycle(objID)

	if !store.Valid(objID) {
		// Object doesn't exist or was already recycled - both are E_INVARG.
		return types.Err(types.E_INVARG)
	}

	// TODO: Check permissions (Layer 8.5)

	// Invoke :recycle hook if present. Missing hook and hook errors are ignored.
	// This matches lifecycle behavior: recycle should proceed even if hook throws.
	if registry != nil {
		_ = registry.CallVerb(objID, "recycle", []types.Value{}, ctx)
	}

	// Recycle anonymous objects reachable via property values (including nested
	// list/map values) before this object is destroyed.
	anonRefs := make(map[types.ObjID]types.ObjValue)
	propValues, errCode := store.PropertyValues(objID)
	if errCode != types.E_NONE {
		return types.Err(types.E_INVARG)
	}
	for _, value := range propValues {
		collectAnonymousRefs(value, anonRefs)
	}

	if len(anonRefs) > 0 {
		ids := make([]int64, 0, len(anonRefs))
		for id := range anonRefs {
			if id != objID {
				ids = append(ids, int64(id))
			}
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			ref := anonRefs[types.ObjID(id)]
			// Ignore errors while cascading anonymous recycling.
			_ = builtinRecycle(ctx, []types.Value{ref})
		}
	}

	// Note: recycling does NOT invalidate anonymous descendants in ToastStunt;
	// they remain valid (property access through the recycled parent raises
	// E_PROPNF).

	// Mark as recycled
	if err := store.Recycle(objID); err != nil {
		return types.Err(types.E_INVARG)
	}
	if globalConnManager != nil {
		_ = globalConnManager.RecyclePlayer(objID)
	}
	store.NoteVerbCacheClear()

	return types.Ok(types.NewInt(0))
}

// builtinValid implements valid(object)
// Tests if an object exists and is not recycled
// Accepts both ObjValue and IntValue (integers are implicitly converted to object IDs)
// Waifs are never valid (always returns 0)
func builtinValid(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Waifs are never valid
	if _, ok := args[0].(types.WaifValue); ok {
		return types.Ok(types.NewInt(0))
	}

	var objID types.ObjID
	switch v := args[0].(type) {
	case types.ObjValue:
		objID = v.ID()
	case types.IntValue:
		objID = types.ObjID(v.Val)
	default:
		return types.Err(types.E_TYPE)
	}

	isValid := store.Valid(objID)
	if isValid {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// builtinMaxObject implements max_object()
// Returns the highest allocated object ID
func builtinMaxObject(ctx *types.TaskContext, args []types.Value) types.Result {
	store, ok := ctx.Store.(*db.Store)
	if !ok {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	maxID := store.MaxObject()
	return types.Ok(types.NewInt(int64(maxID)))
}
