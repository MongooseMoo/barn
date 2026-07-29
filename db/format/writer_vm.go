package format

import (
	"fmt"
	"time"

	"barn/bytecode"
	"barn/task"
	"barn/types"
)

const barnVMFrameMarker = "__barn_vm_frame_v1"

func (w *Writer) writeSuspendedTask(snapshot task.Snapshot) error {
	start := snapshot.StartTime
	if start.IsZero() {
		start = time.Now()
	}
	if _, err := fmt.Fprintf(w.w, "%d %d ", start.Unix(), snapshot.ID); err != nil {
		return err
	}
	if err := w.writeValue(snapshot.WakeValue); err != nil {
		return fmt.Errorf("write wake value: %w", err)
	}
	if err := w.writeValue(snapshot.TaskLocal); err != nil {
		return fmt.Errorf("write task-local value: %w", err)
	}

	machine := snapshot.VM
	ready := 0
	if snapshot.State == task.TaskQueued {
		ready = 1
	}
	if _, err := fmt.Fprintf(w.w, "%d -1 %d %d\n", len(machine.Frames)-1, ready, machine.MaxStackDepth); err != nil {
		return err
	}
	for i, frame := range machine.Frames {
		var activation task.ActivationFrame
		if i < len(snapshot.CallStack) {
			activation = snapshot.CallStack[i]
		} else {
			activation = task.ActivationFrame{
				This:       frame.This,
				ThisValue:  frame.ThisValue,
				Player:     frame.Player,
				Programmer: snapshot.Programmer,
				Caller:     frame.Caller,
				Verb:       frame.Verb,
				StoredVerb: frame.StoredVerb,
				VerbLoc:    frame.VerbLoc,
			}
		}
		if err := w.writeVMFrame(frame, activation); err != nil {
			return fmt.Errorf("write activation %d: %w", i, err)
		}
	}
	return nil
}

func (w *Writer) writeVMFrame(frame task.VMFrameSnapshot, activation task.ActivationFrame) error {
	if len(frame.Program.Source) == 0 {
		return fmt.Errorf("activation has no source program")
	}
	if err := w.writeString("language version 17"); err != nil {
		return err
	}
	for _, line := range frame.Program.Source {
		if err := w.writeString(line); err != nil {
			return err
		}
	}
	if err := w.writeString("."); err != nil {
		return err
	}

	bound := 0
	for i := range frame.Program.VarNames {
		if i < len(frame.Locals) && !frame.Locals[i].IsUnbound() {
			bound++
		}
	}
	if err := w.writeString(fmt.Sprintf("%d variables", bound)); err != nil {
		return err
	}
	for i, name := range frame.Program.VarNames {
		if i >= len(frame.Locals) || frame.Locals[i].IsUnbound() {
			continue
		}
		if err := w.writeString(name); err != nil {
			return err
		}
		if err := w.writeValue(frame.Locals[i]); err != nil {
			return err
		}
	}

	if err := w.writeString(fmt.Sprintf("%d rt_stack slots in use", len(frame.Stack))); err != nil {
		return err
	}
	for _, value := range frame.Stack {
		if err := w.writeValue(value); err != nil {
			return err
		}
	}

	if err := w.writeVMActivationAsPI(frame, activation); err != nil {
		return err
	}
	if err := w.writeValue(vmFrameMetadata(frame)); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w.w, "%d 0 %d\n", frame.IP, frame.IP)
	return err
}

func (w *Writer) writeVMActivationAsPI(frame task.VMFrameSnapshot, activation task.ActivationFrame) error {
	thisValue := frame.ThisValue
	if thisValue.IsNone() {
		thisValue = types.NewObj(frame.This)
	}
	storedVerb := frame.StoredVerb
	if storedVerb == "" {
		storedVerb = frame.Verb
	}
	programmer := activation.Programmer

	if err := w.writeValue(types.NewInt(-111)); err != nil {
		return err
	}
	if err := w.writeValue(thisValue); err != nil {
		return err
	}
	if err := w.writeValue(types.NewObj(frame.VerbLoc)); err != nil {
		return err
	}
	if err := w.writeInt(0); err != nil {
		return err
	}
	debug := 0
	if frame.VerbDebug {
		debug = 1
	}
	if _, err := fmt.Fprintf(
		w.w,
		"%d -7 -8 %d -9 %d %d -10 %d\n",
		frame.This,
		frame.Player,
		programmer,
		frame.VerbLoc,
		debug,
	); err != nil {
		return err
	}
	for _, placeholder := range []string{"No", "More", "Parse", "Infos", frame.Verb, storedVerb} {
		if err := w.writeString(placeholder); err != nil {
			return err
		}
	}
	return nil
}

func vmFrameMetadata(frame task.VMFrameSnapshot) types.Value {
	return types.NewList([]types.Value{
		types.NewStr(barnVMFrameMarker),
		bytesValue(frame.Program.Code),
		types.NewList(frame.Program.Constants),
		stringsValue(frame.Program.VarNames),
		lineInfoValue(frame.Program.LineInfo),
		types.NewInt(int64(frame.Program.NumLocals)),
		handlersValue(frame.ExceptStack),
		pendingErrorValue(frame.PendingError),
		types.NewList([]types.Value{
			types.NewBool(frame.VerbDebug),
			types.NewBool(frame.DiscardReturn),
			types.NewBool(frame.IsVerbCall),
			types.NewBool(frame.IsEvalFrame),
			types.NewBool(frame.SavedIsWizard),
		}),
		types.NewObj(frame.Caller),
		types.NewList(frame.Args),
		types.NewObj(frame.SavedThisObj),
		frame.SavedThisValue,
		types.NewStr(frame.SavedVerb),
		types.NewObj(frame.SavedProgrammer),
	})
}

func bytesValue(values []byte) types.Value {
	result := make([]types.Value, len(values))
	for i, value := range values {
		result[i] = types.NewInt(int64(value))
	}
	return types.NewList(result)
}

func stringsValue(values []string) types.Value {
	result := make([]types.Value, len(values))
	for i, value := range values {
		result[i] = types.NewStr(value)
	}
	return types.NewList(result)
}

func lineInfoValue(entries []bytecode.LineEntry) types.Value {
	result := make([]types.Value, len(entries))
	for i, entry := range entries {
		result[i] = types.NewList([]types.Value{
			types.NewInt(int64(entry.StartIP)),
			types.NewInt(int64(entry.Line)),
		})
	}
	return types.NewList(result)
}

func handlersValue(handlers []bytecode.Handler) types.Value {
	result := make([]types.Value, len(handlers))
	for i, handler := range handlers {
		codes := make([]types.Value, len(handler.Codes))
		for j, code := range handler.Codes {
			codes[j] = types.NewInt(int64(code))
		}
		result[i] = types.NewList([]types.Value{
			types.NewInt(int64(handler.Type)),
			types.NewInt(int64(handler.HandlerIP)),
			types.NewInt(int64(handler.EndIP)),
			types.NewList(codes),
			types.NewInt(int64(handler.VarIndex)),
			types.NewInt(int64(handler.StackDepth)),
		})
	}
	return types.NewList(result)
}

func pendingErrorValue(pending task.VMErrorSnapshot) types.Value {
	return types.NewList([]types.Value{
		types.NewBool(pending.Present),
		types.NewInt(int64(pending.Code)),
		pending.Value,
	})
}
