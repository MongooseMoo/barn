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

func (s *Scheduler) liveWaifs(rootVMs ...*vm.VM) []types.Value {
	roots := s.store.PersistentWaifRoots()
	for _, exec := range rootVMs {
		vm.CollectWaifsFromVM(exec, &roots)
	}
	return roots
}

func (s *Scheduler) finalizePendingWaifs(ctx *kernel.TaskContext, pending []types.Value, rootVMs ...*vm.VM) {
	if len(pending) == 0 || ctx == nil {
		return
	}

	live := s.liveWaifs(rootVMs...)
	for _, waif := range pending {
		if waifInList(waif, live) {
			continue
		}
		s.callWaifRecycle(ctx, waif)
	}
}

// pendingWaifEntry is one waif awaiting a deferred liveness check, together
// with the task context it was pending under (used for :recycle perms).
type pendingWaifEntry struct {
	waif types.Value
	ctx  *kernel.TaskContext
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
func (s *Scheduler) deferPendingWaifs(ctx *kernel.TaskContext, pending []types.Value) {
	if len(pending) == 0 || ctx == nil {
		return
	}
	s.pendingWaifMu.Lock()
	for _, waif := range pending {
		s.pendingWaifBatch = append(s.pendingWaifBatch, pendingWaifEntry{waif: waif, ctx: ctx})
	}
	s.pendingWaifMu.Unlock()
}

// deferAnonGC queues an orphan-anonymous collection request for the next
// deferred-GC flush, for the same reason as deferPendingWaifs: the per-task
// sweep walks every persistent property tree and is prohibitive on large
// databases whose worlds create anonymous objects on nearly every task.
func (s *Scheduler) deferAnonGC(ctx *kernel.TaskContext, minID types.ObjID) {
	if ctx == nil {
		return
	}
	s.pendingWaifMu.Lock()
	s.pendingAnonGC = append(s.pendingAnonGC, vm.AnonGCRequest{Ctx: ctx, MinID: minID})
	s.pendingWaifMu.Unlock()
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
	waifBatch := s.pendingWaifBatch
	anonBatch := s.pendingAnonGC
	s.pendingWaifBatch = nil
	s.pendingAnonGC = nil
	s.lastGCSweep = time.Now()
	s.pendingWaifMu.Unlock()

	sweepStart := time.Now()
	liveVMs := s.liveTaskVMs(nil)

	if len(waifBatch) > 0 {
		live := s.liveWaifs(liveVMs...)
		for _, entry := range waifBatch {
			if waifInList(entry.waif, live) {
				continue
			}
			s.callWaifRecycle(entry.ctx, entry.waif)
		}
	}

	vm.RecycleOrphanAnonymousBatch(s.store, s.registry, anonBatch, liveVMs...)

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
