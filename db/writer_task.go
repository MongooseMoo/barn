package db

import (
	"barn/task"
	"barn/types"
	"fmt"
)

// TaskSource provides task lists for serialization
// This interface allows the writer to get tasks without directly depending on server/scheduler
type TaskSource interface {
	QueuedTasks() []*task.Task
	SuspendedTasks() []*task.Task
}

// SetTaskSource sets the task source for serialization
func (w *Writer) SetTaskSource(ts TaskSource) {
	w.taskSource = ts
}

// writeQueuedTasks writes all queued (forked) tasks
func (w *Writer) writeQueuedTasks() error {
	var tasks []*task.Task
	if w.taskSource != nil {
		tasks = w.taskSource.QueuedTasks()
	}

	// Filter to only tasks that have source lines available
	var serializableTasks []*task.Task
	for _, t := range tasks {
		if t.ForkInfo != nil && len(t.ForkInfo.SourceLines) > 0 {
			serializableTasks = append(serializableTasks, t)
		}
	}

	if err := w.writeString(fmt.Sprintf("%d queued tasks", len(serializableTasks))); err != nil {
		return err
	}

	for _, t := range serializableTasks {
		if err := w.writeQueuedTask(t); err != nil {
			return fmt.Errorf("write queued task %d: %w", t.ID, err)
		}
	}

	return nil
}

// writeQueuedTask writes a single queued (forked) task
// Format:
//   Header: "{unused} {firstLineno} {id} {st}"
//   ActivationAsPI
//   RtEnv: "{count} variables" + name/value pairs
//   Code: lines ending with "."
func (w *Writer) writeQueuedTask(t *task.Task) error {
	if t.ForkInfo == nil {
		return fmt.Errorf("task has no ForkInfo")
	}

	// Header: {unused} {firstLineno} {id} {st}
	// unused = 0, firstLineno = 1, id = task ID, st = queue time unix
	firstLineno := 1
	if len(t.CallStack) > 0 {
		firstLineno = t.CallStack[0].LineNumber
	}
	st := t.QueueTime.Unix()

	if _, err := fmt.Fprintf(w.w, "0 %d %d %d\n", firstLineno, t.ID, st); err != nil {
		return err
	}

	// ActivationAsPI
	if err := w.writeActivationAsPI(t); err != nil {
		return fmt.Errorf("write activation: %w", err)
	}

	// RtEnv: variables from ForkInfo
	if err := w.writeRtEnv(t.ForkInfo.Variables); err != nil {
		return fmt.Errorf("write rtenv: %w", err)
	}

	// Code: source lines
	for _, line := range t.ForkInfo.SourceLines {
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
	var tasks []*task.Task
	if w.taskSource != nil {
		tasks = w.taskSource.SuspendedTasks()
	}

	// For now, we can only serialize suspended tasks if we have source reconstruction
	// This requires more work, so for now write 0 suspended tasks
	// TODO: Implement full VM serialization when source line capture is available
	var serializableTasks []*task.Task
	// Currently empty - need source line capture to properly serialize

	if err := w.writeString(fmt.Sprintf("%d suspended tasks", len(serializableTasks))); err != nil {
		return err
	}

	for _, t := range serializableTasks {
		if err := w.writeSuspendedTask(t); err != nil {
			return fmt.Errorf("write suspended task %d: %w", t.ID, err)
		}
	}

	_ = tasks // Silence unused warning until we implement

	return nil
}

// writeSuspendedTask writes a single suspended task
// Format:
//   Header: "{startTime} {id} {type_code?} {value?}"
//   VM struct
func (w *Writer) writeSuspendedTask(t *task.Task) error {
	// Header: {startTime} {id} [type value]
	startTime := t.StartTime.Unix()

	// Write header with wake value type inlined
	typeCode := getTypeCode(t.WakeValue)
	if _, err := fmt.Fprintf(w.w, "%d %d %d\n", startTime, t.ID, typeCode); err != nil {
		return err
	}

	// Write wake value (without type tag since type is in header)
	if err := w.writeValueRaw(t.WakeValue); err != nil {
		return fmt.Errorf("write wake value: %w", err)
	}

	// Write VM struct
	if err := w.writeVM(t); err != nil {
		return fmt.Errorf("write VM: %w", err)
	}

	return nil
}

// writeInterruptedTasks writes all interrupted tasks (always 0 for now)
func (w *Writer) writeInterruptedTasks() error {
	// Interrupted tasks are rare edge cases - write 0 for now
	return w.writeString("0 interrupted tasks")
}

// writeActivationAsPI writes activation in Program Info format
// Format:
//   temp_value (typed)
//   temp_this (typed)
//   temp_vloc (typed)
//   threaded (raw int)
//   Header: "{this} {unused1} {unused2} {player} {unused3} {programmer} {vloc} {unused4} {debug}"
//   "No"
//   "More"
//   "Parse"
//   "Infos"
//   verb (string)
//   verbname (string)
func (w *Writer) writeActivationAsPI(t *task.Task) error {
	// Get values from task
	thisObj := t.This
	player := t.Owner
	programmer := t.Programmer
	vloc := t.VerbLoc
	verb := t.VerbName

	if len(t.CallStack) > 0 {
		frame := t.CallStack[0]
		thisObj = frame.This
		player = frame.Player
		programmer = frame.Programmer
		vloc = frame.VerbLoc
		verb = frame.Verb
	}

	// temp_value (typed) - typically 0
	if err := w.writeValue(types.NewInt(0)); err != nil {
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
	if _, err := fmt.Fprintf(w.w, "%d 0 0 %d 0 %d %d 0 %d\n",
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
	if err := w.writeString(verb); err != nil {
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

// writeVM writes VM struct for suspended/interrupted tasks
func (w *Writer) writeVM(t *task.Task) error {
	// locals (typed value) - task_local storage
	if err := w.writeValue(t.TaskLocal); err != nil {
		return err
	}

	// VM Header: {top} {vector} {funcId} {maxStackframes}
	top := len(t.CallStack) - 1
	if top < 0 {
		top = 0
	}
	if _, err := fmt.Fprintf(w.w, "%d 0 0 50\n", top); err != nil {
		return err
	}

	// Write (top+1) activations
	for i := 0; i <= top; i++ {
		if err := w.writeFullActivation(t, i); err != nil {
			return fmt.Errorf("write activation %d: %w", i, err)
		}
	}

	return nil
}

// writeFullActivation writes a full activation frame for VM stack
func (w *Writer) writeFullActivation(t *task.Task, index int) error {
	// This is a placeholder - full activation serialization requires
	// source line capture which isn't implemented yet

	// langver
	if err := w.writeString("language version 2"); err != nil {
		return err
	}

	// code (empty for now)
	if err := w.writeString("."); err != nil {
		return err
	}

	// rtEnv (empty)
	if err := w.writeString("0 variables"); err != nil {
		return err
	}

	// stack slots
	if err := w.writeString("0 rt_stack slots in use"); err != nil {
		return err
	}

	// activation_as_pi
	if err := w.writeActivationAsPI(t); err != nil {
		return err
	}

	// temp_end
	if err := w.writeValue(types.NewInt(0)); err != nil {
		return err
	}

	// PC header: {pc} {bi_func} {error}
	if err := w.writeString("0 0 0"); err != nil {
		return err
	}

	return nil
}

