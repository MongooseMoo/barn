package vm

import (
	"barn/builtins"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

// collectAnonymousRefsForGC finds anonymous object references inside value trees.
func collectAnonymousRefsForGC(v types.Value, out map[types.ObjID]struct{}) {
	switch v.Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		if v.IsAnonymous() {
			out[v.ID()] = struct{}{}
		}
	case types.TYPE_LIST:
		for _, elem := range v.Elements() {
			collectAnonymousRefsForGC(elem, out)
		}
	case types.TYPE_MAP:
		for _, pair := range v.Pairs() {
			collectAnonymousRefsForGC(pair[0], out)
			collectAnonymousRefsForGC(pair[1], out)
		}
	}
}

func collectCompositeAnonymousRefs(v types.Value, out map[types.ObjID]struct{}) {
	switch v.Type() {
	case types.TYPE_LIST, types.TYPE_MAP:
		collectAnonymousRefsForGC(v, out)
	}
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
	AutoRecycleOrphanAnonymousSince(store, registry, ctx, 0)
}

// AnonGCRequest is one deferred orphan-anonymous collection request: recycle
// anonymous objects with ids >= MinID that are unreachable, using Ctx for the
// recycle() calls.
type AnonGCRequest struct {
	Ctx   *kernel.TaskContext
	MinID types.ObjID
}

// RecycleOrphanAnonymousBatch settles several deferred collection requests
// with a single persistent-reachability build. Per-task collection pays a
// full-database property sweep per finished task, which is prohibitive on
// large databases; batching preserves the liveness check (reachability plus
// all live task VMs at flush time) and only delays when an orphan is
// recycled.
func RecycleOrphanAnonymousBatch(store *dbstore.Store, registry *builtins.Registry, requests []AnonGCRequest, liveVMs ...*VM) {
	if store == nil || registry == nil || len(requests) == 0 {
		return
	}

	minFloor := requests[0].MinID
	for _, req := range requests[1:] {
		if req.MinID < minFloor {
			minFloor = req.MinID
		}
	}
	if !store.HasAnonymousAtOrAbove(minFloor) {
		return
	}

	reachable := buildPersistentAnonymousReachability(store)
	liveRefs := make(map[types.ObjID]struct{})
	for _, exec := range liveVMs {
		collectAnonymousRefsFromVM(exec, liveRefs)
	}
	expandAnonymousReachability(store, reachable, liveRefs)

	recycleFn, ok := registry.Get("recycle")
	if !ok {
		return
	}

	recycled := make(map[types.ObjID]struct{})
	for _, req := range requests {
		if req.Ctx == nil {
			continue
		}
		for _, id := range store.AnonymousRecycleCandidates(reachable, req.MinID) {
			if _, done := recycled[id]; done {
				continue
			}
			recycled[id] = struct{}{}
			// Best-effort cleanup: recycle() handles missing/already-invalid objects.
			_ = recycleFn(req.Ctx, []types.Value{types.NewAnon(id)})
		}
	}
}

// AutoRecycleOrphanAnonymousSince performs orphan-anonymous collection but only
// recycles anonymous objects with IDs >= minID. This lets task/eval callers
// collect objects created during the current execution without sweeping
// pre-existing database state.
func AutoRecycleOrphanAnonymousSince(store *dbstore.Store, registry *builtins.Registry, ctx *kernel.TaskContext, minID types.ObjID, extraVMs ...*VM) {
	if ctx == nil || store == nil || registry == nil {
		return
	}

	// Fast path: recycle candidates are restricted to anonymous objects with
	// ids >= minID, so when the finished task created none the reachability
	// sweep below is a guaranteed no-op. Skipping it matters: the sweep walks
	// every persistent object's property tree, which is prohibitive to pay
	// after every task on a large database.
	if !store.HasAnonymousAtOrAbove(minID) {
		return
	}

	reachable := buildPersistentAnonymousReachability(store)
	liveRefs := make(map[types.ObjID]struct{})
	if callerVM, ok := ctx.CallerVM.(*VM); ok {
		collectAnonymousRefsFromVM(callerVM, liveRefs)
	}
	for _, exec := range extraVMs {
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
