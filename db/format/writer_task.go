package format

import (
	"barn/task"
	"barn/types"
	"fmt"
	"time"
)

// SetTaskSnapshots sets queued and suspended task snapshots for serialization.
func (w *Writer) SetTaskSnapshots(queued, suspended []task.Snapshot) {
	w.queuedTasks = append([]task.Snapshot(nil), queued...)
	w.suspendedTasks = w.suspendedTasks[:0]
	w.interruptedTasks = w.interruptedTasks[:0]
	for _, snapshot := range suspended {
		if snapshot.ReadingPlayer != types.ObjNothing || snapshot.IsExecSuspended || snapshot.IsHTTPReadSuspended {
			w.interruptedTasks = append(w.interruptedTasks, snapshot)
		} else {
			w.suspendedTasks = append(w.suspendedTasks, snapshot)
		}
	}
}

// writeQueuedTasks writes all queued (forked) tasks
func (w *Writer) writeQueuedTasks() error {
	for _, t := range w.queuedTasks {
		if t.Fork == nil || len(t.Fork.SourceLines) == 0 {
			return fmt.Errorf("queued task %d has no serializable fork program", t.ID)
		}
	}

	if err := w.writeString(fmt.Sprintf("%d queued tasks", len(w.queuedTasks))); err != nil {
		return err
	}

	for _, t := range w.queuedTasks {
		if err := w.writeQueuedTask(t); err != nil {
			return fmt.Errorf("write queued task %d: %w", t.ID, err)
		}
	}

	return nil
}

// writeQueuedTask writes a single queued (forked) task
// Format:
//
//	Header: "{unused} {firstLineno} {st} {id}"
//	ActivationAsPI
//	RtEnv: "{count} variables" + name/value pairs
//	Code: lines ending with "."
func (w *Writer) writeQueuedTask(t task.Snapshot) error {
	if t.Fork == nil {
		return fmt.Errorf("task has no ForkInfo")
	}

	// Header: {unused} {firstLineno} {st} {id}
	// unused = 0, firstLineno = 1, id = task ID, st = queue time unix
	firstLineno := t.Fork.FirstLine
	if firstLineno <= 0 {
		return fmt.Errorf("task has no fork program first line")
	}
	st := t.StartTime.Add(500 * time.Millisecond).Unix()

	if _, err := fmt.Fprintf(w.w, "0 %d %d %d\n", firstLineno, st, t.ID); err != nil {
		return err
	}

	// ActivationAsPI
	if err := w.writeActivationAsPI(t); err != nil {
		return fmt.Errorf("write activation: %w", err)
	}

	// RtEnv: variables from ForkInfo
	if err := w.writeRtEnv(t.Fork.VariableNames, t.Fork.Variables); err != nil {
		return fmt.Errorf("write rtenv: %w", err)
	}

	// Code: source lines
	for _, line := range t.Fork.SourceLines {
		if err := w.writeString(line); err != nil {
			return err
		}
	}
	if err := w.writeString("."); err != nil {
		return err
	}

	return nil
}

// writeSuspendedTasks writes all suspended tasks
func (w *Writer) writeSuspendedTasks() error {
	for _, suspended := range w.suspendedTasks {
		if suspended.VM == nil || len(suspended.VM.Frames) == 0 {
			return fmt.Errorf("suspended task %d has no serializable VM state", suspended.ID)
		}
	}
	if err := w.writeString(fmt.Sprintf("%d suspended tasks", len(w.suspendedTasks))); err != nil {
		return err
	}
	for _, suspended := range w.suspendedTasks {
		if err := w.writeSuspendedTask(suspended); err != nil {
			return fmt.Errorf("write suspended task %d: %w", suspended.ID, err)
		}
	}
	return nil
}

// writeInterruptedTasks writes tasks blocked in read().
func (w *Writer) writeInterruptedTasks() error {
	for _, interrupted := range w.interruptedTasks {
		if interrupted.VM == nil || len(interrupted.VM.Frames) == 0 {
			return fmt.Errorf("interrupted task %d has no serializable VM state", interrupted.ID)
		}
	}
	if err := w.writeString(fmt.Sprintf("%d interrupted tasks", len(w.interruptedTasks))); err != nil {
		return err
	}
	for _, interrupted := range w.interruptedTasks {
		status := "interrupted reading task"
		if interrupted.IsExecSuspended {
			if interrupted.ExecCommandName == "" {
				return fmt.Errorf("interrupted exec task %d has no command name", interrupted.ID)
			}
			status = interrupted.ExecCommandName
		}
		if err := w.writeString(fmt.Sprintf("%d %s", interrupted.ID, status)); err != nil {
			return err
		}
		if err := w.writeValue(interrupted.TaskLocal); err != nil {
			return fmt.Errorf("write interrupted task-local value: %w", err)
		}
		machine := interrupted.VM
		if _, err := fmt.Fprintf(w.w, "%d -1 0 %d\n", len(machine.Frames)-1, machine.MaxStackDepth); err != nil {
			return err
		}
		for i, frame := range machine.Frames {
			var activation task.ActivationFrame
			if i < len(interrupted.CallStack) {
				activation = interrupted.CallStack[i]
			} else {
				activation = task.ActivationFrame{
					This:       frame.This,
					ThisValue:  frame.ThisValue,
					Player:     frame.Player,
					Programmer: interrupted.Programmer,
					Caller:     frame.Caller,
					Verb:       frame.Verb,
					StoredVerb: frame.StoredVerb,
					VerbLoc:    frame.VerbLoc,
				}
			}
			if err := w.writeVMFrame(frame, activation); err != nil {
				return fmt.Errorf("write interrupted activation %d: %w", i, err)
			}
		}
	}
	return nil
}

// writeActivationAsPI writes activation in Program Info format
// Format:
//
//	temp_value (typed)
//	temp_this (typed)
//	temp_vloc (typed)
//	threaded (raw int)
//	Header: "{this} {unused1} {unused2} {player} {unused3} {programmer} {vloc} {unused4} {debug}"
//	"No"
//	"More"
//	"Parse"
//	"Infos"
//	verb (string)
//	verbname (string)
func (w *Writer) writeActivationAsPI(t task.Snapshot) error {
	// Get values from task
	thisObj := t.This
	player := t.Owner
	programmer := t.Programmer
	vloc := t.VerbLoc
	verb := t.VerbName
	verbname := t.VerbName
	thisValue := types.NewObj(thisObj)

	if len(t.CallStack) > 0 {
		frame := t.CallStack[0]
		thisObj = frame.This
		player = frame.Player
		programmer = frame.Programmer
		vloc = frame.VerbLoc
		verb = frame.Verb
		verbname = frame.StoredVerb
		if verbname == "" {
			verbname = verb
		}
		if !frame.ThisValue.IsNone() {
			thisValue = frame.ThisValue
		}
	}

	// Compatibility sentinel written by Toast's write_activ_as_pi.
	if err := w.writeValue(types.NewInt(-111)); err != nil {
		return err
	}

	// _this (typed)
	if err := w.writeValue(thisValue); err != nil {
		return err
	}

	// temp_vloc (typed)
	if err := w.writeValue(types.NewObj(vloc)); err != nil {
		return err
	}

	// threaded (raw int, no type tag) - 0 for non-threaded
	if err := w.writeInt(0); err != nil {
		return err
	}

	// Header: {this} {unused1} {unused2} {player} {unused3} {programmer} {vloc} {unused4} {debug}
	debug := 0
	if _, err := fmt.Fprintf(w.w, "%d -7 -8 %d -9 %d %d -10 %d\n",
		thisObj, player, programmer, vloc, debug); err != nil {
		return err
	}

	// Argstr placeholders
	if err := w.writeString("No"); err != nil {
		return err
	}
	if err := w.writeString("More"); err != nil {
		return err
	}
	if err := w.writeString("Parse"); err != nil {
		return err
	}
	if err := w.writeString("Infos"); err != nil {
		return err
	}

	// verb and verbname
	if err := w.writeString(verb); err != nil {
		return err
	}
	if err := w.writeString(verbname); err != nil {
		return err
	}

	return nil
}

// writeRtEnv writes runtime environment variables
func (w *Writer) writeRtEnv(names []string, vars map[string]types.Value) error {
	if len(names) != len(vars) {
		return fmt.Errorf("runtime environment has %d names for %d values", len(names), len(vars))
	}
	if err := w.writeString(fmt.Sprintf("%d variables", len(names))); err != nil {
		return err
	}

	for _, name := range names {
		val, ok := vars[name]
		if !ok {
			return fmt.Errorf("runtime environment is missing variable %q", name)
		}
		if err := w.writeString(name); err != nil {
			return err
		}
		if err := w.writeValue(val); err != nil {
			return err
		}
	}

	return nil
}
