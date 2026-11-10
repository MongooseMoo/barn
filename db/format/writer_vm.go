package format

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// barnVMFrameMarker heads a Barn frame's metadata. A v2 frame stores no
// bytecode: element 2 names the compile of the frame's source that produced
// its program and that program's fingerprint, and every later element is a
// v1 element from 7 on. A v1 frame stored the bytecode in elements 2-6.
const (
	barnVMFrameMarker   = "__barn_vm_frame_v2"
	barnVMFrameMarkerV1 = "__barn_vm_frame_v1"
)

func (w *Writer) writeSuspendedTask(snapshot task.Snapshot) error {
	start := snapshot.StartTime
	if start.IsZero() {
		start = time.Now()
	}
	if _, err := fmt.Fprintf(w.w, "%d %d ", start.Add(500*time.Millisecond).Unix(), snapshot.ID); err != nil {
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
		var activation types.ActivationFrame
		if i < len(snapshot.CallStack) {
			activation = snapshot.CallStack[i]
		} else {
			activation = types.ActivationFrame{
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

func (w *Writer) writeVMFrame(frame task.VMFrameSnapshot, activation types.ActivationFrame) error {
	for _, reason := range frame.PendingReasons {
		if !reason.Error.ValidExceptionReason() {
			return fmt.Errorf("invalid pending error reason")
		}
	}
	// An empty source is an empty program, which Toast writes as the "."
	// terminator alone (execute.cc write_activ -> dbio_write_program).
	// Restore runs the compile of this source that vmFrameMetadata names.
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

	bound := orderedBoundLocalIndices(frame)
	if err := w.writeString(fmt.Sprintf("%d variables", len(bound))); err != nil {
		return err
	}
	for _, i := range bound {
		name := frame.Program.VarNames[i]
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

// orderedBoundLocalIndices keeps ordinary environment order while moving a
// direct WAIF alias behind a composite local that owns the same identity. That
// makes the aggregate value the creation site in Toast's c/r WAIF stream while
// retaining both named locals and their shared identity on reload.
func orderedBoundLocalIndices(frame task.VMFrameSnapshot) []int {
	ordinary := make([]int, 0, len(frame.Program.VarNames))
	coveredAliases := make([]int, 0)
	for index := range frame.Program.VarNames {
		if index >= len(frame.Locals) || frame.Locals[index].IsUnbound() {
			continue
		}
		candidate := frame.Locals[index]
		if candidate.Type() == types.TYPE_WAIF && waifOwnedByAnotherLocal(candidate, frame.Locals, index) {
			coveredAliases = append(coveredAliases, index)
			continue
		}
		ordinary = append(ordinary, index)
	}
	return append(ordinary, coveredAliases...)
}

func waifOwnedByAnotherLocal(candidate types.Value, locals []types.Value, candidateIndex int) bool {
	for index, owner := range locals {
		if index == candidateIndex {
			continue
		}
		switch owner.Type() {
		case types.TYPE_LIST, types.TYPE_MAP:
			if valueContainsWaif(owner, candidate, nil) {
				return true
			}
		case types.TYPE_WAIF:
			if !owner.Equal(candidate) && valueContainsWaifProperties(owner, candidate, nil) {
				return true
			}
		}
	}
	return false
}

func valueContainsWaif(value, candidate types.Value, visited map[types.WaifIdentity]struct{}) bool {
	switch value.Type() {
	case types.TYPE_WAIF:
		if value.Equal(candidate) {
			return true
		}
		return valueContainsWaifProperties(value, candidate, visited)
	case types.TYPE_LIST:
		for _, element := range value.Elements() {
			if valueContainsWaif(element, candidate, visited) {
				return true
			}
		}
	case types.TYPE_MAP:
		for _, pair := range value.Pairs() {
			if valueContainsWaif(pair[0], candidate, visited) || valueContainsWaif(pair[1], candidate, visited) {
				return true
			}
		}
	}
	return false
}

func valueContainsWaifProperties(value, candidate types.Value, visited map[types.WaifIdentity]struct{}) bool {
	identity := value.WaifIdentity()
	if _, seen := visited[identity]; seen {
		return false
	}
	if visited == nil {
		visited = make(map[types.WaifIdentity]struct{})
	}
	visited[identity] = struct{}{}
	for _, name := range value.PropertyNames() {
		if property, ok := value.GetProperty(name); ok && valueContainsWaif(property, candidate, visited) {
			return true
		}
	}
	return false
}

func (w *Writer) writeVMActivationAsPI(frame task.VMFrameSnapshot, activation types.ActivationFrame) error {
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
	threaded := 0
	if frame.ThreadMode {
		threaded = 1
	}
	if err := w.writeInt(threaded); err != nil {
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
		programIdentityValue(&frame.Program),
		handlersValue(frame.ExceptStack),
		pendingErrorValue(task.VMErrorSnapshot{}), // fixed legacy slot, no duplicate reason state
		types.NewList([]types.Value{
			types.NewBool(frame.VerbDebug),
			types.NewBool(frame.DiscardReturn),
			types.NewBool(frame.IsVerbCall),
			types.NewBool(frame.IsEvalFrame),
			types.NewBool(frame.SavedIsWizard),
			types.NewBool(frame.ThreadMode),
		}),
		types.NewObj(frame.Caller),
		types.NewList(frame.Args),
		types.NewObj(frame.SavedThisObj),
		frame.SavedThisValue,
		types.NewStr(frame.SavedVerb),
		types.NewObj(frame.SavedProgrammer),
		moveContinuationValue(frame.MoveContinuation),
		pendingReturnValue(types.None, false), // fixed legacy slot
		recycleContinuationValue(frame.RecycleContinuation),
		internalLocalsValue(frame),
		types.NewStr(hex.EncodeToString(frame.Program.BuiltinLayout[:])),
		types.NewList(frame.CommandEnvironment[:]),
		types.NewInt(int64(frame.ReturnLocal)),
		types.NewList([]types.Value{types.NewStr(frame.BuiltinCaller), types.NewInt(int64(frame.BuiltinPC))}),
		pendingControlValue(frame),
	})
}

// programIdentityValue is {fork, native, fingerprint}: the program's
// bytecode.Origin, with native {} or {vector, language version}, and the
// hex SHA-256 bytecode.Program.Fingerprint.
func programIdentityValue(program *bytecode.Program) types.Value {
	native := types.NewList(nil)
	if program.Origin.Native {
		native = types.NewList([]types.Value{types.NewInt(int64(program.Origin.NativeVector)), types.NewInt(int64(program.Origin.LanguageVersion))})
	}
	fingerprint := program.Fingerprint()
	return types.NewList([]types.Value{
		types.NewInt(int64(program.Origin.Fork)),
		native,
		types.NewStr(hex.EncodeToString(fingerprint[:])),
	})
}

func pendingControlValue(frame task.VMFrameSnapshot) types.Value {
	reasons := make([]types.Value, len(frame.PendingReasons))
	for index, pending := range frame.PendingReasons {
		reasons[index] = types.NewList([]types.Value{
			types.NewInt(int64(pending.Kind)), types.NewInt(int64(pending.FinallyIP)), pending.Value,
			pendingErrorValue(pending.Error), types.NewInt(int64(pending.TargetIP)), types.NewInt(int64(pending.HandlerDepth)), types.NewList(pending.Stack),
		})
	}
	return types.NewList([]types.Value{
		types.NewStr("pending-reasons-v1"), types.NewList(reasons),
	})
}

// internalLocalsValue captures the bound compiler-temporary slots, which sit
// above the named variables and therefore never appear in the Toast-shaped
// rt_env written by writeVMFrame. Loop cursors and other continuation state
// live there, so a task suspended inside such a construct needs them back.
// Encoded as {{slot, value}, ...} with zero-based slots.
func internalLocalsValue(frame task.VMFrameSnapshot) types.Value {
	entries := make([]types.Value, 0)
	for slot := len(frame.Program.VarNames); slot < len(frame.Locals); slot++ {
		if frame.Locals[slot].IsUnbound() || frame.Locals[slot].IsNone() {
			continue
		}
		entries = append(entries, types.NewList([]types.Value{
			types.NewInt(int64(slot)),
			frame.Locals[slot],
		}))
	}
	return types.NewList(entries)
}

func pendingReturnValue(value types.Value, present bool) types.Value {
	return types.NewList([]types.Value{types.NewBool(present), value})
}

func moveContinuationValue(state *task.MoveContinuationSnapshot) types.Value {
	if state == nil {
		return types.NewList(nil)
	}
	return types.NewList([]types.Value{
		types.NewInt(int64(state.Stage)),
		state.What,
		state.Where,
		state.OldLocation,
		types.NewInt(state.Position),
		types.NewBool(state.Decentralized),
	})
}

func recycleContinuationValue(state *task.RecycleContinuationSnapshot) types.Value {
	if state == nil {
		return types.NewList(nil)
	}
	return types.NewList([]types.Value{
		state.Object,
		objectIDsValue(state.OldParents),
		objectIDsValue(state.OldChildren),
		objectIDsValue(state.OldContents),
		types.NewObj(state.OldLocation),
		types.NewBool(state.ExitCallback),
	})
}

func objectIDsValue(ids []types.ObjID) types.Value {
	values := make([]types.Value, len(ids))
	for i, id := range ids {
		values[i] = types.NewObj(id)
	}
	return types.NewList(values)
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
			types.NewInt(int64(handler.ExceptBase)),
		})
	}
	return types.NewList(result)
}

func pendingErrorValue(pending task.VMErrorSnapshot) types.Value {
	return types.NewList([]types.Value{
		types.NewBool(pending.Present),
		types.NewInt(int64(pending.Code)),
		pending.Value,
		types.NewInt(int64(pending.Reason)),
	})
}
