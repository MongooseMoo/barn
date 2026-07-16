package scheduler

import (
	"time"

	"barn/compiler"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/metrics"
	"barn/types"
	"barn/vm"
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

func (s *Scheduler) liveWaifs(siblingWaifs []types.Value, rootVMs ...*vm.VM) []types.Value {
	roots := s.store.PersistentWaifRoots()
	roots = append(roots, siblingWaifs...)
	for _, exec := range rootVMs {
		vm.CollectWaifsFromVM(exec, &roots)
	}
	return roots
}

// finalizePendingWaifs recycles the task's pending waifs that nothing still
// references. siblingWaifs are waif references already snapshotted from other tasks'
// VMs under the scheduler lock; rootVMs are this goroutine's own VMs.
func (s *Scheduler) finalizePendingWaifs(ctx *kernel.TaskContext, pending []types.Value, siblingWaifs []types.Value, rootVMs ...*vm.VM) {
	if len(pending) == 0 || ctx == nil {
		return
	}

	live := s.liveWaifs(siblingWaifs, rootVMs...)
	for _, waif := range pending {
		if waifInList(waif, live) {
			continue
		}
		s.callWaifRecycle(ctx, waif)
	}
}

// pendingWaifEntry is one waif awaiting a deferred liveness check, together with
// the task context it was pending under (used for :recycle perms) and the waif
// references its own task's VM held at defer time. ownRefs is captured at defer
// time, by the goroutine owning that VM, because the VM is released once the task
// completes and so cannot be walked when the flush finally runs.
type pendingWaifEntry struct {
	waif    types.Value
	ctx     *kernel.TaskContext
	ownRefs []types.Value
}

const (
	gcSweepInterval = 2 * time.Second
	// While sweeps stay cheaper than this, the deferred batches are flushed on
	// every scheduler pass (matching the old per-task promptness on small
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
func (s *Scheduler) deferPendingWaifs(ctx *kernel.TaskContext, pending []types.Value, ownVM *vm.VM) {
	if len(pending) == 0 || ctx == nil {
		return
	}
	var ownRefs []types.Value
	if ownVM != nil {
		vm.CollectWaifsFromVM(ownVM, &ownRefs)
	}
	s.pendingWaifMu.Lock()
	for _, waif := range pending {
		s.pendingWaifBatch = append(s.pendingWaifBatch, pendingWaifEntry{waif: waif, ctx: ctx, ownRefs: ownRefs})
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
func (s *Scheduler) deferAnonGC(ctx *kernel.TaskContext, minID types.ObjID, ownVM *vm.VM) {
	if ctx == nil {
		return
	}
	var ownRefs map[types.ObjID]struct{}
	if ownVM != nil {
		ownRefs = make(map[types.ObjID]struct{})
		vm.CollectAnonymousRefsFromVM(ownVM, ownRefs)
	}
	s.pendingWaifMu.Lock()
	s.pendingAnonGC = append(s.pendingAnonGC, vm.AnonGCRequest{Ctx: s.gcRecycleContext(ctx), MinID: minID, OwnRefs: ownRefs})
	s.pendingWaifMu.Unlock()
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
func (s *Scheduler) gcRecycleContext(parent *kernel.TaskContext) *kernel.TaskContext {
	gcCtx := kernel.NewTaskContext()
	gcCtx.Player = parent.Player
	gcCtx.Programmer = parent.Programmer
	gcCtx.IsWizard = parent.IsWizard
	gcCtx.ThisObj = parent.ThisObj
	gcCtx.Task = parent.Task
	gcCtx.TaskID = parent.TaskID
	gcCtx.Store = s.store
	gcCtx.Registry = s.registry
	gcCtx.RuntimeOptions = s.options
	return gcCtx
}

// flushDeferredGC settles the deferred waif and anonymous-object batches.
// Called from the scheduler loop after every task pass. Liveness is judged at
// flush time against persistent state plus all live task VMs, so deferral
// only changes WHEN an orphan's :recycle runs: immediately after the pass
// while sweeps stay cheap, on gcSweepInterval once they become expensive.
func (s *Scheduler) flushDeferredGC() {
	s.pendingWaifMu.Lock()
	if len(s.pendingWaifBatch) == 0 && len(s.pendingAnonGC) == 0 {
		s.pendingWaifMu.Unlock()
		return
	}
	due := time.Since(s.lastGCSweep) >= gcSweepInterval
	if !due && s.lastGCCost >= cheapGCSweep {
		s.pendingWaifMu.Unlock()
		return
	}
	s.pendingWaifMu.Unlock()

	// Gather roots before draining, and only from a quiescent scheduler: a running
	// task's VM holds roots but cannot be read without racing it. If one is running,
	// leave the batches queued — deferral only changes WHEN an orphan's :recycle
	// runs, never whether, and the end-of-pass flush will settle them.
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
	s.pendingWaifMu.Unlock()

	sweepStart := time.Now()

	if len(waifBatch) > 0 {
		roots := append([]types.Value(nil), siblingWaifs...)
		for _, entry := range waifBatch {
			roots = append(roots, entry.ownRefs...)
		}
		live := s.liveWaifs(roots)
		for _, entry := range waifBatch {
			if waifInList(entry.waif, live) {
				continue
			}
			s.callWaifRecycle(entry.ctx, entry.waif)
		}
	}

	vm.RecycleOrphanAnonymousBatch(s.store, s.registry, anonBatch, siblingAnon)

	cost := time.Since(sweepStart)
	metrics.GCSweeps.Add(1)
	metrics.GCSweepLastMs.Set(cost.Milliseconds())

	s.pendingWaifMu.Lock()
	s.lastGCCost = cost
	s.pendingWaifMu.Unlock()
}

func (s *Scheduler) callWaifRecycle(parentCtx *kernel.TaskContext, waif types.Value) {
	verb, defObjID, err := s.store.FindVerb(waif.Class(), ":recycle")
	if err != nil {
		return
	}
	if !verb.Perms.Has(dbstore.VerbExecute) {
		return
	}

	prog, diagnostics := compiler.CompileMOO(verb.Code, s.registry)
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
	recycleCtx.Task = parentCtx.Task
	recycleCtx.TaskID = parentCtx.TaskID
	recycleCtx.Store = s.store
	recycleCtx.Registry = s.registry
	recycleCtx.RuntimeOptions = s.options

	recycleVM := vm.NewVM(s.store, s.registry)
	recycleVM.Context = recycleCtx
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
	_ = recycleVM.ExecuteLoop()
}
