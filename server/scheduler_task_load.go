package server

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"barn/bytecode"
	dbformat "barn/db/format"
	"barn/task"
	"barn/types"
	"barn/vm"
)

func (s *Scheduler) LoadQueuedTasks(queued []*dbformat.QueuedTask) {
	restored := 0
	for _, saved := range queued {
		if saved == nil || saved.ID <= 0 || len(saved.Code) == 0 {
			continue
		}
		if err := s.loadQueuedTask(saved); err != nil {
			log.Printf("Warning: failed to restore queued task %d: %v", saved.ID, err)
			continue
		}
		restored++
	}
}

func (s *Scheduler) loadQueuedTask(saved *dbformat.QueuedTask) error {
	program, errors := bytecode.CompileVerb(saved.Code)
	if len(errors) > 0 {
		return fmt.Errorf("%s", errors[0])
	}

	compiler := bytecode.NewCompilerWithRegistry(s.registry)
	prog, err := compiler.CompileStatements(program.Statements)
	if err != nil {
		return err
	}
	prog.Source = append([]string(nil), saved.Code...)

	ticks, seconds := backgroundTaskLimits()
	t := task.NewTaskFull(saved.ID, saved.Player, nil, ticks, seconds)
	s.populateTaskContextDependencies(t.Context)

	start := time.Unix(saved.StartTime, 0)
	if saved.StartTime <= 0 {
		start = time.Now()
	}
	t.StartTime = start
	t.QueueTime = start
	t.Kind = task.TaskForked
	t.IsForked = true
	t.Programmer = saved.Programmer
	t.This = saved.This
	t.Caller = saved.Player
	t.VerbName = saved.Verb
	t.VerbLoc = saved.VerbLoc
	t.ForkCreator = s
	t.ForkInfo = &types.ForkInfo{
		SourceLines: append([]string(nil), saved.Code...),
		Variables:   copyValueMap(saved.Variables),
		ThisObj:     saved.This,
		Player:      saved.Player,
		Caller:      saved.Player,
		Verb:        saved.Verb,
		VerbLoc:     saved.VerbLoc,
	}

	t.Context.ThisObj = saved.This
	t.Context.Player = saved.Player
	t.Context.Programmer = saved.Programmer
	t.Context.Verb = saved.Verb
	t.Context.IsWizard = s.isWizard(saved.Programmer)
	t.Context.Task = t

	machine := vm.NewVM(s.store, s.registry)
	machine.Context = t.Context
	machine.TickLimit = ticks
	configureVMStackLimit(machine)
	frame := machine.PrepareVerbFrame(prog, saved.This, saved.Player, saved.Player, saved.Verb, saved.VerbLoc, nil)
	for name, value := range saved.Variables {
		vm.SetLocalByName(frame, prog, name, value)
	}
	t.BytecodeVM = machine

	t.PushFrame(task.ActivationFrame{
		This:       saved.This,
		Player:     saved.Player,
		Programmer: saved.Programmer,
		Caller:     saved.Player,
		Verb:       saved.Verb,
		VerbLoc:    saved.VerbLoc,
		LineNumber: 1,
	})

	s.QueueTask(t)
	for {
		current := atomic.LoadInt64(&s.nextTaskID)
		if current >= saved.ID {
			break
		}
		if atomic.CompareAndSwapInt64(&s.nextTaskID, current, saved.ID) {
			break
		}
	}
	return nil
}

func copyValueMap(values map[string]types.Value) map[string]types.Value {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]types.Value, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}
