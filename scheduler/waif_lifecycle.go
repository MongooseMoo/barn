package scheduler

import (
	"barn/bytecode"
	dbstore "barn/db/store"
	"barn/kernel"
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

func (s *Scheduler) callWaifRecycle(parentCtx *kernel.TaskContext, waif types.Value) {
	verb, defObjID, err := s.store.FindVerb(waif.Class(), ":recycle")
	if err != nil {
		return
	}
	if !verb.Perms.Has(dbstore.VerbExecute) {
		return
	}

	prog, compileErr := bytecode.CompileVerbBytecode(verb.Code, s.registry)
	if compileErr != nil {
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
