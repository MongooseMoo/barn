package scheduler

import (
	"fmt"
	"time"

	"barn/bytecode"
	"barn/compiler"
	dbstore "barn/db/store"
	"barn/task"
	"barn/types"
)

// RunStartupPendingFinalizations resumes the finalization queue loaded from a
// checkpoint before the host opens listeners. Each root runs as a registered
// scheduler task, so lifecycle builtins use the same task and shutdown
// semantics as ordinary MOO execution.
func (s *Scheduler) RunStartupPendingFinalizations() error {
	roots := s.store.TakePendingFinalizations()
	if len(roots) == 0 {
		return nil
	}
	if !s.beginFinalizationProducer() {
		s.store.AppendPendingFinalizations(roots)
		return fmt.Errorf("shutdown already published before startup finalization")
	}
	defer s.finishFinalizationProducer()

	for i, root := range roots {
		if err := s.runStartupFinalization(root); err != nil {
			// Preserve the failed root and every root not yet attempted. The
			// caller fails startup, and the next checkpoint/restart can retry.
			s.store.AppendPendingFinalizations(roots[i:])
			return err
		}
	}
	return nil
}

func (s *Scheduler) runStartupFinalization(root types.Value) error {
	var (
		programmer types.ObjID
		program    = (*bytecode.Program)(nil)
		verbName   string
		verbLoc    types.ObjID
		thisObj    types.ObjID
	)

	switch root.Type() {
	case types.TYPE_WAIF:
		thisObj = root.Class()
		verb, defObjID, err := s.store.FindVerb(thisObj, ":recycle")
		if err != nil || !verb.Perms.Has(dbstore.VerbExecute) {
			return nil
		}
		compiled, diagnostics := compiler.CompileMOOWithKey(verb.Code, verb.CodeKey, s.registry)
		if len(diagnostics) > 0 {
			return fmt.Errorf("compile :recycle on #%d: %s", defObjID, diagnostics[0].Error())
		}
		program = compiled
		programmer = verb.Owner
		verbName = ":recycle"
		verbLoc = defObjID
	case types.TYPE_ANON:
		thisObj = root.ID()
		if !s.store.Valid(thisObj) {
			return nil
		}
		owner, errCode := s.store.ObjectOwner(thisObj)
		if errCode != types.E_NONE {
			return nil
		}
		compiled, diagnostics := compiler.CompileMOO([]string{"recycle(this);"}, s.registry)
		if len(diagnostics) > 0 {
			return fmt.Errorf("compile anonymous startup recycle: %s", diagnostics[0].Error())
		}
		program = compiled
		programmer = owner
		verbName = "startup_finalization"
		verbLoc = thisObj
	default:
		return nil
	}

	taskID := s.newTaskID()
	ticks, seconds := foregroundTaskLimits()
	tk := task.NewTaskFull(taskID, 0, program, ticks, seconds)
	s.populateTaskContextDependencies(tk.Context)
	tk.StartTime = time.Now()
	tk.Programmer = programmer
	tk.Context.Programmer = programmer
	tk.Context.IsWizard = s.isWizard(programmer)
	tk.Context.ServerInitiated = true
	tk.Context.FinalizingValue = root
	tk.VerbName = verbName
	tk.VerbLoc = verbLoc
	tk.This = thisObj
	tk.ThisValue = root
	tk.Caller = types.ObjNothing
	tk.ForkCreator = s

	tk.SetState(task.TaskQueued)
	s.mu.Lock()
	s.tasks[tk.ID] = tk
	s.mu.Unlock()
	task.GetManager().RegisterTask(tk)

	if err := s.runTask(tk); err != nil {
		return fmt.Errorf("run startup finalization %s: %w", root.String(), err)
	}
	if tk.Result.Flow == types.FlowSuspend {
		return fmt.Errorf("startup finalization %s suspended", root.String())
	}
	return nil
}
