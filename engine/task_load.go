package engine

import (
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

func (s *Runtime) LoadQueuedTasks(queued []*dbformat.QueuedTask) {
	restored := 0
	for _, saved := range queued {
		if saved == nil || saved.ID <= 0 {
			continue
		}
		if err := s.loadQueuedTask(saved); err != nil {
			slog.Warn("failed to restore queued task",
				slog.Int64("task_id", saved.ID),
				slog.Any("err", err))
			continue
		}
		restored++
	}
}

func (s *Runtime) LoadSuspendedTasks(suspended []*dbformat.SuspendedTask) error {
	var failures []error
	for index, saved := range suspended {
		if saved == nil || saved.Snapshot.ID <= 0 || saved.Snapshot.VM == nil {
			failures = append(failures, fmt.Errorf("suspended task record %d is incomplete", index))
			continue
		}
		err := s.loadSuspendedTask(saved.Snapshot)
		var mismatch *vm.ProgramMismatchError
		if errors.As(err, &mismatch) {
			// Q, 2026-11-10: a task whose program is not what its saved source
			// compiles to is dropped, and the server keeps booting.
			slog.Error("dropped suspended task: saved program does not match its source",
				slog.Int64("task_id", saved.Snapshot.ID),
				slog.String("verb", saved.Snapshot.VerbName),
				slog.Int64("verbloc", int64(saved.Snapshot.VerbLoc)),
				slog.Int64("this", int64(saved.Snapshot.This)),
				slog.Int64("programmer", int64(saved.Snapshot.Programmer)),
				slog.String("reason", mismatch.Error()))
			continue
		}
		if err != nil {
			slog.Warn("failed to restore suspended task",
				slog.Int64("task_id", saved.Snapshot.ID),
				slog.String("verb", saved.Snapshot.VerbName),
				slog.Int64("verbloc", int64(saved.Snapshot.VerbLoc)),
				slog.Int64("this", int64(saved.Snapshot.This)),
				slog.Int64("programmer", int64(saved.Snapshot.Programmer)),
				slog.Any("err", err))
			failures = append(failures, fmt.Errorf("suspended task %d: %w", saved.Snapshot.ID, err))
		}
	}
	return errors.Join(failures...)
}

func (s *Runtime) loadSuspendedTask(saved task.Snapshot) error {
	if len(saved.VM.Frames) == 0 {
		return fmt.Errorf("VM has no activations")
	}
	rootProgram := saved.VM.Frames[0].Program
	ticks, seconds := backgroundTaskLimits(s.session)
	t := NewTaskFull(saved.ID, saved.Owner, &rootProgram, ticks, seconds)
	s.populateContextDependencies(t.Context)
	t.StartTime = saved.StartTime
	t.QueueTime = saved.StartTime
	t.WakeValue = saved.WakeValue
	t.TaskLocal = saved.TaskLocal
	t.Kind = task.TaskSuspendedTask
	t.IsForked = true
	t.Programmer = saved.Programmer
	t.This = saved.This
	t.VerbName = saved.VerbName
	t.VerbLoc = saved.VerbLoc
	t.ForkCreator = s
	for _, frame := range saved.CallStack {
		t.PushFrame(frame)
	}

	top := saved.CallStack[len(saved.CallStack)-1]
	t.Context.ThisObj = top.This
	t.Context.ThisValue = top.ThisValue
	t.Context.Player = top.Player
	t.Context.Programmer = top.Programmer
	t.Context.Verb = top.Verb
	t.Context.IsWizard = s.isWizard(top.Programmer)

	machine, err := vm.RestoreVMSnapshot(saved.VM, s.store, s.session, t.Context)
	if err != nil {
		return err
	}
	machine.Task = &t.State
	// Native import can produce a different root program from the raw saved
	// frame. Keep the task's program consistent with its installed executable VM.
	t.Program = machine.Frames[0].Program
	t.SetVM(machine)

	if saved.State == task.TaskQueued {
		s.QueueTask(t)
	} else {
		t.SetState(task.TaskSuspended)
		t.WakeTime = saved.StartTime
		if saved.StartTime.Unix() < 0 {
			// Older 64-bit Toast servers wrote an indefinite suspension's
			// INTNUM_MAX start time through "%d" as -1; Toast's reader treats a
			// negative start time as indefinite (tasks.cc read_task_queue).
			t.WakeTime = time.Time{}
			t.StartTime = task.IndefiniteSuspendStartTime
		}
		s.taskManager.RegisterTask(t)
	}
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

func (s *Runtime) loadQueuedTask(saved *dbformat.QueuedTask) error {
	prog, diagnostics := s.registry.Compiler().CompileMOO(saved.Code)
	if len(diagnostics) > 0 {
		return fmt.Errorf("%s", diagnostics[0].Error())
	}

	ticks, seconds := backgroundTaskLimits(s.session)
	t := NewTaskFull(saved.ID, saved.Player, prog, ticks, seconds)
	s.populateContextDependencies(t.Context)

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

	machine := vm.NewVM(s.store, s.session)
	machine.Context = t.Context
	machine.Task = &t.State
	machine.TickLimit = ticks
	configureVMStackLimit(machine, s.session)
	frame := machine.PrepareVerbFrame(prog, saved.This, saved.Player, saved.Player, saved.Verb, saved.VerbLoc, nil)
	// A restored fork runs the recompiled isolated body. Its environment must
	// use that program's names, not the enclosing parent's saved name table.
	// Native reorder_rt_env leaves unmatched slots uninitialized, including
	// implicit type constants; this is not a fresh verb's initialization.
	variables, err := task.RestoreForkEnvironment(prog, saved.Variables)
	if err != nil {
		return err
	}
	t.ForkInfo.Variables = variables
	for slot, name := range prog.VarNames {
		frame.Locals[slot] = variables[name]
	}
	for _, name := range task.CommandVariableNames {
		if value, present := variables[name]; present {
			vm.SetLocalByName(frame, prog, name, value)
		}
	}
	t.SetVM(machine)

	t.PushFrame(types.ActivationFrame{
		This:       saved.This,
		ThisValue:  types.None, // explicit None: zero Value{} is int 0 post-de-box; ToList would render this as 0
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
