package vm

import (
	"barn/builtins"
	"barn/db"
	"barn/types"
	"sort"
)

// collectAnonymousRefsForGC finds anonymous object references inside value trees.
func collectAnonymousRefsForGC(v types.Value, out map[types.ObjID]struct{}) {
	switch val := v.(type) {
	case types.ObjValue:
		if val.IsAnonymous() {
			out[val.ID()] = struct{}{}
		}
	case types.ListValue:
		for _, elem := range val.Elements() {
			collectAnonymousRefsForGC(elem, out)
		}
	case types.MapValue:
		for _, pair := range val.Pairs() {
			collectAnonymousRefsForGC(pair[0], out)
			collectAnonymousRefsForGC(pair[1], out)
		}
	}
}

func collectCompositeAnonymousRefs(v types.Value, out map[types.ObjID]struct{}) {
	switch v.(type) {
	case types.ListValue, types.MapValue:
		collectAnonymousRefsForGC(v, out)
	}
}

func buildPersistentAnonymousReachability(store *db.Store) map[types.ObjID]struct{} {
	reachable := make(map[types.ObjID]struct{})
	if store == nil {
		return reachable
	}

	queue := make([]types.ObjID, 0)
	enqueueRefs := func(v types.Value) {
		refs := make(map[types.ObjID]struct{})
		collectAnonymousRefsForGC(v, refs)
		for id := range refs {
			queue = append(queue, id)
		}
	}

	for _, obj := range store.All() {
		if obj == nil || obj.Recycled || obj.Flags.Has(db.FlagInvalid) || obj.Anonymous {
			continue
		}
		for _, prop := range obj.Properties {
			if prop == nil {
				continue
			}
			enqueueRefs(prop.Value)
		}
	}

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		if _, seen := reachable[id]; seen {
			continue
		}

		obj := store.GetUnsafe(id)
		if obj == nil || obj.Recycled || obj.Flags.Has(db.FlagInvalid) || !obj.Anonymous {
			continue
		}

		reachable[id] = struct{}{}
		for _, prop := range obj.Properties {
			if prop == nil {
				continue
			}
			enqueueRefs(prop.Value)
		}
	}

	return reachable
}

func pendingFinalizationValues(store *db.Store, refs map[types.ObjID]struct{}) []types.Value {
	if len(refs) == 0 || store == nil {
		return nil
	}

	reachable := buildPersistentAnonymousReachability(store)
	ids := make([]types.ObjID, 0, len(refs))
	for id := range refs {
		obj := store.GetUnsafe(id)
		if obj == nil || obj.Recycled || obj.Flags.Has(db.FlagInvalid) || !obj.Anonymous {
			continue
		}
		if _, keep := reachable[id]; keep {
			continue
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil
	}

	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	values := make([]types.Value, 0, len(ids))
	for _, id := range ids {
		values = append(values, types.NewAnon(id))
	}
	return values
}

// CollectPendingFinalizationValues snapshots anonymous-object references held by
// a live VM and returns the bare anonymous IDs that still need pending
// finalization because they are not already reachable from persistent object
// properties.
func CollectPendingFinalizationValues(store *db.Store, exec *VM) []types.Value {
	if store == nil || exec == nil {
		return nil
	}

	refs := make(map[types.ObjID]struct{})
	for _, frame := range exec.Frames {
		if frame == nil {
			continue
		}
		for _, value := range frame.Locals {
			collectCompositeAnonymousRefs(value, refs)
		}
	}
	for i := 0; i < exec.SP && i < len(exec.Stack); i++ {
		collectCompositeAnonymousRefs(exec.Stack[i], refs)
	}

	return pendingFinalizationValues(store, refs)
}

// AutoRecycleOrphanAnonymous recycles anonymous objects that are not reachable
// from any persistent non-anonymous object's properties.
func (e *Evaluator) AutoRecycleOrphanAnonymous(ctx *types.TaskContext) {
	AutoRecycleOrphanAnonymousWith(e.store, e.builtins, ctx)
}

// AutoRecycleOrphanAnonymousWith recycles anonymous objects that are not reachable
// from any persistent non-anonymous object's properties.
func AutoRecycleOrphanAnonymousWith(store *db.Store, registry *builtins.Registry, ctx *types.TaskContext) {
	AutoRecycleOrphanAnonymousSince(store, registry, ctx, 0)
}

// AutoRecycleOrphanAnonymousSince performs orphan-anonymous collection but only
// recycles anonymous objects with IDs >= minID. This lets task/eval callers
// collect objects created during the current execution without sweeping
// pre-existing database state.
func AutoRecycleOrphanAnonymousSince(store *db.Store, registry *builtins.Registry, ctx *types.TaskContext, minID types.ObjID) {
	if ctx == nil || store == nil || registry == nil {
		return
	}

	reachable := buildPersistentAnonymousReachability(store)

	// Recycle all currently-valid anonymous objects that are unreachable.
	candidates := make([]types.ObjID, 0)
	for _, obj := range store.GetAnonymousObjects() {
		if obj == nil || obj.Recycled || obj.Flags.Has(db.FlagInvalid) {
			continue
		}
		if obj.ID < minID {
			continue
		}
		// Never auto-recycle player objects even if they carry the 'a' flag.
		if obj.Flags.Has(db.FlagUser) {
			continue
		}
		if _, keep := reachable[obj.ID]; keep {
			continue
		}
		candidates = append(candidates, obj.ID)
	}

	if len(candidates) == 0 {
		return
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i] < candidates[j] })

	recycleFn, ok := registry.Get("recycle")
	if !ok {
		return
	}

	for _, id := range candidates {
		// Best-effort cleanup: recycle() handles missing/already-invalid objects.
		_ = recycleFn(ctx, []types.Value{types.NewAnon(id)})
	}
}
