package builtins

import (
	"sort"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
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
func builtinCreate(ctx *Execution, args []types.Value) types.Result {
	store := ctx.Store
	session := ctx.Session
	if session == nil {
		return types.Err(types.E_INVARG)
	}

	if len(args) < 1 {
		return types.Err(types.E_ARGS)
	}

	// Get parent(s) - OBJ or negative INT, or list of same
	// Positive integers are NOT valid as parent references (E_TYPE)
	var parents []types.ObjID
	var parentFromInteger []bool
	parentsFromList := false
	switch args[0].Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		parents = []types.ObjID{args[0].ID()}
		parentFromInteger = []bool{false}
	case types.TYPE_INT:
		// Only negative integers are valid as object references
		if args[0].Int() >= 0 {
			return types.Err(types.E_TYPE)
		}
		parents = []types.ObjID{types.ObjID(args[0].Int())}
		parentFromInteger = []bool{true}
	case types.TYPE_LIST:
		// Multiple parents
		parentsFromList = true
		elements := args[0].Elements()
		parents = make([]types.ObjID, len(elements))
		parentFromInteger = make([]bool, len(elements))
		for i, elem := range elements {
			switch elem.Type() {
			case types.TYPE_OBJ, types.TYPE_ANON:
				parents[i] = elem.ID()
			case types.TYPE_INT:
				// Only negative integers are valid as object references
				if elem.Int() >= 0 {
					return types.Err(types.E_TYPE)
				}
				parents[i] = types.ObjID(elem.Int())
				parentFromInteger[i] = true
			default:
				return types.Err(types.E_TYPE)
			}
		}
	default:
		return types.Err(types.E_TYPE)
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
		switch args[i].Type() {
		case types.TYPE_OBJ, types.TYPE_ANON:
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
			owner = args[i].ID()
			ownerSpecified = true
		case types.TYPE_INT:
			// Toast treats an integer optional argument as the anonymous flag;
			// any non-zero value, including -1, requests an anonymous object.
			if anonymousSeen {
				return types.Err(types.E_TYPE)
			}
			anonymous = args[i].Int() != 0
			anonymousSeen = true
		case types.TYPE_LIST:
			// LIST is initialization arguments (only once)
			if initArgsSeen {
				return types.Err(types.E_TYPE)
			}
			initArgs = args[i].Elements()
			initArgsSeen = true
		case types.TYPE_FLOAT:
			// Float is always an error
			return types.Err(types.E_TYPE)
		case types.TYPE_MAP:
			// Map is always an error
			return types.Err(types.E_TYPE)
		default:
			return types.Err(types.E_TYPE)
		}
	}

	// Validate parents after optional argument types are checked. Toast reports
	// malformed optional argument shapes before invalid parent existence.
	// -1 ($nothing) is valid as a solo parent (means no parent)
	// -1 ($nothing) in a list is E_INVARG
	// -2, -3, -4 (special invalid object numbers) are E_TYPE (not valid object types)
	// Other negative IDs and non-existent objects are E_INVARG
	validParents := []types.ObjID{}
	seenParents := make(map[types.ObjID]bool)
	for i, parentID := range parents {
		if parentID < -1 {
			if parentFromInteger[i] {
				return types.Err(types.E_TYPE)
			}
			return types.Err(types.E_INVARG)
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
		if !validForRead(ctx, parentID) {
			return types.Err(types.E_INVARG)
		}
		// Permission check deferred until after anonymous flag is parsed
		validParents = append(validParents, parentID)
	}
	parents = validParents

	duplicateProps, errCode := readTxn(ctx).HasDuplicateDefinedPropertyAmong(parents)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	if duplicateProps {
		return types.Err(types.E_INVARG)
	}

	// Validate owner if explicitly specified
	// Special case: invalid object numbers like -2, -3, -4 automatically create anonymous objects
	playerIsWizard := ctx.IsWizard || isPlayerWizard(ctx, ctx.Player)
	if ownerSpecified {
		if owner < -1 {
			// Special invalid object numbers like -2, -3, -4 ($ambiguous_match, $failed_match)
			// These automatically create anonymous objects (force anonymous flag)
			anonymous = true
			owner = ctx.Programmer // Use programmer as owner
		} else if owner != types.ObjNothing && !validForRead(ctx, owner) {
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
			parentOwner, errCode := objectOwnerForRead(ctx, parentID)
			if errCode != types.E_NONE {
				continue
			}
			isOwner := parentOwner == ctx.Programmer
			if anonymous {
				hasAnonFlag, errCode := hasObjectFlagForRead(ctx, parentID, dbstore.FlagAnonymous)
				if errCode != types.E_NONE {
					continue
				}
				if !isOwner && !hasAnonFlag {
					return types.Err(types.E_PERM)
				}
			} else {
				hasFertile, errCode := hasObjectFlagForRead(ctx, parentID, dbstore.FlagFertile)
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

	var newID types.ObjID
	tx := readTxn(ctx)
	if !ctx.LiveStoreMutated && !anonymous {
		// Decentralized create: stage the new NUMBERED object so it commits on the fast
		// path. No live mutation, no adopt — the staged object is in the txn cache for
		// read-your-writes (and for :initialize below). A later coarse builtin flushes
		// staged writes to live first (flushStagedBeforeCoarse), so it never reads stale.
		var ec types.ErrorCode
		newID, ec = tx.CreateObject(parents, owner)
		if ec != types.E_NONE {
			return types.Err(types.E_QUOTA)
		}
	} else {
		// Coarse create: anonymous objects (out-of-band, no slot), a task that has
		// already live-mutated, or no transaction. store.CreateObject reads the parents
		// from LIVE (to copy their inherited properties), so any parent staged by an
		// earlier decentralized create in this same task must be flushed to live first —
		// otherwise an anonymous child of a just-created numbered object inherits from a
		// parent the coarse store cannot see yet (E_INVIND on later property access).
		if errCode := flushStagedBeforeCoarse(ctx); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		var ec types.ErrorCode
		if anonymous {
			newID, ec = tx.CreateObject(parents, owner, true)
		} else {
			newID, ec = store.DirectTxn().CreateObject(parents, owner)
		}
		if ec != types.E_NONE {
			return types.Err(types.E_QUOTA)
		}
		markLiveStoreMutated(ctx)
		if ec := tx.AdoptLiveObject(newID); ec != types.E_NONE {
			return types.Err(ec)
		}
		adoptIDs := append([]types.ObjID{newID}, parents...)
		if ec := tx.AdoptLiveRelationships(adoptIDs...); ec != types.E_NONE {
			return types.Err(ec)
		}
	}

	// Call :initialize verb if it exists
	// The :initialize verb receives the init args and can set up the new object
	// If verb not found (E_VERBNF), that's fine - just means no initialize
	// Other errors should be propagated
	result := session.CallVerb(newID, "initialize", initArgs, ctx)
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

func beginRecycle(ctx *Execution, id types.ObjID) bool {
	return ctx.Session.startRecycle(id)
}

func (r *Session) startRecycle(id types.ObjID) bool {
	state := &r.runtime.recycle
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.ids[id] > 0 {
		return false
	}
	state.ids[id] = 1
	return true
}

func endRecycle(ctx *Execution, id types.ObjID) {
	ctx.Session.endRecycle(id)
}

func (r *Session) endRecycle(id types.ObjID) {
	state := &r.runtime.recycle
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(state.ids, id)
}

func collectAnonymousRefs(v types.Value, out map[types.ObjID]types.Value) {
	switch v.Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		if v.IsAnonymous() {
			out[v.ID()] = v
		}
	case types.TYPE_LIST:
		for _, elem := range v.Elements() {
			collectAnonymousRefs(elem, out)
		}
	case types.TYPE_MAP:
		for _, pair := range v.Pairs() {
			collectAnonymousRefs(pair[0], out)
			collectAnonymousRefs(pair[1], out)
		}
	}
}

// builtinRecycle implements recycle(object)
// Destroys an object and invokes :recycle lifecycle hooks.
type RecycleLifecycleRequest struct {
	Object      types.Value
	OldParents  []types.ObjID
	OldChildren []types.ObjID
	OldContents []types.ObjID
	OldLocation types.ObjID
}

func builtinRecycle(ctx *Execution, args []types.Value) types.Result {
	session := ctx.Session
	if session == nil {
		return types.Err(types.E_INVARG)
	}

	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if !isObjectRef(args[0]) {
		return types.Err(types.E_TYPE)
	}

	objID := args[0].ID()
	if !validForRead(ctx, objID) {
		// Object doesn't exist or was already recycled - both are E_INVARG.
		return types.Err(types.E_INVARG)
	}
	owner, ownerErr := objectOwnerForRead(ctx, objID)
	if ownerErr != types.E_NONE {
		return types.Err(ownerErr)
	}
	programmerIsWizard, wizardErr := hasObjectFlagForRead(ctx, ctx.Programmer, dbstore.FlagWizard)
	if wizardErr != types.E_NONE {
		programmerIsWizard = false
	}
	if !programmerIsWizard && owner != ctx.Programmer {
		return types.Err(types.E_PERM)
	}

	var oldParents []types.ObjID
	var oldChildren []types.ObjID
	var oldContents []types.ObjID
	oldLocation := types.ObjNothing
	tx := readTxn(ctx)
	var errCode types.ErrorCode
	oldParents, errCode = tx.Parents(objID)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	oldChildren, errCode = tx.Children(objID)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	oldContents, errCode = tx.Contents(objID)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	oldLocation, errCode = tx.Location(objID)
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}

	request := RecycleLifecycleRequest{
		Object: args[0], OldParents: oldParents, OldChildren: oldChildren,
		OldContents: oldContents, OldLocation: oldLocation,
	}
	if !beginRecycle(ctx, objID) {
		return types.Err(types.E_INVARG)
	}
	if ctx.PushRecycleLifecycle != nil {
		return ctx.PushRecycleLifecycle(request)
	}
	hookResult := session.CallVerb(objID, "recycle", []types.Value{}, ctx)
	return FinishRecycleLifecycle(ctx, request, hookResult)
}

// FinishRecycleLifecycle applies topology cleanup after the object's recycle
// verb has completed on either the owning VM or a synchronous test host.
func FinishRecycleLifecycle(ctx *Execution, request RecycleLifecycleRequest, hookResult types.Result) types.Result {
	defer endRecycle(ctx, request.Object.ID())
	store := ctx.Store
	tx := readTxn(ctx)
	objID := request.Object.ID()
	oldParents := request.OldParents
	oldChildren := request.OldChildren
	oldContents := request.OldContents
	oldLocation := request.OldLocation

	// Recycle anonymous objects reachable via property values (including nested
	// list/map values) before this object is destroyed.
	anonRefs := make(map[types.ObjID]types.Value)
	propValues, errCode := tx.PropertyValues(objID)
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
	if errCode := flushStagedBeforeCoarse(ctx); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	for _, contentID := range oldContents {
		content := moveObjectReferenceForRead(ctx, contentID)
		if ec := applyRecycleMove(ctx, contentID, objID); ec != types.E_NONE {
			return types.Err(ec)
		}
		if _, _, err := findVerbForRead(ctx, objID, "exitfunc"); err == nil {
			result := ctx.Session.CallVerb(objID, "exitfunc", []types.Value{content}, ctx)
			if result.IsError() && result.Error != types.E_VERBNF {
				return result
			}
		}
	}
	if oldLocation != types.ObjNothing {
		if ec := applyRecycleMove(ctx, objID, oldLocation); ec != types.E_NONE {
			return types.Err(ec)
		}
		if _, _, err := findVerbForRead(ctx, oldLocation, "exitfunc"); err == nil {
			result := ctx.Session.CallVerb(oldLocation, "exitfunc", []types.Value{request.Object}, ctx)
			if result.IsError() && result.Error != types.E_VERBNF {
				return result
			}
		}
	}

	// Recycle: decentralized for a SIMPLE, non-player object in a not-yet-live-mutated
	// task (the common create;recycle build shape); otherwise the coarse store.Recycle,
	// which reparents children and boots player connections. The anon cascade above
	// sets liveMutated when it fires, so objects holding anon refs correctly go coarse.
	isPlayer, _ := hasObjectFlagForRead(ctx, objID, dbstore.FlagUser)
	isAnon, _ := objectIsAnonymousForRead(ctx, objID)
	decentralized := false
	if !ctx.LiveStoreMutated && !isPlayer && !isAnon {
		// Anonymous objects live out-of-band with no numbered slot, so the decentralized
		// committer (which publishes into numbered slots) can't tombstone one — they stay
		// coarse. Players stay coarse too (the connection boot is an irreversible side
		// effect that must not run on a staged, retryable recycle).
		handled, ec := tx.RecycleObject(objID)
		if ec != types.E_NONE {
			return types.Err(ec)
		}
		decentralized = handled
	}
	if decentralized {
		// Staged tombstone; invalidate the verb cache. No player boot (non-player), no
		// ForgetObject — the reads RecycleObject recorded are the conflict guard against
		// a concurrent move-into/create-under the dying object.
		store.NoteVerbCacheClear()
	} else {
		// Coarse recycle reads/reparents through the LIVE store, so flush any topology this
		// task staged decentrally first (e.g. a just-created child of the object being
		// recycled) — otherwise store.Recycle cannot see it. Mirrors the coarse create path.
		if errCode := flushStagedBeforeCoarse(ctx); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		if err := store.Recycle(objID); err != nil {
			return types.Err(types.E_INVARG)
		}
		markLiveStoreMutated(ctx)
		tx.ForgetObject(objID)
		adoptIDs := append([]types.ObjID{}, oldParents...)
		adoptIDs = append(adoptIDs, oldChildren...)
		adoptIDs = append(adoptIDs, oldContents...)
		if oldLocation != types.ObjNothing {
			adoptIDs = append(adoptIDs, oldLocation)
		}
		liveAdoptIDs := adoptIDs[:0]
		for _, id := range adoptIDs {
			if store.DirectTxn().Valid(id) {
				liveAdoptIDs = append(liveAdoptIDs, id)
			}
		}
		if errCode := tx.AdoptLiveRelationships(liveAdoptIDs...); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		if cm := hostOf(ctx).ConnManager; cm != nil {
			_ = cm.RecyclePlayer(objID)
		}
		store.NoteVerbCacheClear()
	}
	if hookResult.IsError() && hookResult.Error != types.E_VERBNF {
		return hookResult
	}

	return types.Ok(types.NewInt(0))
}

func applyRecycleMove(ctx *Execution, object, oldLocation types.ObjID) types.ErrorCode {
	if errCode := ctx.Store.DirectTxn().MoveObject(object, types.ObjNothing, 0); errCode != types.E_NONE {
		return errCode
	}
	markLiveStoreMutated(ctx)
	return readTxn(ctx).AdoptLiveRelationships(object, oldLocation)
}

// builtinValid implements valid(object)
// Tests if an object exists and is not recycled
// Accepts both ObjValue and IntValue (integers are implicitly converted to object IDs)
// Waifs are never valid (always returns 0)
func builtinValid(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Waifs are never valid
	if args[0].Type() == types.TYPE_WAIF {
		return types.Ok(types.NewInt(0))
	}

	// Toast bf_valid (objects.cc) accepts only object-flavored values
	// (is_object(): OBJ/ANON/WAIF); anything else — including plain ints —
	// is E_TYPE. Mongoose room titles depend on valid(<int>) raising: the
	// debug-off title verb treats the E_TYPE value as false.
	var objID types.ObjID
	switch args[0].Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		objID = args[0].ID()
	default:
		return types.Err(types.E_TYPE)
	}

	isValid := validForRead(ctx, objID)
	if isValid {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// builtinMaxObject implements max_object()
// Returns the highest allocated object as an object value.
func builtinMaxObject(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Read through the transaction so a decentralized create() staged earlier in this
	// verb is visible to max_object() (read-your-writes).
	maxID := readTxn(ctx).MaxObject()
	return types.Ok(types.NewObj(types.ObjID(maxID)))
}
