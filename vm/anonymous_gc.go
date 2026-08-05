package vm

import (
	"barn/builtins"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/types"
	"sort"
	"unsafe"
)

// collectAnonymousRefsForGC finds anonymous object references inside value trees.
func collectAnonymousRefsForGC(v types.Value, out map[types.ObjID]struct{}) {
	collectAnonymousRefsForGCVisited(v, out, nil)
}

func collectAnonymousRefsForGCVisited(v types.Value, out map[types.ObjID]struct{}, visitedWaifs map[unsafe.Pointer]struct{}) {
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
			visitedWaifs = make(map[unsafe.Pointer]struct{})
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
		if taskLocal, ok := taskLocalFromContext(exec.Context); ok {
			collectAnonymousRefsForGC(taskLocal, out)
		}
	}
}

func taskLocalFromContext(ctx *kernel.TaskContext) (types.Value, bool) {
	if ctx == nil {
		return types.None, false
	}
	owner, ok := ctx.Task.(*task.Task)
	if !ok || owner == nil {
		return types.None, false
	}
	return owner.GetTaskLocal(), true
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
		for _, elem := range value.Elements() {
			collectDirectFinalizationRoots(elem, refs, waifs)
		}
	case types.TYPE_MAP:
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
		if taskLocal, ok := taskLocalFromContext(exec.Context); ok {
			collect(taskLocal)
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

func buildPersistentAnonymousReachability(store *dbstore.Store) map[types.ObjID]struct{} {
	if store == nil {
		return map[types.ObjID]struct{}{}
	}
	return store.PersistentAnonymousReachability()
}

func pendingFinalizationValues(refs map[types.ObjID]struct{}, waifs []types.Value) []types.Value {
	if len(refs) == 0 && len(waifs) == 0 {
		return nil
	}

	ids := make([]types.ObjID, 0, len(refs))
	for id := range refs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	values := make([]types.Value, 0, len(ids)+len(waifs))
	for _, id := range ids {
		values = append(values, types.NewAnon(id))
	}
	values = append(values, waifs...)
	return values
}

func expandAnonymousReachability(store *dbstore.Store, reachable map[types.ObjID]struct{}, refs map[types.ObjID]struct{}) {
	if store != nil {
		store.ExpandAnonymousReachability(reachable, refs)
	}
}

// CollectPendingFinalizationValues snapshots anonymous-object references held by
// a live VM. Every directly referenced identity is retained in deterministic
// order, including references nested in lists and maps. Collection deliberately
// does not inspect the Store graph: callers can commit staged edge removals after
// this function returns, and concurrent workers can mutate that graph. The Store
// snapshot later traverses all pending roots and their descendants under one lock.
func CollectPendingFinalizationValues(store *dbstore.Store, exec *VM) []types.Value {
	if store == nil || exec == nil {
		return nil
	}

	refs := make(map[types.ObjID]struct{})
	var waifs []types.Value
	collectDirectFinalizationRootsFromVM(exec, refs, &waifs)
	return pendingFinalizationValues(refs, waifs)
}

// TakePendingFinalizationValues returns the finalizable identities retained at
// frame-pop boundaries plus identities still live in the VM/task-local owner.
// It is used by terminal shutdown handoff, after the last activation may already
// have been removed from Frames.
func (vm *VM) TakePendingFinalizationValues() []types.Value {
	if vm == nil {
		return nil
	}
	values := CollectPendingFinalizationValues(vm.Store, vm)
	vm.PendingFinalizations = nil
	return values
}

func (vm *VM) collectPendingFinalizationsFromFrame(frame *StackFrame) {
	if frame == nil {
		return
	}
	refs := make(map[types.ObjID]struct{})
	var waifs []types.Value
	collect := func(value types.Value) { collectDirectFinalizationRoots(value, refs, &waifs) }
	for _, value := range frame.Locals {
		collect(value)
	}
	collect(frame.ThisValue)
	for _, value := range frame.Args {
		collect(value)
	}
	collect(frame.SavedThisValue)
	collectDirectFinalizationRootsFromPendingError(frame.PendingError, refs, &waifs)

	for _, value := range pendingFinalizationValues(refs, waifs) {
		if !pendingFinalizationValueInList(value, vm.PendingFinalizations) {
			vm.PendingFinalizations = append(vm.PendingFinalizations, value)
		}
	}
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
			if needle.WaifIdentity() == candidate.WaifIdentity() {
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
	Ctx       *kernel.TaskContext
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
