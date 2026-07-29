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
	w.suspendedTasks = append([]task.Snapshot(nil), suspended...)
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
	firstLineno := 1
	if len(t.CallStack) > 0 {
		firstLineno = t.CallStack[0].LineNumber
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
	if err := w.writeRtEnv(t.Fork.Variables); err != nil {
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
	if len(w.suspendedTasks) != 0 {
		return fmt.Errorf("cannot serialize %d suspended tasks without complete VM state", len(w.suspendedTasks))
	}
	return w.writeString("0 suspended tasks")
}

// writeInterruptedTasks writes all interrupted tasks (always 0 for now)
func (w *Writer) writeInterruptedTasks() error {
	// Interrupted tasks are rare edge cases - write 0 for now
	return w.writeString("0 interrupted tasks")
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
	}

	// Compatibility sentinel written by Toast's write_activ_as_pi.
	if err := w.writeValue(types.NewInt(-111)); err != nil {
		return err
	}

	// temp_this (typed)
	if err := w.writeValue(types.NewObj(thisObj)); err != nil {
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
func (w *Writer) writeRtEnv(vars map[string]types.Value) error {
	if err := w.writeString(fmt.Sprintf("%d variables", len(vars))); err != nil {
		return err
	}

	for name, val := range vars {
		if err := w.writeString(name); err != nil {
			return err
		}
		if err := w.writeValue(val); err != nil {
			return err
		}
	}

	return nil
}
