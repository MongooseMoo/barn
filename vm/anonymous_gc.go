package vm

import (
	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
	"sort"
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

func expandAnonymousReachability(store *dbstore.Store, tx *dbstore.StoreTxn, reachable map[types.ObjID]struct{}, refs map[types.ObjID]struct{}) {
	if store != nil {
		store.ExpandAnonymousReachability(reachable, refs)
	}
	if tx != nil {
		// Add the owning task's in-flight property graph without replacing the
		// live expansion: refs from sibling VMs may postdate this transaction's
		// snapshot and must remain protected by the live-store walk.
		tx.ExpandAnonymousReachability(reachable, refs)
	}
}

// CollectPendingFinalizationValues snapshots anonymous-object references held by
// a live VM and returns one bare anonymous root for each still-unreachable graph
// that must survive shutdown finalization. A single root retains its complete
// anonymous-object component for serialization; emitting every local in a cycle
// would duplicate the same pending finalization graph.
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
			collectAnonymousRefsForGC(value, refs)
		}
	}
	for i := 0; i < exec.SP && i < len(exec.Stack); i++ {
		collectAnonymousRefsForGC(exec.Stack[i], refs)
	}

	candidates := pendingFinalizationValues(store, refs)
	if len(candidates) == 0 {
		return nil
	}

	type candidateRoot struct {
		value   types.Value
		closure map[types.ObjID]struct{}
	}
	ordered := make([]candidateRoot, 0, len(candidates))
	for _, candidate := range candidates {
		closure := make(map[types.ObjID]struct{})
		store.ExpandAnonymousReachability(closure, map[types.ObjID]struct{}{
			candidate.ID(): {},
		})
		ordered = append(ordered, candidateRoot{value: candidate, closure: closure})
	}
	// A root that reaches another candidate must be considered first. Closure
	// size provides a deterministic topological order for acyclic reachability;
	// equal closures are the same cycle, so identity order chooses one member.
	sort.Slice(ordered, func(i, j int) bool {
		if len(ordered[i].closure) != len(ordered[j].closure) {
			return len(ordered[i].closure) > len(ordered[j].closure)
		}
		return ordered[i].value.ID() < ordered[j].value.ID()
	})

	covered := buildPersistentAnonymousReachability(store)
	roots := make([]types.Value, 0, len(ordered))
	for _, candidate := range ordered {
		if _, seen := covered[candidate.value.ID()]; seen {
			continue
		}
		roots = append(roots, candidate.value)
		for id := range candidate.closure {
			covered[id] = struct{}{}
		}
	}
	return roots
}

// AutoRecycleOrphanAnonymousWith recycles anonymous objects that are not reachable
// from any persistent non-anonymous object's properties.
func AutoRecycleOrphanAnonymousWith(store *dbstore.Store, registry *builtins.Registry, ctx *kernel.TaskContext) {
	AutoRecycleOrphanAnonymousSince(store, registry, ctx, 0, nil)
}

// AnonGCRequest is one deferred orphan-anonymous collection request: recycle
// anonymous objects with ids >= MinID that are unreachable, using Ctx for the
// recycle() calls. OwnRefs holds the anonymous ids the requesting task's own VM
// referenced, snapshotted at defer time by the goroutine that owned that VM. A
// completed task's VM is released before the flush runs, so its locals cannot be
// walked then; capturing the ids up front keeps them as roots without retaining
// the *VM (which a concurrent flush must never touch).
type AnonGCRequest struct {
	Ctx     *kernel.TaskContext
	MinID   types.ObjID
	OwnRefs map[types.ObjID]struct{}
}

// RecycleOrphanAnonymousBatch settles several deferred collection requests
// with a single persistent-reachability build. Per-task collection pays a
// full-database property sweep per finished task, which is prohibitive on
// large databases; batching preserves the liveness check (reachability plus
// every live task's VM references at flush time) and only delays when an orphan
// is recycled.
//
// siblingRefs holds the anonymous ids snapshotted from every live task's VM under
// the scheduler lock. Together with each request's OwnRefs it covers the same root
// set the inline per-task sweep saw, without walking a *VM here — so a task running
// concurrently on another goroutine is never read.
func RecycleOrphanAnonymousBatch(store *dbstore.Store, registry *builtins.Registry, requests []AnonGCRequest, siblingRefs map[types.ObjID]struct{}) {
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
	liveRefs := make(map[types.ObjID]struct{}, len(siblingRefs))
	for id := range siblingRefs {
		liveRefs[id] = struct{}{}
	}
	for _, req := range requests {
		for id := range req.OwnRefs {
			liveRefs[id] = struct{}{}
		}
	}
	expandAnonymousReachability(store, nil, reachable, liveRefs)

	recycleFn, ok := registry.Get("recycle")
	if !ok {
		return
	}

	// Freeze one candidate snapshot at the lowest request floor before invoking
	// any recycle callback. A callback may create and persist a new anonymous
	// object; recomputing candidates for a later request against stale reachability
	// would incorrectly recycle that newly-live object. Filtering this one slice
	// per request preserves request/context order with O(candidate count) memory.
	frozenCandidates := store.AnonymousRecycleCandidates(reachable, minFloor)

	recycleFrozenAnonymousCandidates(requests, frozenCandidates, func(ctx *kernel.TaskContext, id types.ObjID) {
		// Best-effort cleanup: recycle() handles missing/already-invalid objects.
		_ = recycleFn(ctx, []types.Value{types.NewAnon(id)})
	})
}

// recycleFrozenAnonymousCandidates routes one immutable candidate snapshot
// through request contexts in request order. Filtering by each floor and global
// de-duplication reproduce the old per-request behavior without retaining one
// candidate slice per request.
func recycleFrozenAnonymousCandidates(requests []AnonGCRequest, frozenCandidates []types.ObjID, recycle func(*kernel.TaskContext, types.ObjID)) {
	recycled := make(map[types.ObjID]struct{}, len(frozenCandidates))
	for _, req := range requests {
		if req.Ctx == nil {
			continue
		}
		for _, id := range frozenCandidates {
			if id < req.MinID {
				continue
			}
			if _, done := recycled[id]; done {
				continue
			}
			recycled[id] = struct{}{}
			recycle(req.Ctx, id)
		}
	}
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

	// Fast path: recycle candidates are restricted to anonymous objects with
	// ids >= minID, so when the finished task created none the reachability
	// sweep below is a guaranteed no-op. Skipping it matters: the sweep walks
	// every persistent object's property tree, which is prohibitive to pay
	// after every task on a large database.
	if !store.HasAnonymousAtOrAbove(minID) {
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
	expandAnonymousReachability(store, ctx.StoreTxn, reachable, liveRefs)

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
