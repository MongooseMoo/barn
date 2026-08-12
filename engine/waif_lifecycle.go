package engine

import (
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
	"sort"
	"time"
)

func sameWaif(a, b types.Value) bool {
	return a.Equal(b)
}

func waifInList(needle types.Value, haystack []types.Value) bool {
	for _, candidate := range haystack {
		if sameWaif(needle, candidate) {
			return true
		}
	}
	return false
}

func (s *Runtime) liveWaifs(siblingWaifs []types.Value, rootVMs ...*vm.VM) []types.Value {
	roots := s.store.PersistentWaifRoots()
	roots = append(roots, siblingWaifs...)
	for _, exec := range rootVMs {
		vm.CollectWaifsFromVM(exec, &roots)
	}
	return roots
}

// finalizePendingWaifs recycles the task's pending waifs that nothing still
// references. siblingWaifs are waif references already snapshotted from other tasks'
// VMs under the runtime lock; rootVMs are this goroutine's own VMs.
func (s *Runtime) finalizePendingWaifs(ctx *kernel.TaskContext, pending []types.Value, siblingWaifs []types.Value, rootVMs ...*vm.VM) {
	if len(pending) == 0 || ctx == nil {
		return
	}

	live := s.liveWaifs(siblingWaifs, rootVMs...)
	var owner *task.Task
	if len(rootVMs) > 0 && rootVMs[0] != nil {
		owner = rootVMs[0].Task
	}
	for _, waif := range pending {
		if waifInList(waif, live) {
			continue
		}
		s.callWaifRecycle(ctx, owner, waif)
	}
}

// pendingWaifEntry is one waif awaiting a deferred liveness check, together with
// the task context it was pending under (used for :recycle perms) and the waif
// references its own task's VM held at defer time. ownRefs is captured at defer
// time, by the goroutine owning that VM, because the VM is released once the task
// completes and so cannot be walked when the flush finally runs.
type pendingWaifEntry struct {
	waif          types.Value
	ctx           *kernel.TaskContext
	task          *task.Task
	ownRefs       []types.Value
	shutdownRoots []types.Value
}

const (
	gcSweepInterval = 2 * time.Second
	// While sweeps stay cheaper than this, the deferred batches are flushed on
	// every runtime pass (matching the old per-task promptness on small
	// databases); once a sweep costs more, flushes throttle to gcSweepInterval.
	cheapGCSweep = 50 * time.Millisecond
)

// deferPendingWaifs queues waifs for batched finalization. The per-task
// finalizePendingWaifs pays a full-database waif-roots sweep on every call,
// which is prohibitive on large databases where busy worlds surface pending
// waifs after nearly every task. Deferral only delays when an orphaned
// waif's :recycle runs (by up to waifSweepInterval); a waif that is still
// referenced anywhere is skipped at flush time exactly as before.
//
// ownVM is the deferring task's own VM, walked here on the goroutine that owns it
// so its references survive as roots after the VM is released.
func (s *Runtime) deferPendingWaifs(ctx *kernel.TaskContext, pending []types.Value, ownVM *vm.VM) {
	if len(pending) == 0 || ctx == nil {
		return
	}
	var ownRefs []types.Value
	if ownVM != nil {
		vm.CollectWaifsFromVM(ownVM, &ownRefs)
	}
	shutdownRoots := vm.CollectPendingFinalizationValues(s.store, ownVM)
	s.pendingWaifMu.Lock()
	if s.shutdownRequested {
		published := s.shutdownPublished
		if !published {
			s.pendingShutdownRoots = append(s.pendingShutdownRoots, shutdownRoots...)
		}
		s.pendingWaifMu.Unlock()
		if published {
			s.appendPendingFinalizations(shutdownRoots)
		}
		return
	}
	for _, waif := range pending {
		var owner *task.Task
		if ownVM != nil {
			owner = ownVM.Task
		}
		s.pendingWaifBatch = append(s.pendingWaifBatch, pendingWaifEntry{
			waif: waif, ctx: ctx, task: owner, ownRefs: ownRefs, shutdownRoots: shutdownRoots,
		})
	}
	s.pendingWaifMu.Unlock()
}

// deferAnonGC queues an orphan-anonymous collection request for the next
// deferred-GC flush, for the same reason as deferPendingWaifs: the per-task
// sweep walks every persistent property tree and is prohibitive on large
// databases whose worlds create anonymous objects on nearly every task.
//
// ownVM is the deferring task's own VM (nil when the task is suspending, since its
// VM stays registered and the flush reads it directly). When non-nil it is walked
// here, on the goroutine that owns it, so its references survive the VM's release.
func (s *Runtime) deferAnonGC(ctx *kernel.TaskContext, minID types.ObjID, ownVM *vm.VM) {
	if ctx == nil {
		return
	}
	var ownRefs map[types.ObjID]struct{}
	var owner *task.Task
	if ownVM != nil {
		ownRefs = make(map[types.ObjID]struct{})
		vm.CollectAnonymousRefsFromVM(ownVM, ownRefs)
		owner = ownVM.Task
	}
	s.pendingWaifMu.Lock()
	if s.shutdownRequested {
		roots := vm.CollectPendingFinalizationValues(s.store, ownVM)
		published := s.shutdownPublished
		if ownVM != nil && !published {
			s.pendingShutdownRoots = append(s.pendingShutdownRoots, roots...)
		}
		s.pendingWaifMu.Unlock()
		if ownVM != nil && published {
			s.appendPendingFinalizations(roots)
		}
		return
	}
	s.pendingAnonGC = append(s.pendingAnonGC, vm.AnonGCRequest{
		Ctx: s.gcRecycleContext(ctx), MinID: minID, OwnRefs: ownRefs, TaskOwned: ownVM == nil, Task: owner,
	})
	s.pendingWaifMu.Unlock()
}

// settleCompletedTaskFinalizations chooses exactly one owner for a terminal
// task's roots. An explicit shutdown transfers them to the checkpoint domain;
// generic cancellation and normal completion retain the ordinary GC path.
func (s *Runtime) settleCompletedTaskFinalizations(ctx *kernel.TaskContext, exec *vm.VM, minID types.ObjID, hasAnonymousCreations bool) bool {
	if exec == nil {
		return s.ShutdownRequested()
	}
	s.pendingWaifMu.Lock()
	shutdown := s.shutdownRequested
	s.pendingWaifMu.Unlock()
	if shutdown {
		roots := exec.TakePendingFinalizationValues()
		s.pendingWaifMu.Lock()
		published := s.shutdownPublished
		if !published {
			s.pendingShutdownRoots = append(s.pendingShutdownRoots, roots...)
		}
		s.pendingWaifMu.Unlock()
		if published {
			s.appendPendingFinalizations(roots)
		}
		return true
	}
	s.deferPendingWaifs(ctx, exec.TakePendingWaifs(), exec)
	if hasAnonymousCreations {
		s.deferAnonGC(ctx, minID, exec)
	}
	return false
}

// gcRecycleContext derives the context an orphan's :recycle runs under at flush
// time. It deliberately carries no store transaction: when collection ran inline
// the :recycle writes joined the task's transaction and were committed with it,
// but a deferred sweep runs after that transaction has committed and been
// released — writing through it would silently discard the recycle's side effects
// (a :recycle that bumps a counter would never be observed). Writing straight to
// the live store instead is what callWaifRecycle already does for waifs.
//
// The task's own context is never mutated: the suspend path also defers, and that
// task's transaction is still live.
func (s *Runtime) gcRecycleContext(parent *kernel.TaskContext) *kernel.TaskContext {
	gcCtx := kernel.NewTaskContext()
	gcCtx.Player = parent.Player
	gcCtx.Programmer = parent.Programmer
	gcCtx.IsWizard = parent.IsWizard
	gcCtx.ThisObj = parent.ThisObj
	gcCtx.TaskID = parent.TaskID
	gcCtx.Store = s.store
	gcCtx.RuntimeOptions = s.options
	gcCtx.DeferredGC = true
	return gcCtx
}

// flushDeferredGC settles the deferred waif and anonymous-object batches.
// Called after each runTask releases its physical execution lease, with an
// end-of-runtime-pass call as a fallback. Liveness is judged at flush time
// against persistent state plus all live task VMs, so deferral only changes
// WHEN an orphan's :recycle runs: immediately while sweeps stay cheap, on
// gcSweepInterval once they become expensive.
func (s *Runtime) flushDeferredGC() {
	// Avoid contending on the sweep barrier when there is plainly no work.
	s.pendingWaifMu.Lock()
	if s.shutdownRequested || s.gcRunning || (len(s.pendingWaifBatch) == 0 && len(s.pendingAnonGC) == 0) {
		s.pendingWaifMu.Unlock()
		return
	}
	s.pendingWaifMu.Unlock()

	// Serialize sweeps, then stop new VM starts before testing quiescence. Both
	// locks remain held through root capture, batch drain, and every recycle hook.
	// This makes the captured VM roots valid for the complete sweep.
	s.gcSweepMu.Lock()
	defer s.gcSweepMu.Unlock()
	s.vmStartMu.Lock()
	defer s.vmStartMu.Unlock()

	// Another flush may have settled the batch while this goroutine waited.
	s.pendingWaifMu.Lock()
	if s.shutdownRequested || s.gcRunning || (len(s.pendingWaifBatch) == 0 && len(s.pendingAnonGC) == 0) {
		s.pendingWaifMu.Unlock()
		return
	}
	due := time.Since(s.lastGCSweep) >= gcSweepInterval
	if !due && s.lastGCCost >= cheapGCSweep {
		s.pendingWaifMu.Unlock()
		return
	}
	s.pendingWaifMu.Unlock()

	// A VM that acquired its lease before vmStartMu cannot be inspected. Leave
	// the batches queued; that VM's lifecycle release will retry the flush.
	siblingAnon, siblingWaifs, quiescent := s.collectAllGCRefs()
	if !quiescent {
		return
	}

	s.pendingWaifMu.Lock()
	waifBatch := s.pendingWaifBatch
	anonBatch := s.pendingAnonGC
	s.pendingWaifBatch = nil
	s.pendingAnonGC = nil
	s.lastGCSweep = time.Now()
	s.gcRunning = true
	s.pendingWaifMu.Unlock()

	sweepStart := time.Now()
	defer func() {
		cost := time.Since(sweepStart)
		metrics.GCSweeps.Add(1)
		metrics.GCSweepLastMs.Set(cost.Milliseconds())
		s.pendingWaifMu.Lock()
		s.lastGCCost = cost
		s.gcRunning = false
		publish := s.canPublishShutdownLocked()
		if publish {
			s.shutdownPublishing = true
		}
		s.pendingWaifMu.Unlock()
		if publish {
			s.publishShutdown()
		}
	}()

	if len(waifBatch) > 0 {
		roots := append([]types.Value(nil), siblingWaifs...)
		for _, entry := range waifBatch {
			roots = append(roots, entry.ownRefs...)
		}
		live := s.liveWaifs(roots)
		for index, entry := range waifBatch {
			if waifInList(entry.waif, live) {
				continue
			}
			s.callWaifRecycle(entry.ctx, entry.task, entry.waif)
			s.pendingWaifMu.Lock()
			shutdown := s.shutdownRequested
			if shutdown {
				for _, remaining := range waifBatch[index:] {
					if remaining.ctx != nil && remaining.task != nil {
						s.pendingShutdownRoots = append(s.pendingShutdownRoots, remaining.shutdownRoots...)
					}
				}
			}
			s.pendingWaifMu.Unlock()
			if shutdown {
				break
			}
		}
	}

	if !s.ShutdownRequested() {
		markedContexts := make(map[*kernel.TaskContext]struct{})
		for _, req := range anonBatch {
			if req.Ctx == nil {
				continue
			}
			if _, marked := markedContexts[req.Ctx]; marked {
				continue
			}
			markedContexts[req.Ctx] = struct{}{}
			s.acquireSweepContext(req.Ctx)
		}
		defer func() {
			for ctx := range markedContexts {
				s.releaseSweepContext(ctx)
			}
		}()
		vm.RecycleOrphanAnonymousBatch(s.store, s.registry, anonBatch, siblingAnon)
	} else {
		s.pendingWaifMu.Lock()
		for _, request := range anonBatch {
			if !request.TaskOwned {
				s.pendingShutdownRoots = append(s.pendingShutdownRoots, s.anonymousRequestRootValues(request)...)
			}
		}
		s.pendingWaifMu.Unlock()
	}
}

func anonymousRootValues(refs map[types.ObjID]struct{}) []types.Value {
	ids := make([]types.ObjID, 0, len(refs))
	for id := range refs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	values := make([]types.Value, 0, len(ids))
	for _, id := range ids {
		values = append(values, types.NewAnon(id))
	}
	return values
}

func (s *Runtime) anonymousRequestRootValues(request vm.AnonGCRequest) []types.Value {
	refs := make(map[types.ObjID]struct{}, len(request.OwnRefs))
	for id := range request.OwnRefs {
		refs[id] = struct{}{}
	}
	for _, id := range s.store.AnonymousRecycleCandidates(map[types.ObjID]struct{}{}, request.MinID) {
		refs[id] = struct{}{}
	}
	return anonymousRootValues(refs)
}

func (s *Runtime) takeDeferredFinalizationRootsLocked() []types.Value {
	refs := make(map[types.ObjID]struct{})
	for _, request := range s.pendingAnonGC {
		if request.TaskOwned {
			continue
		}
		for _, value := range s.anonymousRequestRootValues(request) {
			refs[value.ID()] = struct{}{}
		}
	}
	values := anonymousRootValues(refs)
	for _, entry := range s.pendingWaifBatch {
		if !waifInList(entry.waif, values) {
			values = append(values, entry.waif)
		}
	}
	s.pendingWaifBatch = nil
	s.pendingAnonGC = nil
	return values
}

// AdoptPendingFinalizations transfers values loaded from the database into the
// runtime's deferred finalizer. They are no longer checkpoint roots unless a
// new shutdown wins before their recycle begins.
func (s *Runtime) AdoptPendingFinalizations(values []types.Value) {
	if len(values) == 0 {
		return
	}
	var waifs []types.Value
	anons := make(map[types.ObjID]struct{})
	for _, value := range values {
		collectLoadedFinalizationRoots(value, &waifs, anons)
	}
	s.pendingWaifMu.Lock()
	for _, waif := range waifs {
		ctx := kernel.NewTaskContext()
		ctx.Player = waif.Owner()
		ctx.Programmer = waif.Owner()
		ctx.Store = s.store
		ctx.RuntimeOptions = s.options
		s.pendingWaifBatch = append(s.pendingWaifBatch, pendingWaifEntry{waif: waif, ctx: ctx})
	}
	for id := range anons {
		ctx := kernel.NewTaskContext()
		ctx.Store = s.store
		ctx.RuntimeOptions = s.options
		ctx.DeferredGC = true
		s.pendingAnonGC = append(s.pendingAnonGC, vm.AnonGCRequest{Ctx: ctx, MinID: id})
	}
	s.pendingWaifMu.Unlock()
}

func collectLoadedFinalizationRoots(value types.Value, waifs *[]types.Value, anons map[types.ObjID]struct{}) {
	switch value.Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		if value.IsAnonymous() {
			anons[value.ID()] = struct{}{}
		}
	case types.TYPE_WAIF:
		if !waifInList(value, *waifs) {
			*waifs = append(*waifs, value)
		}
	case types.TYPE_LIST:
		for _, element := range value.Elements() {
			collectLoadedFinalizationRoots(element, waifs, anons)
		}
	case types.TYPE_MAP:
		for _, pair := range value.Pairs() {
			collectLoadedFinalizationRoots(pair[0], waifs, anons)
			collectLoadedFinalizationRoots(pair[1], waifs, anons)
		}
	}
}

func (s *Runtime) callWaifRecycle(parentCtx *kernel.TaskContext, parentTask *task.Task, waif types.Value) {
	verb, defObjID, err := s.store.FindVerb(waif.Class(), ":recycle")
	if err != nil {
		return
	}
	if !verb.Perms.Has(dbstore.VerbExecute) {
		return
	}

	prog, diagnostics := s.registry.Compiler().CompileMOOWithKey(verb.Code, verb.CodeKey)
	if len(diagnostics) > 0 {
		return
	}

	player := parentCtx.Player
	if player == types.ObjNothing {
		player = parentCtx.Programmer
	}
	recycleCtx := kernel.NewTaskContext()
	recycleCtx.Player = player
	recycleCtx.Programmer = verb.Owner
	recycleCtx.IsWizard = s.isWizard(verb.Owner)
	recycleCtx.ThisObj = waif.Class()
	recycleCtx.ThisValue = waif
	recycleCtx.Verb = ":recycle"
	recycleCtx.TaskID = parentCtx.TaskID
	recycleCtx.Store = s.store
	recycleCtx.RuntimeOptions = s.options
	recycleCtx.DeferredGC = true

	recycleVM := vm.NewVM(s.store, s.registry)
	recycleVM.Context = recycleCtx
	recycleVM.Task = parentTask
	recycleVM.TickLimit = 300000
	frame := recycleVM.PrepareVerbFrame(prog, waif.Class(), player, parentCtx.ThisObj, ":recycle", defObjID, nil)
	frame.VerbDebug = verb.Perms.Has(dbstore.VerbDebug)
	vm.SetLocalByName(frame, prog, "this", waif)
	vm.SetLocalByName(frame, prog, "player", types.NewObj(player))
	vm.SetLocalByName(frame, prog, "caller", types.NewObj(parentCtx.ThisObj))
	vm.SetLocalByName(frame, prog, "verb", types.NewStr(":recycle"))
	vm.SetLocalByName(frame, prog, "args", types.NewList([]types.Value{}))
	vm.SetLocalByName(frame, prog, "argstr", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "dobjstr", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "iobjstr", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "prepstr", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "dobj", types.NewObj(types.ObjNothing))
	vm.SetLocalByName(frame, prog, "iobj", types.NewObj(types.ObjNothing))
	s.acquireSweepContext(recycleCtx)
	defer s.releaseSweepContext(recycleCtx)
	_ = recycleVM.ExecuteLoop()
}
