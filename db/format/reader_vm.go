package format

import (
	"bufio"
	"fmt"
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
	_ = rootVector
	if top < 0 {
		return nil, fmt.Errorf("invalid top activation %d", top)
	}

	vmSnapshot := &task.VMSnapshot{
		MaxStackDepth: maxStack,
		Frames:        make([]task.VMFrameSnapshot, 0, top+1),
	}
	callStack := make([]task.ActivationFrame, 0, top+1)
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
		ID:         id,
		Owner:      root.Player,
		State:      state,
		StartTime:  time.Unix(start, 0),
		WakeValue:  wakeValue,
		TaskLocal:  taskLocal,
		CallStack:  callStack,
		Programmer: root.Programmer,
		VerbLoc:    root.VerbLoc,
		VerbName:   root.Verb,
		This:       root.This,
		VM:         vmSnapshot,
	}}, nil
}

func (database *Database) readVMFrame(r *bufio.Reader) (task.VMFrameSnapshot, task.ActivationFrame, error) {
	var frame task.VMFrameSnapshot
	var activation task.ActivationFrame

	version, err := readLine(r)
	if err != nil {
		return frame, activation, err
	}
	if !strings.HasPrefix(version, "language version ") {
		return frame, activation, fmt.Errorf("expected language version, got %q", version)
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
	frame.Stack = make([]types.Value, stackCount)
	for i := range frame.Stack {
		frame.Stack[i], err = database.readValue(r)
		if err != nil {
			return frame, activation, fmt.Errorf("read runtime stack value %d: %w", i, err)
		}
	}

	if _, err := database.readValue(r); err != nil {
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
	if _, err := readLine(r); err != nil {
		return frame, activation, fmt.Errorf("read activation thread mode: %w", err)
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
	if n, scanErr := fmt.Sscanf(pcLine, "%d %d %d", &pc, &builtinPC, &errorPC); scanErr != nil || n < 2 {
		return frame, activation, fmt.Errorf("parse activation PC %q", pcLine)
	}
	_ = errorPC
	if builtinPC != 0 {
		if err := database.skipBiFuncData(r); err != nil {
			return frame, activation, err
		}
	}

	runtimeStack := frame.Stack
	isBarnFrame := metadata.Type() == types.TYPE_LIST &&
		metadata.Len() >= 1 &&
		metadata.Get(1).Type() == types.TYPE_STR &&
		metadata.Get(1).Str() == barnVMFrameMarker
	if isBarnFrame {
		frame, err = decodeVMFrameMetadata(metadata)
		if err != nil {
			return frame, activation, err
		}
	} else {
		frame.Program = bytecode.Program{
			VarNames:  append([]string(nil), envNames...),
			NumLocals: len(envNames),
		}
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
	frame.Locals = make([]types.Value, frame.Program.NumLocals)
	for i := range frame.Locals {
		frame.Locals[i] = types.Unbound
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
	activation = task.ActivationFrame{
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
	names := make([]string, count)
	values := make([]types.Value, count)
	for i := 0; i < count; i++ {
		names[i], err = readLine(r)
		if err != nil {
			return nil, nil, err
		}
		values[i], err = database.readValue(r)
		if err != nil {
			return nil, nil, err
		}
	}
	return names, values, nil
}

func decodeVMFrameMetadata(value types.Value) (task.VMFrameSnapshot, error) {
	var frame task.VMFrameSnapshot
	if value.Type() != types.TYPE_LIST || value.Len() < 15 || value.Get(1).Type() != types.TYPE_STR || value.Get(1).Str() != barnVMFrameMarker {
		return frame, fmt.Errorf("suspended activation lacks Barn VM metadata")
	}

	code := value.Get(2)
	frame.Program.Code = make([]byte, code.Len())
	for i := 1; i <= code.Len(); i++ {
		frame.Program.Code[i-1] = byte(code.Get(i).Int())
	}
	frame.Program.Constants = append([]types.Value(nil), value.Get(3).Elements()...)
	names := value.Get(4)
	frame.Program.VarNames = make([]string, names.Len())
	for i := 1; i <= names.Len(); i++ {
		frame.Program.VarNames[i-1] = names.Get(i).Str()
	}
	lineInfo := value.Get(5)
	frame.Program.LineInfo = make([]bytecode.LineEntry, lineInfo.Len())
	for i := 1; i <= lineInfo.Len(); i++ {
		entry := lineInfo.Get(i)
		frame.Program.LineInfo[i-1] = bytecode.LineEntry{
			StartIP: int(entry.Get(1).Int()),
			Line:    int(entry.Get(2).Int()),
		}
	}
	frame.Program.NumLocals = int(value.Get(6).Int())

	handlers := value.Get(7)
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
		for j := 1; j <= codes.Len(); j++ {
			handler.Codes[j-1] = types.ErrorCode(codes.Get(j).Int())
		}
		frame.ExceptStack[i-1] = handler
	}

	pending := value.Get(8)
	frame.PendingError = task.VMErrorSnapshot{
		Present: pending.Get(1).Truthy(),
		Code:    types.ErrorCode(pending.Get(2).Int()),
		Value:   pending.Get(3),
	}
	flags := value.Get(9)
	frame.VerbDebug = flags.Get(1).Truthy()
	frame.DiscardReturn = flags.Get(2).Truthy()
	frame.IsVerbCall = flags.Get(3).Truthy()
	frame.IsEvalFrame = flags.Get(4).Truthy()
	frame.SavedIsWizard = flags.Get(5).Truthy()
	frame.Caller = value.Get(10).Obj()
	frame.Args = append([]types.Value(nil), value.Get(11).Elements()...)
	frame.SavedThisObj = value.Get(12).Obj()
	frame.SavedThisValue = value.Get(13)
	frame.SavedVerb = value.Get(14).Str()
	frame.SavedProgrammer = value.Get(15).Obj()
	if value.Len() >= 16 {
		moveState := value.Get(16)
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
	return frame, nil
}
