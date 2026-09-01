package vm

import (
	"sort"

	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// collectAnonymousRefsForGC finds anonymous object references inside value trees.
func collectAnonymousRefsForGC(v types.Value, out map[types.ObjID]struct{}) {
	collectAnonymousRefsForGCVisited(v, out, nil)
}

func collectAnonymousRefsForGCVisited(v types.Value, out map[types.ObjID]struct{}, visitedWaifs map[types.WaifIdentity]struct{}) {
	switch v.Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		if v.IsAnonymous() {
			out[v.ID()] = struct{}{}
		}
	case types.TYPE_WAIF:
		identity := v.WaifIdentity()
		if _, seen := visitedWaifs[identity]; seen {
			return
		}
		if visitedWaifs == nil {
			visitedWaifs = make(map[types.WaifIdentity]struct{})
		}
		visitedWaifs[identity] = struct{}{}
		for _, name := range v.PropertyNames() {
			if prop, ok := v.GetProperty(name); ok {
				collectAnonymousRefsForGCVisited(prop, out, visitedWaifs)
			}
		}
	case types.TYPE_LIST:
		for _, elem := range v.Elements() {
			collectAnonymousRefsForGCVisited(elem, out, visitedWaifs)
		}
	case types.TYPE_MAP:
		for _, pair := range v.Pairs() {
			collectAnonymousRefsForGCVisited(pair[0], out, visitedWaifs)
			collectAnonymousRefsForGCVisited(pair[1], out, visitedWaifs)
		}
	}
}

// CollectAnonymousRefsFromVM gathers the anonymous-object IDs reachable from a VM's
// frames and stack into out. It touches only VM state (no Store lock), so callers may
// run it while holding the engine lock to snapshot a sibling task's references
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
		collectAnonymousRefsForGC(frame.ThisValue, out)
		for _, value := range frame.Args {
			collectAnonymousRefsForGC(value, out)
		}
		collectAnonymousRefsForGC(frame.SavedThisValue, out)
		collectAnonymousRefsFromPendingError(frame.PendingError, out)
	}
	for i := 0; i < exec.SP && i < len(exec.Stack); i++ {
		collectAnonymousRefsForGC(exec.Stack[i], out)
	}
	for _, value := range exec.PendingWaifs {
		collectAnonymousRefsForGC(value, out)
	}
	collectAnonymousRefsForGC(exec.yieldResult.Val, out)
	if fork := exec.yieldResult.ForkInfo; fork != nil {
		collectAnonymousRefsForGC(fork.ThisValue, out)
		for _, value := range fork.Variables {
			collectAnonymousRefsForGC(value, out)
		}
	}
	if exec.Context != nil {
		collectAnonymousRefsForGC(exec.Context.ThisValue, out)
		collectAnonymousRefsForGC(exec.Context.MapFirstKey, out)
		collectAnonymousRefsForGC(exec.Context.MapLastKey, out)
		collectAnonymousRefsForGC(exec.Context.TaskLocal, out)
		if exec.Task != nil {
			collectAnonymousRefsForGC(exec.Task.GetTaskLocal(), out)
		}
	}
}

func collectAnonymousRefsFromPendingError(err error, out map[types.ObjID]struct{}) {
	for err != nil {
		switch pending := err.(type) {
		case VMException:
			collectAnonymousRefsForGC(pending.Value, out)
			return
		case *VMException:
			collectAnonymousRefsForGC(pending.Value, out)
			return
		case interface{ Unwrap() error }:
			err = pending.Unwrap()
		default:
			return
		}
	}
}

func collectDirectFinalizationRoots(value types.Value, refs map[types.ObjID]struct{}, waifs *[]types.Value) {
	switch value.Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		if value.IsAnonymous() {
			refs[value.ID()] = struct{}{}
		}
	case types.TYPE_WAIF:
		if !pendingFinalizationValueInList(value, *waifs) {
			*waifs = append(*waifs, value)
		}
	case types.TYPE_LIST:
		if !value.MayHoldFinalizable() {
			return
		}
		for _, elem := range value.Elements() {
			collectDirectFinalizationRoots(elem, refs, waifs)
		}
	case types.TYPE_MAP:
		if !value.MayHoldFinalizable() {
			return
		}
		for _, pair := range value.Pairs() {
			collectDirectFinalizationRoots(pair[0], refs, waifs)
			collectDirectFinalizationRoots(pair[1], refs, waifs)
		}
	}
}

func collectDirectFinalizationRootsFromVM(exec *VM, refs map[types.ObjID]struct{}, waifs *[]types.Value) {
	if exec == nil {
		return
	}
	collect := func(value types.Value) { collectDirectFinalizationRoots(value, refs, waifs) }
	for _, frame := range exec.Frames {
		if frame == nil {
			continue
		}
		for _, value := range frame.Locals {
			collect(value)
		}
		collect(frame.ThisValue)
		for _, value := range frame.Args {
			collect(value)
		}
		collect(frame.SavedThisValue)
		collectDirectFinalizationRootsFromPendingError(frame.PendingError, refs, waifs)
	}
	for i := 0; i < exec.SP && i < len(exec.Stack); i++ {
		collect(exec.Stack[i])
	}
	for _, value := range exec.PendingWaifs {
		collect(value)
	}
	for _, value := range exec.PendingFinalizations {
		collect(value)
	}
	collect(exec.yieldResult.Val)
	if fork := exec.yieldResult.ForkInfo; fork != nil {
		collect(fork.ThisValue)
		for _, value := range fork.Variables {
			collect(value)
		}
	}
	if exec.Context != nil {
		collect(exec.Context.ThisValue)
		collect(exec.Context.MapFirstKey)
		collect(exec.Context.MapLastKey)
		collect(exec.Context.TaskLocal)
		if exec.Task != nil {
			collect(exec.Task.GetTaskLocal())
		}
	}
}

func collectDirectFinalizationRootsFromPendingError(err error, refs map[types.ObjID]struct{}, waifs *[]types.Value) {
	for err != nil {
		switch pending := err.(type) {
		case VMException:
			collectDirectFinalizationRoots(pending.Value, refs, waifs)
			return
		case *VMException:
			collectDirectFinalizationRoots(pending.Value, refs, waifs)
			return
		case interface{ Unwrap() error }:
			err = pending.Unwrap()
		default:
			return
		}
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
		store.DirectTxn().ExpandAnonymousReachability(reachable, refs)
	}
	if !tx.IsDirect() {
		// Add the owning task's in-flight property graph without replacing the
		// live expansion: refs from sibling VMs may postdate this transaction's
		// snapshot and must remain protected by the live-store walk.
		tx.ExpandAnonymousReachability(reachable, refs)
	}
}

// CollectPendingFinalizationValues snapshots direct anonymous and WAIF identities
// held by a live VM. Nested graphs are reduced to one covering root so a child
// WAIF retained by its parent is not promoted to a second top-level finalization.
func CollectPendingFinalizationValues(store *dbstore.Store, exec *VM) []types.Value {
	if store == nil || exec == nil {
		return nil
	}

	refs := make(map[types.ObjID]struct{})
	var waifs []types.Value
	collectDirectFinalizationRootsFromVM(exec, refs, &waifs)
	candidates := pendingFinalizationValues(store, refs)
	type candidateRoot struct {
		value   types.Value
		closure map[types.ObjID]struct{}
	}
	ordered := make([]candidateRoot, 0, len(candidates))
	for _, candidate := range candidates {
		closure := make(map[types.ObjID]struct{})
		store.DirectTxn().ExpandAnonymousReachability(closure, map[types.ObjID]struct{}{
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
	roots := canonicalWaifRoots(waifs, store.PersistentWaifRoots())
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

func canonicalWaifRoots(candidates []types.Value, persistent []types.Value) []types.Value {
	var persistentClosure []types.Value
	for _, root := range persistent {
		collectWaifsForGC(root, &persistentClosure)
	}
	type candidateRoot struct {
		value   types.Value
		closure []types.Value
		order   int
	}
	ordered := make([]candidateRoot, 0, len(candidates))
	for index, candidate := range candidates {
		if candidate.Type() != types.TYPE_WAIF || waifValueInListInternal(candidate, persistentClosure) {
			continue
		}
		var closure []types.Value
		collectWaifsForGC(candidate, &closure)
		ordered = append(ordered, candidateRoot{value: candidate, closure: closure, order: index})
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if len(ordered[i].closure) != len(ordered[j].closure) {
			return len(ordered[i].closure) > len(ordered[j].closure)
		}
		return ordered[i].order < ordered[j].order
	})
	covered := append([]types.Value(nil), persistentClosure...)
	roots := make([]types.Value, 0, len(ordered))
	for _, candidate := range ordered {
		if waifValueInListInternal(candidate.value, covered) {
			continue
		}
		roots = append(roots, candidate.value)
		for _, value := range candidate.closure {
			if !waifValueInListInternal(value, covered) {
				covered = append(covered, value)
			}
		}
	}
	return roots
}

func waifValueInListInternal(needle types.Value, values []types.Value) bool {
	for _, value := range values {
		if value.Type() == types.TYPE_WAIF && value.Equal(needle) {
			return true
		}
	}
	return false
}

func (vm *VM) collectPendingFinalizationsFromFrame(frame *StackFrame) {
	if frame == nil {
		return
	}
	for _, value := range frame.Locals {
		vm.collectPendingFinalizationsFromValue(value)
	}
	vm.collectPendingFinalizationsFromValue(frame.ThisValue)
	for _, value := range frame.Args {
		vm.collectPendingFinalizationsFromValue(value)
	}
	vm.collectPendingFinalizationsFromValue(frame.SavedThisValue)
	refs := make(map[types.ObjID]struct{})
	var waifs []types.Value
	collectDirectFinalizationRootsFromPendingError(frame.PendingError, refs, &waifs)
	vm.appendPendingFinalizationRoots(refs, waifs)
}

func (vm *VM) collectPendingFinalizationsFromValue(value types.Value) {
	if !value.MayHoldFinalizable() {
		return
	}
	refs := make(map[types.ObjID]struct{})
	var waifs []types.Value
	collectDirectFinalizationRoots(value, refs, &waifs)
	vm.appendPendingFinalizationRoots(refs, waifs)
}

func (vm *VM) appendPendingFinalizationRoots(refs map[types.ObjID]struct{}, waifs []types.Value) {
	for id := range refs {
		value := types.NewAnon(id)
		if !pendingFinalizationValueInList(value, vm.PendingFinalizations) {
			vm.PendingFinalizations = append(vm.PendingFinalizations, value)
		}
	}
	for _, value := range waifs {
		if !pendingFinalizationValueInList(value, vm.PendingFinalizations) {
			vm.PendingFinalizations = append(vm.PendingFinalizations, value)
		}
	}
}

// TakePendingFinalizationValues returns the canonical roots retained across
// frame-pop boundaries together with values still live in the VM.
func (vm *VM) TakePendingFinalizationValues() []types.Value {
	if vm == nil {
		return nil
	}
	values := CollectPendingFinalizationValues(vm.Store, vm)
	vm.PendingFinalizations = nil
	return values
}

func pendingFinalizationValueInList(needle types.Value, values []types.Value) bool {
	for _, candidate := range values {
		if needle.Type() != candidate.Type() {
			continue
		}
		switch needle.Type() {
		case types.TYPE_ANON:
			if needle.ID() == candidate.ID() {
				return true
			}
		case types.TYPE_WAIF:
			if needle.Equal(candidate) {
				return true
			}
		default:
			if needle.Equal(candidate) {
				return true
			}
		}
	}
	return false
}

// AutoRecycleOrphanAnonymousWith recycles anonymous objects that are not reachable
// from any persistent non-anonymous object's properties.
func AutoRecycleOrphanAnonymousWith(store *dbstore.Store, session *builtins.Session, ctx *kernel.TaskContext) {
	AutoRecycleOrphanAnonymousSince(store, session, session.NewExecution(ctx, nil), 0, nil)
}

// AnonGCRequest is one deferred orphan-anonymous collection request: recycle
// anonymous objects with ids >= MinID that are unreachable, using Ctx for the
// recycle() calls. OwnRefs holds the anonymous ids the requesting task's own VM
// referenced, snapshotted at defer time by the goroutine that owned that VM. A
// completed task's VM is released before the flush runs, so its locals cannot be
// walked then; capturing the ids up front keeps them as roots without retaining
// the *VM (which a concurrent flush must never touch).
type AnonGCRequest struct {
	Ctx       *kernel.TaskContext
	Task      *task.Task
	MinID     types.ObjID
	OwnRefs   map[types.ObjID]struct{}
	TaskOwned bool
}

// RecycleOrphanAnonymousBatch settles several deferred collection requests
// with a single persistent-reachability build. Per-task collection pays a
// full-database property sweep per finished task, which is prohibitive on
// large databases; batching preserves the liveness check (reachability plus
// every live task's VM references at flush time) and only delays when an orphan
// is recycled.
//
// siblingRefs holds the anonymous ids snapshotted from every live task's VM under
// the engine lock. Together with each request's OwnRefs it covers the same root
// set the inline per-task sweep saw, without walking a *VM here — so a task running
// concurrently on another goroutine is never read.
func RecycleOrphanAnonymousBatch(store *dbstore.Store, session *builtins.Session, requests []AnonGCRequest, siblingRefs map[types.ObjID]struct{}) {
	if store == nil || session == nil || len(requests) == 0 {
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
	expandAnonymousReachability(store, store.DirectTxn(), reachable, liveRefs)

	recycleFn, ok := session.Registry().Get("recycle")
	if !ok {
		return
	}

	// Freeze one candidate snapshot at the lowest request floor before invoking
	// any recycle callback. A callback may create and persist a new anonymous
	// object; recomputing candidates for a later request against stale reachability
	// would incorrectly recycle that newly-live object. Filtering this one slice
	// per request preserves request/context order with O(candidate count) memory.
	frozenCandidates := store.AnonymousRecycleCandidates(reachable, minFloor)

	recycleFrozenAnonymousCandidates(requests, frozenCandidates, func(request AnonGCRequest, id types.ObjID) {
		// Best-effort cleanup: recycle() handles missing/already-invalid objects.
		_ = recycleFn(session.NewExecution(request.Ctx, request.Task), []types.Value{types.NewAnon(id)})
	})
}

// recycleFrozenAnonymousCandidates routes one immutable candidate snapshot
// through request contexts in request order. Filtering by each floor and global
// de-duplication reproduce the old per-request behavior without retaining one
// candidate slice per request.
func recycleFrozenAnonymousCandidates(requests []AnonGCRequest, frozenCandidates []types.ObjID, recycle func(AnonGCRequest, types.ObjID)) {
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
			recycle(req, id)
		}
	}
}

// AutoRecycleOrphanAnonymousSince performs orphan-anonymous collection but only
// recycles anonymous objects with IDs >= minID. This lets task/eval callers
// collect objects created during the current execution without sweeping
// pre-existing database state.
// siblingRefs holds anonymous IDs already collected from other tasks' VMs (under the
// engine lock, so they were snapshotted without racing those tasks). localVMs are
// VMs owned by the calling goroutine (this task's own VM), safe to walk here.
func AutoRecycleOrphanAnonymousSince(store *dbstore.Store, session *builtins.Session, execution *builtins.Execution, minID types.ObjID, siblingRefs map[types.ObjID]struct{}, localVMs ...*VM) {
	if execution == nil || execution.TaskContext == nil || store == nil || session == nil {
		return
	}
	ctx := execution.TaskContext

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
	if execution.CollectAnonymousRefs != nil {
		execution.CollectAnonymousRefs(liveRefs)
	}
	for _, exec := range localVMs {
		collectAnonymousRefsFromVM(exec, liveRefs)
	}
	expandAnonymousReachability(store, ctx.StoreTxn, reachable, liveRefs)

	candidates := store.AnonymousRecycleCandidates(reachable, minID)
	if len(candidates) == 0 {
		return
	}

	recycleFn, ok := session.Registry().Get("recycle")
	if !ok {
		return
	}

	for _, id := range candidates {
		// Best-effort cleanup: recycle() handles missing/already-invalid objects.
		_ = recycleFn(execution, []types.Value{types.NewAnon(id)})
	}
}
