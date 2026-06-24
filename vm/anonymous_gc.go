package vm

import (
	"barn/builtins"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
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

// CollectAnonymousRefsFromVM gathers the anonymous-object IDs reachable from a VM's
// frames and stack into out. It touches only VM state (no Store lock), so callers may
// run it while holding the scheduler lock to snapshot a sibling task's references
// without racing that task's execution.
func CollectAnonymousRefsFromVM(exec *VM, out map[types.ObjID]struct{}) {
	collectAnonymousRefsFromVM(exec, out)
}

func collectAnonymousRefsFromVM(exec *VM, out map[types.ObjID]struct{}) {
	if exec == nil {
		return
	}
	for _, frame := range exec.Frames {
		if frame == nil {
			continue
		}
		for _, value := range frame.Locals {
			collectAnonymousRefsForGC(value, out)
		}
	}
	for i := 0; i < exec.SP && i < len(exec.Stack); i++ {
		collectAnonymousRefsForGC(exec.Stack[i], out)
	}
}

func buildPersistentAnonymousReachability(store *dbstore.Store) map[types.ObjID]struct{} {
	if store == nil {
		return map[types.ObjID]struct{}{}
	}
	return store.PersistentAnonymousReachability()
}

func pendingFinalizationValues(store *dbstore.Store, refs map[types.ObjID]struct{}) []types.Value {
	if len(refs) == 0 || store == nil {
		return nil
	}

	reachable := buildPersistentAnonymousReachability(store)
	return store.UnreachableAnonymousValues(reachable, refs)
}

func expandAnonymousReachability(store *dbstore.Store, reachable map[types.ObjID]struct{}, refs map[types.ObjID]struct{}) {
	if store != nil {
		store.ExpandAnonymousReachability(reachable, refs)
	}
}

// CollectPendingFinalizationValues snapshots anonymous-object references held by
// a live VM and returns the bare anonymous IDs that still need pending
// finalization because they are not already reachable from persistent object
// properties.
func CollectPendingFinalizationValues(store *dbstore.Store, exec *VM) []types.Value {
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

// AutoRecycleOrphanAnonymousWith recycles anonymous objects that are not reachable
// from any persistent non-anonymous object's properties.
func AutoRecycleOrphanAnonymousWith(store *dbstore.Store, registry *builtins.Registry, ctx *kernel.TaskContext) {
	AutoRecycleOrphanAnonymousSince(store, registry, ctx, 0, nil)
}

// AutoRecycleOrphanAnonymousSince performs orphan-anonymous collection but only
// recycles anonymous objects with IDs >= minID. This lets task/eval callers
// collect objects created during the current execution without sweeping
// pre-existing database state.
// siblingRefs holds anonymous IDs already collected from other tasks' VMs (under the
// scheduler lock, so they were snapshotted without racing those tasks). localVMs are
// VMs owned by the calling goroutine (this task's own VM), safe to walk here.
func AutoRecycleOrphanAnonymousSince(store *dbstore.Store, registry *builtins.Registry, ctx *kernel.TaskContext, minID types.ObjID, siblingRefs map[types.ObjID]struct{}, localVMs ...*VM) {
	if ctx == nil || store == nil || registry == nil {
		return
	}

	reachable := buildPersistentAnonymousReachability(store)
	liveRefs := make(map[types.ObjID]struct{}, len(siblingRefs))
	for id := range siblingRefs {
		liveRefs[id] = struct{}{}
	}
	if callerVM, ok := ctx.CallerVM.(*VM); ok {
		collectAnonymousRefsFromVM(callerVM, liveRefs)
	}
	for _, exec := range localVMs {
		collectAnonymousRefsFromVM(exec, liveRefs)
	}
	expandAnonymousReachability(store, reachable, liveRefs)

	candidates := store.AnonymousRecycleCandidates(reachable, minID)
	if len(candidates) == 0 {
		return
	}

	recycleFn, ok := registry.Get("recycle")
	if !ok {
		return
	}

	for _, id := range candidates {
		// Best-effort cleanup: recycle() handles missing/already-invalid objects.
		_ = recycleFn(ctx, []types.Value{types.NewAnon(id)})
	}
}
