package format

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func (database *Database) readSuspendedTask(r *bufio.Reader) (*SuspendedTask, error) {
	header, err := readLine(r)
	if err != nil {
		return nil, fmt.Errorf("read task header: %w", err)
	}
	fields := strings.Fields(header)
	if len(fields) < 3 {
		return nil, fmt.Errorf("parse task header %q: expected time, id, and value type", header)
	}
	start, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse task start time: %w", err)
	}
	id, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse task id: %w", err)
	}
	typeCode, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, fmt.Errorf("parse wake value type: %w", err)
	}
	wakeValue, err := database.readValueAfterType(r, typeCode)
	if err != nil {
		return nil, fmt.Errorf("read wake value: %w", err)
	}
	taskLocal, err := database.readValue(r)
	if err != nil {
		return nil, fmt.Errorf("read task-local value: %w", err)
	}

	vmHeader, err := readLine(r)
	if err != nil {
		return nil, fmt.Errorf("read VM header: %w", err)
	}
	var top, rootVector, ready, maxStack int
	if n, scanErr := fmt.Sscanf(vmHeader, "%d %d %d %d", &top, &rootVector, &ready, &maxStack); scanErr != nil || n != 4 {
		return nil, fmt.Errorf("parse VM header %q", vmHeader)
	}
	if top < 0 || top == math.MaxInt {
		return nil, fmt.Errorf("invalid top activation %d", top)
	}

	vmSnapshot := &task.VMSnapshot{
		MaxStackDepth:    maxStack,
		Frames:           make([]task.VMFrameSnapshot, 0, min(top+1, maxTrustedCount)),
		NativeRootVector: rootVector,
		NativeFunctionID: ready,
	}
	callStack := make([]types.ActivationFrame, 0, min(top+1, maxTrustedCount))
	for i := 0; i <= top; i++ {
		frame, activation, err := database.readVMFrame(r)
		if err != nil {
			return nil, fmt.Errorf("read activation %d: %w", i, err)
		}
		vmSnapshot.Frames = append(vmSnapshot.Frames, frame)
		callStack = append(callStack, activation)
	}

	state := task.TaskSuspended
	if ready != 0 {
		state = task.TaskQueued
	}
	root := callStack[0]
	return &SuspendedTask{Snapshot: task.Snapshot{
		ID:            id,
		Owner:         root.Player,
		State:         state,
		StartTime:     time.Unix(start, 0),
		WakeValue:     wakeValue,
		TaskLocal:     taskLocal,
		CallStack:     callStack,
		Programmer:    root.Programmer,
		VerbLoc:       root.VerbLoc,
		VerbName:      root.Verb,
		This:          root.This,
		ReadingPlayer: types.ObjNothing,
		VM:            vmSnapshot,
	}}, nil
}

func (database *Database) readVMFrame(r *bufio.Reader) (task.VMFrameSnapshot, types.ActivationFrame, error) {
	var frame task.VMFrameSnapshot
	var activation types.ActivationFrame

	version, err := readLine(r)
	if err != nil {
		return frame, activation, err
	}
	if !strings.HasPrefix(version, "language version ") {
		return frame, activation, fmt.Errorf("expected language version, got %q", version)
	}
	languageVersion, err := strconv.Atoi(strings.TrimPrefix(version, "language version "))
	if err != nil {
		return frame, activation, fmt.Errorf("parse language version %q: %w", version, err)
	}
	var source []string
	for {
		line, err := readLine(r)
		if err != nil {
			return frame, activation, fmt.Errorf("read program: %w", err)
		}
		if line == "." {
			break
		}
		source = append(source, line)
	}

	envNames, envValues, err := database.readOrderedRtEnv(r)
	if err != nil {
		return frame, activation, err
	}
	stackHeader, err := readLine(r)
	if err != nil {
		return frame, activation, err
	}
	var stackCount int
	if _, err := fmt.Sscanf(stackHeader, "%d rt_stack slots in use", &stackCount); err != nil {
		return frame, activation, fmt.Errorf("parse runtime stack header %q: %w", stackHeader, err)
	}
	if stackCount < 0 {
		return frame, activation, fmt.Errorf("invalid runtime stack count %d", stackCount)
	}
	frame.Stack = make([]types.Value, 0, min(stackCount, maxTrustedCount))
	operandTypes := make([]int, 0, min(stackCount, maxTrustedCount))
	for i := 0; i < stackCount; i++ {
		kind, err := readInt(r)
		if err != nil {
			return frame, activation, fmt.Errorf("read runtime stack type %d: %w", i, err)
		}
		value, err := database.readValueAfterType(r, kind)
		if err != nil {
			return frame, activation, fmt.Errorf("read runtime stack value %d: %w", i, err)
		}
		frame.Stack = append(frame.Stack, value)
		operandTypes = append(operandTypes, kind)
	}

	sentinel, err := database.readValue(r)
	if err != nil {
		return frame, activation, fmt.Errorf("read activation sentinel: %w", err)
	}
	thisValue, err := database.readValue(r)
	if err != nil {
		return frame, activation, fmt.Errorf("read activation receiver: %w", err)
	}
	verbLocValue, err := database.readValue(r)
	if err != nil {
		return frame, activation, fmt.Errorf("read activation verb location: %w", err)
	}
	threadedLine, err := readLine(r)
	if err != nil {
		return frame, activation, fmt.Errorf("read activation thread mode: %w", err)
	}
	threaded, err := strconv.Atoi(strings.TrimSpace(threadedLine))
	if err != nil {
		return frame, activation, fmt.Errorf("parse activation thread mode %q: %w", threadedLine, err)
	}
	verbRef, err := readLine(r)
	if err != nil {
		return frame, activation, err
	}
	var thisObj, player, programmer, verbLoc types.ObjID
	var unused1, unused2, unused3, unused4, debug int
	if _, err := fmt.Sscanf(
		verbRef,
		"%d %d %d %d %d %d %d %d %d",
		&thisObj, &unused1, &unused2, &player, &unused3,
		&programmer, &verbLoc, &unused4, &debug,
	); err != nil {
		return frame, activation, fmt.Errorf("parse activation verbref %q: %w", verbRef, err)
	}
	if id, ok := asObjID(verbLocValue); ok {
		verbLoc = id
	}
	for i := 0; i < 4; i++ {
		if _, err := readLine(r); err != nil {
			return frame, activation, fmt.Errorf("read activation placeholder %d: %w", i, err)
		}
	}
	verb, err := readLine(r)
	if err != nil {
		return frame, activation, err
	}
	storedVerb, err := readLine(r)
	if err != nil {
		return frame, activation, err
	}

	metadata, err := database.readValue(r)
	if err != nil {
		return frame, activation, fmt.Errorf("read activation metadata: %w", err)
	}
	pcLine, err := readLine(r)
	if err != nil {
		return frame, activation, err
	}
	var pc, builtinPC, errorPC int
	n, scanErr := fmt.Sscanf(pcLine, "%d %d %d", &pc, &builtinPC, &errorPC)
	if n < 2 || (scanErr != nil && scanErr != io.EOF) {
		return frame, activation, fmt.Errorf("parse activation PC %q", pcLine)
	}
	if n == 2 {
		errorPC = pc
	}
	var nativeBuiltin *task.NativeBuiltinSnapshot
	if builtinPC != 0 {
		nativeBuiltin, err = database.readNativeBuiltin(r)
		if err != nil {
			return frame, activation, err
		}
	}

	runtimeStack := frame.Stack
	isBarnFrame := metadata.Type() == types.TYPE_LIST &&
		metadata.Len() >= 1 &&
		metadata.Get(1).Type() == types.TYPE_STR &&
		(metadata.Get(1).Str() == barnVMFrameMarker || metadata.Get(1).Str() == barnVMFrameMarkerV1)
	if isBarnFrame {
		frame, err = decodeVMFrameMetadata(metadata, pc)
		if err != nil {
			return frame, activation, err
		}
		if !frame.Rebuild.Legacy {
			// The program is rebuilt at restore; until then the frame holds
			// its named locals in saved order, as a Toast-shaped frame does.
			frame.Program.VarNames = append([]string(nil), envNames...)
			frame.Program.NumLocals = len(envNames)
		}
	} else {
		frame.Native = &task.NativeFrameSnapshot{
			LanguageVersion: languageVersion,
			Programmer:      programmer,
			ErrorPC:         errorPC,
			OperandTypes:    operandTypes,
			Sentinel:        sentinel,
			Temporary:       metadata,
			Builtin:         nativeBuiltin,
		}
		frame.BuiltinPC = builtinPC
		frame.Program = bytecode.Program{
			VarNames:  append([]string(nil), envNames...),
			NumLocals: len(envNames),
		}
		frame.ThreadMode = threaded != 0
	}
	// Neither encoding stores built-in slots; derive them from the names the
	// way the compiler does (compiler/lower_locals.go declareVariable, which
	// lowercases the upper-cased canonical identifier).
	for index, name := range frame.Program.VarNames {
		frame.Program.BuiltinSlots.Set(strings.ToLower(strings.ToUpper(name)), index)
	}
	frame.Stack = runtimeStack
	frame.Program.Source = source
	frame.IP = pc
	frame.This = thisObj
	frame.ThisValue = thisValue
	frame.Player = player
	frame.Verb = verb
	frame.StoredVerb = storedVerb
	frame.VerbLoc = verbLoc
	frame.VerbDebug = debug != 0
	// A v1 Barn frame arrives with its slots (and restored temporaries)
	// already allocated by decodeVMFrameMetadata; other frames only have names.
	if len(frame.Locals) != frame.Program.NumLocals {
		frame.Locals = make([]types.Value, frame.Program.NumLocals)
		for i := range frame.Locals {
			frame.Locals[i] = types.Unbound
		}
	}
	for i, name := range envNames {
		for localIndex, declared := range frame.Program.VarNames {
			if declared == name && localIndex < len(frame.Locals) {
				frame.Locals[localIndex] = envValues[i]
				break
			}
		}
	}

	variablePairs := make([][2]types.Value, len(envNames))
	for i, name := range envNames {
		variablePairs[i] = [2]types.Value{types.NewStr(name), envValues[i]}
	}
	line := frame.Program.LineForIP(max(pc-1, 0))
	if line < 1 {
		line = 1
	}
	activation = types.ActivationFrame{
		This:             thisObj,
		ThisValue:        thisValue,
		Player:           player,
		Programmer:       programmer,
		Caller:           frame.Caller,
		Verb:             verb,
		StoredVerb:       storedVerb,
		VerbLoc:          verbLoc,
		Args:             append([]types.Value(nil), frame.Args...),
		LineNumber:       line,
		RuntimeVariables: types.NewMap(variablePairs),
		IsEvalFrame:      frame.IsEvalFrame,
		BuiltinCaller:    frame.BuiltinCaller,
		BuiltinPC:        frame.BuiltinPC,
	}
	return frame, activation, nil
}

func (database *Database) readOrderedRtEnv(r *bufio.Reader) ([]string, []types.Value, error) {
	header, err := readLine(r)
	if err != nil {
		return nil, nil, err
	}
	var count int
	if _, err := fmt.Sscanf(header, "%d variables", &count); err != nil {
		return nil, nil, fmt.Errorf("parse runtime environment header %q: %w", header, err)
	}
	if count < 0 {
		return nil, nil, fmt.Errorf("invalid runtime variable count %d", count)
	}
	names := make([]string, 0, min(count, maxTrustedCount))
	values := make([]types.Value, 0, min(count, maxTrustedCount))
	for i := 0; i < count; i++ {
		name, err := readLine(r)
		if err != nil {
			return nil, nil, err
		}
		value, err := database.readValue(r)
		if err != nil {
			return nil, nil, err
		}
		names = append(names, name)
		values = append(values, value)
	}
	return names, values, nil
}

// frameFields reads Barn frame metadata by v1 element number. A v2 frame
// folds v1 elements 2-6 (the bytecode) into its program identity element 2,
// so its elements from v1's 7 on sit four places earlier.
type frameFields struct {
	value types.Value
	shift int
}

func (f frameFields) index(n int) int {
	if n >= 7 {
		return n - f.shift
	}
	return n
}

func (f frameFields) has(n int) bool        { return f.value.Len() >= f.index(n) }
func (f frameFields) get(n int) types.Value { return f.value.Get(f.index(n)) }

func decodeVMFrameMetadata(metadata types.Value, pc int) (task.VMFrameSnapshot, error) {
	var frame task.VMFrameSnapshot
	if metadata.Type() != types.TYPE_LIST || metadata.Len() < 1 || metadata.Get(1).Type() != types.TYPE_STR {
		return frame, fmt.Errorf("suspended activation lacks Barn VM metadata")
	}
	f := frameFields{value: metadata}
	switch metadata.Get(1).Str() {
	case barnVMFrameMarkerV1:
		if metadata.Len() < 15 {
			return frame, fmt.Errorf("suspended activation lacks Barn VM metadata")
		}
		frame.Rebuild = &task.RebuildSnapshot{Legacy: true}
	case barnVMFrameMarker:
		// The v2 writer always writes every element.
		f.shift = 4
		if metadata.Len() != f.index(24) {
			return frame, fmt.Errorf("suspended activation has %d metadata elements, want %d", metadata.Len(), f.index(24))
		}
		frame.Rebuild = &task.RebuildSnapshot{}
	default:
		return frame, fmt.Errorf("suspended activation lacks Barn VM metadata")
	}
	if err := checkVMFrameShape(f); err != nil {
		return frame, err
	}

	for index := range frame.CommandEnvironment {
		frame.CommandEnvironment[index] = types.NewStr("")
		if index >= 4 {
			frame.CommandEnvironment[index] = types.NewObj(types.ObjNothing)
		}
	}
	if f.has(21) {
		environment := f.get(21)
		if environment.Type() != types.TYPE_LIST || environment.Len() != len(frame.CommandEnvironment) {
			return frame, fmt.Errorf("invalid command environment")
		}
		copy(frame.CommandEnvironment[:], environment.Elements())
	}
	if f.has(22) {
		local := f.get(22)
		if local.Type() != types.TYPE_INT || local.Int() < 0 || local.Int() > bytecode.MaxLocals {
			return frame, fmt.Errorf("invalid caller result local")
		}
		frame.ReturnLocal = int(local.Int())
	}
	if f.has(23) {
		builtin := f.get(23)
		if builtin.Type() != types.TYPE_LIST || builtin.Len() != 2 || builtin.Get(1).Type() != types.TYPE_STR || builtin.Get(2).Type() != types.TYPE_INT {
			return frame, fmt.Errorf("invalid builtin continuation")
		}
		frame.BuiltinCaller, frame.BuiltinPC = builtin.Get(1).Str(), int(builtin.Get(2).Int())
	}
	if f.has(20) {
		encoded := f.get(20)
		if encoded.Type() != types.TYPE_STR {
			return frame, fmt.Errorf("invalid builtin layout fingerprint")
		}
		layout, err := hex.DecodeString(encoded.Str())
		if err != nil || len(layout) != len(frame.Program.BuiltinLayout) {
			return frame, fmt.Errorf("invalid builtin layout fingerprint")
		}
		copy(frame.Program.BuiltinLayout[:], layout)
	}
	var err error
	if frame.Rebuild.Legacy {
		err = decodeLegacyProgram(&frame, f)
	} else {
		err = decodeProgramIdentity(&frame, f)
	}
	if err != nil {
		return frame, err
	}
	if err := decodeFrameState(&frame, f, pc); err != nil {
		return frame, err
	}
	return frame, nil
}

// decodeProgramIdentity reads a v2 frame's program identity (element 2,
// written by programIdentityValue) and its bound compiler temporaries.
func decodeProgramIdentity(frame *task.VMFrameSnapshot, f frameFields) error {
	identity := f.get(2)
	if identity.Type() != types.TYPE_LIST || identity.Len() != 3 || identity.Get(1).Type() != types.TYPE_INT ||
		identity.Get(2).Type() != types.TYPE_LIST || identity.Get(3).Type() != types.TYPE_STR {
		return fmt.Errorf("suspended activation has invalid program identity")
	}
	fork := identity.Get(1).Int()
	if fork < 0 || fork > math.MaxInt32 {
		return fmt.Errorf("suspended activation has invalid fork index %d", fork)
	}
	frame.Program.Origin.Fork = int(fork)
	native := identity.Get(2)
	switch native.Len() {
	case 0:
	case 2:
		vector, version := native.Get(1), native.Get(2)
		if vector.Type() != types.TYPE_INT || version.Type() != types.TYPE_INT || vector.Int() < -1 || vector.Int() > math.MaxInt32 || version.Int() < 0 || version.Int() > math.MaxInt32 {
			return fmt.Errorf("suspended activation has invalid native origin")
		}
		frame.Program.Origin.Native = true
		frame.Program.Origin.NativeVector = int(vector.Int())
		frame.Program.Origin.LanguageVersion = int(version.Int())
	default:
		return fmt.Errorf("suspended activation has invalid native origin")
	}
	fingerprint, err := hex.DecodeString(identity.Get(3).Str())
	if err != nil || len(fingerprint) != len(frame.Rebuild.Fingerprint) {
		return fmt.Errorf("suspended activation has invalid program fingerprint")
	}
	copy(frame.Rebuild.Fingerprint[:], fingerprint)
	return readInternalLocals(f, func(slot int, value types.Value) error {
		if slot < 0 || slot >= bytecode.MaxLocals {
			return fmt.Errorf("suspended activation internal local slot %d outside [0, %d)", slot, bytecode.MaxLocals)
		}
		frame.Rebuild.Internal = append(frame.Rebuild.Internal, task.LocalSlot{Slot: slot, Value: value})
		return nil
	})
}

// readInternalLocals visits element 19, written by internalLocalsValue;
// checkpoints predating that element leave the temporaries unbound.
func readInternalLocals(f frameFields, visit func(slot int, value types.Value) error) error {
	if !f.has(19) {
		return nil
	}
	internal := f.get(19)
	if internal.Type() != types.TYPE_LIST {
		return fmt.Errorf("suspended activation internal locals must be a list")
	}
	for i := 1; i <= internal.Len(); i++ {
		entry := internal.Get(i)
		if entry.Type() != types.TYPE_LIST || entry.Len() != 2 || entry.Get(1).Type() != types.TYPE_INT {
			return fmt.Errorf("suspended activation internal local %d is malformed", i)
		}
		if err := visit(int(entry.Get(1).Int()), entry.Get(2)); err != nil {
			return err
		}
	}
	return nil
}

// decodeLegacyProgram reads a v1 frame's stored bytecode (elements 2-6) and
// places its locals by slot. Restore accepts it only when it equals a
// compile of the frame's source.
func decodeLegacyProgram(frame *task.VMFrameSnapshot, f frameFields) error {
	code := f.get(2)
	frame.Program.Code = make([]byte, code.Len())
	for i := 1; i <= code.Len(); i++ {
		frame.Program.Code[i-1] = byte(code.Get(i).Int())
	}
	frame.Program.Constants = append([]types.Value(nil), f.get(3).Elements()...)
	names := f.get(4)
	frame.Program.VarNames = make([]string, names.Len())
	for i := 1; i <= names.Len(); i++ {
		frame.Program.VarNames[i-1] = names.Get(i).Str()
	}
	lineInfo := f.get(5)
	frame.Program.LineInfo = make([]bytecode.LineEntry, lineInfo.Len())
	for i := 1; i <= lineInfo.Len(); i++ {
		entry := lineInfo.Get(i)
		frame.Program.LineInfo[i-1] = bytecode.LineEntry{
			StartIP: int(entry.Get(1).Int()),
			Line:    int(entry.Get(2).Int()),
		}
	}
	frame.Program.NumLocals = int(f.get(6).Int())
	if frame.Program.NumLocals < len(frame.Program.VarNames) || frame.Program.NumLocals > bytecode.MaxLocals {
		return fmt.Errorf("suspended activation has invalid local metadata: %d locals, %d names", frame.Program.NumLocals, len(frame.Program.VarNames))
	}
	// Compiler temporaries live above the named variables.
	frame.Locals = make([]types.Value, frame.Program.NumLocals)
	for i := range frame.Locals {
		frame.Locals[i] = types.Unbound
	}
	return readInternalLocals(f, func(slot int, value types.Value) error {
		if slot < len(frame.Program.VarNames) || slot >= frame.Program.NumLocals {
			return fmt.Errorf("suspended activation internal local slot %d outside [%d, %d)", slot, len(frame.Program.VarNames), frame.Program.NumLocals)
		}
		frame.Locals[slot] = value
		return nil
	})
}

// decodeFrameState reads the activation state every Barn frame version
// shares: handlers, flags, call context, continuations, pending reasons.
func decodeFrameState(frame *task.VMFrameSnapshot, f frameFields, pc int) error {
	handlers := f.get(7)
	frame.ExceptStack = make([]bytecode.Handler, handlers.Len())
	for i := 1; i <= handlers.Len(); i++ {
		saved := handlers.Get(i)
		codes := saved.Get(4)
		handler := bytecode.Handler{
			Type:       bytecode.HandlerType(saved.Get(1).Int()),
			HandlerIP:  int(saved.Get(2).Int()),
			EndIP:      int(saved.Get(3).Int()),
			Codes:      make([]types.ErrorCode, codes.Len()),
			VarIndex:   int(saved.Get(5).Int()),
			StackDepth: int(saved.Get(6).Int()),
		}
		if saved.Len() >= 7 {
			if saved.Get(7).Type() != types.TYPE_INT {
				return fmt.Errorf("invalid handler group base")
			}
			handler.ExceptBase = int(saved.Get(7).Int())
		}
		for j := 1; j <= codes.Len(); j++ {
			handler.Codes[j-1] = types.ErrorCode(codes.Get(j).Int())
		}
		frame.ExceptStack[i-1] = handler
	}

	legacyError, err := decodePendingError(f.get(8))
	if err != nil {
		return err
	}
	flags := f.get(9)
	frame.VerbDebug = flags.Get(1).Truthy()
	frame.DiscardReturn = flags.Get(2).Truthy()
	frame.IsVerbCall = flags.Get(3).Truthy()
	frame.IsEvalFrame = flags.Get(4).Truthy()
	frame.SavedIsWizard = flags.Get(5).Truthy()
	// Barn frames written before the activation's thread mode was recorded
	// carry a constant 0 threaded flag; they ran in Toast's default mode.
	frame.ThreadMode = flags.Len() < 6 || flags.Get(6).Truthy()
	frame.Caller = f.get(10).Obj()
	frame.Args = append([]types.Value(nil), f.get(11).Elements()...)
	frame.SavedThisObj = f.get(12).Obj()
	frame.SavedThisValue = f.get(13)
	frame.SavedVerb = f.get(14).Str()
	frame.SavedProgrammer = f.get(15).Obj()
	if f.has(16) {
		moveState := f.get(16)
		if moveState.Type() == types.TYPE_LIST && moveState.Len() >= 6 {
			frame.MoveContinuation = &task.MoveContinuationSnapshot{
				Stage:         int(moveState.Get(1).Int()),
				What:          moveState.Get(2),
				Where:         moveState.Get(3),
				OldLocation:   moveState.Get(4),
				Position:      moveState.Get(5).Int(),
				Decentralized: moveState.Get(6).Truthy(),
			}
		}
	}
	var legacyReturn types.Value
	hasLegacyReturn := false
	if f.has(17) {
		pendingReturn := f.get(17)
		if pendingReturn.Type() == types.TYPE_LIST && pendingReturn.Len() >= 2 {
			hasLegacyReturn = pendingReturn.Get(1).Truthy()
			legacyReturn = pendingReturn.Get(2)
		}
	}
	if f.has(18) {
		recycleState := f.get(18)
		if recycleState.Type() == types.TYPE_LIST && recycleState.Len() >= 5 {
			frame.RecycleContinuation = &task.RecycleContinuationSnapshot{
				Object:      recycleState.Get(1),
				OldParents:  objectIDsFromValue(recycleState.Get(2)),
				OldChildren: objectIDsFromValue(recycleState.Get(3)),
				OldContents: objectIDsFromValue(recycleState.Get(4)),
				OldLocation: recycleState.Get(5).Obj(),
			}
			if recycleState.Len() >= 6 {
				frame.RecycleContinuation.ExitCallback = recycleState.Get(6).Truthy()
			}
		}
	}
	frame.IP = pc
	return decodePendingReasons(frame, f, legacyError, hasLegacyReturn, legacyReturn)
}

func decodePendingError(value types.Value) (task.VMErrorSnapshot, error) {
	var pending task.VMErrorSnapshot
	if value.Type() != types.TYPE_LIST || (value.Len() != 3 && value.Len() != 4) {
		return pending, fmt.Errorf("invalid pending error metadata")
	}
	pending = task.VMErrorSnapshot{Present: value.Get(1).Truthy(), Code: types.ErrorCode(value.Get(2).Int()), Value: value.Get(3)}
	if value.Len() == 4 {
		reason := value.Get(4)
		if reason.Type() != types.TYPE_INT || reason.Int() < 0 || reason.Int() > int64(types.ExceptionReasonSecondsBudget) {
			return pending, fmt.Errorf("invalid pending error reason")
		}
		pending.Reason = types.ExceptionReason(reason.Int())
	}
	if !pending.ValidExceptionReason() {
		return pending, fmt.Errorf("invalid pending error reason")
	}
	return pending, nil
}

func decodePendingReasons(frame *task.VMFrameSnapshot, f frameFields, legacyError task.VMErrorSnapshot, hasReturn bool, legacyReturn types.Value) error {
	returnOwner, errorOwner := 0, 0
	canonical := false
	if f.has(24) {
		control := f.get(24)
		if control.Type() != types.TYPE_LIST {
			return fmt.Errorf("invalid pending control state")
		}
		canonical = control.Len() == 2 && control.Get(1).Type() == types.TYPE_STR && control.Get(1).Str() == "pending-reasons-v1"
		if canonical {
			if legacyError.Present || hasReturn || control.Get(2).Type() != types.TYPE_LIST {
				return fmt.Errorf("mixed or invalid pending control state")
			}
			for _, row := range control.Get(2).Elements() {
				if row.Type() != types.TYPE_LIST || row.Len() != 7 || row.Get(1).Type() != types.TYPE_INT || row.Get(2).Type() != types.TYPE_INT || row.Get(5).Type() != types.TYPE_INT || row.Get(6).Type() != types.TYPE_INT || row.Get(7).Type() != types.TYPE_LIST {
					return fmt.Errorf("invalid pending reason")
				}
				pendingError, err := decodePendingError(row.Get(4))
				if err != nil {
					return err
				}
				kind := task.PendingReasonKind(row.Get(1).Int())
				if row.Get(1).Int() < int64(task.PendingReturn) || row.Get(1).Int() > int64(task.PendingJump) || (kind == task.PendingError) != pendingError.Present {
					return fmt.Errorf("invalid pending reason kind or error")
				}
				frame.PendingReasons = append(frame.PendingReasons, task.PendingReasonSnapshot{Kind: kind, FinallyIP: int(row.Get(2).Int()), Value: row.Get(3), Error: pendingError, TargetIP: int(row.Get(5).Int()), HandlerDepth: int(row.Get(6).Int()), Stack: append([]types.Value(nil), row.Get(7).Elements()...)})
			}
		} else {
			if control.Len() != 3 || control.Get(1).Type() != types.TYPE_INT || control.Get(2).Type() != types.TYPE_INT || control.Get(3).Type() != types.TYPE_LIST {
				return fmt.Errorf("invalid legacy pending control state")
			}
			returnOwner, errorOwner = int(control.Get(1).Int()), int(control.Get(2).Int())
			for _, row := range control.Get(3).Elements() {
				if row.Type() != types.TYPE_LIST || row.Len() != 4 || row.Get(1).Type() != types.TYPE_INT || row.Get(2).Type() != types.TYPE_INT || row.Get(3).Type() != types.TYPE_INT || row.Get(4).Type() != types.TYPE_LIST {
					return fmt.Errorf("invalid legacy pending jump")
				}
				frame.PendingReasons = append(frame.PendingReasons, task.PendingReasonSnapshot{Kind: task.PendingJump, TargetIP: int(row.Get(1).Int()), HandlerDepth: int(row.Get(2).Int()), FinallyIP: int(row.Get(3).Int()), Stack: append([]types.Value(nil), row.Get(4).Elements()...)})
			}
		}
	}
	if !frame.Rebuild.Legacy {
		// The v2 writer always writes canonical reasons. Their cleanup owners
		// are checked against the rebuilt program by vm.RestoreVMSnapshot.
		if !canonical {
			return fmt.Errorf("suspended activation lacks canonical pending reasons")
		}
		return nil
	}
	if !canonical {
		if legacyError.Present {
			frame.PendingReasons = append(frame.PendingReasons, task.PendingReasonSnapshot{Kind: task.PendingError, FinallyIP: errorOwner, Error: legacyError})
		}
		if hasReturn {
			frame.PendingReasons = append(frame.PendingReasons, task.PendingReasonSnapshot{Kind: task.PendingReturn, FinallyIP: returnOwner, Value: legacyReturn})
		}
		for index := range frame.PendingReasons {
			pending := &frame.PendingReasons[index]
			if pending.FinallyIP == 0 {
				owners := frame.Program.FinallyOwnersAt(frame.IP)
				if len(owners) != 1 {
					return fmt.Errorf("unsupported legacy pending reason: ambiguous cleanup ownership")
				}
				pending.FinallyIP = owners[0]
			}
		}
		// Order legacy records only by proven cleanup containment.
		for i := 1; i < len(frame.PendingReasons); i++ {
			for j := i; j > 0; j-- {
				left, right := frame.PendingReasons[j-1].FinallyIP, frame.PendingReasons[j].FinallyIP
				if left != right && frame.Program.FinallyContains(right, left) {
					frame.PendingReasons[j-1], frame.PendingReasons[j] = frame.PendingReasons[j], frame.PendingReasons[j-1]
				} else {
					break
				}
			}
		}
	}
	for index, pending := range frame.PendingReasons {
		if !frame.Program.IsInstructionBoundary(pending.FinallyIP) || !frame.Program.FinallyContains(pending.FinallyIP, frame.IP) {
			return fmt.Errorf("pending reason has no containing cleanup")
		}
		if index > 0 {
			previous := frame.PendingReasons[index-1].FinallyIP
			if previous == pending.FinallyIP || !frame.Program.FinallyContains(previous, pending.FinallyIP) {
				return fmt.Errorf("unsupported pending reason history: ambiguous cleanup order")
			}
		}
	}
	return nil
}

// checkVMFrameShape rejects frame metadata whose fields decodeVMFrameMetadata
// indexes without its own type check. Get, Elements and Str panic on a value of
// another type, and Len counts a string's bytes or a map's entries, so each
// field read element by element must be a list long enough for its reads.
// It is a list of at least 15 v1 elements. Elements 2-5 hold bytecode only in
// a v1 frame; decodeProgramIdentity checks a v2 frame's element 2.
func checkVMFrameShape(f frameFields) error {
	isList := func(v types.Value, minLen int) bool {
		return v.Type() == types.TYPE_LIST && v.Len() >= minLen
	}
	eachList := func(v types.Value, ok func(types.Value) bool) bool {
		if !isList(v, 0) {
			return false
		}
		for _, element := range v.Elements() {
			if !ok(element) {
				return false
			}
		}
		return true
	}
	checks := []struct {
		field int
		name  string
		ok    func(types.Value) bool
	}{
		{2, "code", func(v types.Value) bool { return isList(v, 0) }},
		{3, "constants", func(v types.Value) bool { return isList(v, 0) }},
		{4, "variable names", func(v types.Value) bool {
			return eachList(v, func(e types.Value) bool { return e.Type() == types.TYPE_STR })
		}},
		{5, "line info", func(v types.Value) bool {
			return eachList(v, func(e types.Value) bool { return isList(e, 2) })
		}},
		{7, "handlers", func(v types.Value) bool {
			return eachList(v, func(e types.Value) bool { return isList(e, 6) && isList(e.Get(4), 0) })
		}},
		{9, "flags", func(v types.Value) bool { return isList(v, 5) }},
		{11, "arguments", func(v types.Value) bool { return isList(v, 0) }},
		{14, "verb", func(v types.Value) bool { return v.Type() == types.TYPE_STR }},
	}
	for _, check := range checks {
		if check.field < 7 && f.shift != 0 {
			continue
		}
		if !check.ok(f.get(check.field)) {
			return fmt.Errorf("suspended activation has invalid %s metadata", check.name)
		}
	}
	if f.has(18) {
		recycleState := f.get(18)
		if isList(recycleState, 5) {
			for i := 2; i <= 4; i++ {
				if !isList(recycleState.Get(i), 0) {
					return fmt.Errorf("suspended activation has invalid recycle continuation metadata")
				}
			}
		}
	}
	return nil
}

func objectIDsFromValue(value types.Value) []types.ObjID {
	ids := make([]types.ObjID, value.Len())
	for i := 1; i <= value.Len(); i++ {
		ids[i-1] = value.Get(i).Obj()
	}
	return ids
}
