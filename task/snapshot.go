package task

import (
	"time"

	"barn/bytecode"
	"barn/types"
)

// ForkSnapshot is the persistence-relevant portion of a forked task.
type ForkSnapshot struct {
	Variables     map[string]types.Value
	VariableNames []string
	SourceLines   []string
	FirstLine     int
}

// Snapshot is an immutable copy of the task fields the database writer needs.
type Snapshot struct {
	ID              int64
	Owner           types.ObjID
	State           TaskState
	StartTime       time.Time
	WakeValue       types.Value
	TaskLocal       types.Value
	CallStack       []ActivationFrame
	Fork            *ForkSnapshot
	Programmer      types.ObjID
	VerbLoc         types.ObjID
	VerbName        string
	This            types.ObjID
	ReadingPlayer   types.ObjID
	IsExecSuspended bool
	ExecCommandName string
	VM              *VMSnapshot
}

// VMSnapshot is the complete resumable state of a yielded bytecode VM.
type VMSnapshot struct {
	MaxStackDepth int
	Frames        []VMFrameSnapshot
}

// VMFrameSnapshot is one activation and its operand-stack segment.
type VMFrameSnapshot struct {
	Program         bytecode.Program
	IP              int
	Locals          []types.Value
	Stack           []types.Value
	This            types.ObjID
	ThisValue       types.Value
	Player          types.ObjID
	Verb            string
	StoredVerb      string
	Caller          types.ObjID
	VerbLoc         types.ObjID
	Args            []types.Value
	ExceptStack     []bytecode.Handler
	PendingError    VMErrorSnapshot
	VerbDebug       bool
	DiscardReturn   bool
	IsVerbCall      bool
	IsEvalFrame     bool
	SavedThisObj    types.ObjID
	SavedThisValue  types.Value
	SavedVerb       string
	SavedProgrammer types.ObjID
	SavedIsWizard   bool
}

// VMErrorSnapshot is an error held while a finally block is executing.
type VMErrorSnapshot struct {
	Present bool
	Code    types.ErrorCode
	Value   types.Value
}

type vmSnapshotter interface {
	PersistenceVMSnapshot() *VMSnapshot
}

// PersistenceSnapshot copies the task fields needed for checkpoint output while
// holding the task lock.
func (t *Task) PersistenceSnapshot() Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	snapshot := Snapshot{
		ID:              t.ID,
		Owner:           t.Owner,
		State:           t.State,
		StartTime:       t.StartTime,
		WakeValue:       t.WakeValue,
		TaskLocal:       t.TaskLocal,
		CallStack:       cloneActivationFrames(t.CallStack),
		Programmer:      t.Programmer,
		VerbLoc:         t.VerbLoc,
		VerbName:        t.VerbName,
		This:            t.This,
		ReadingPlayer:   t.ReadingPlayer,
		IsExecSuspended: t.IsExecSuspended,
		ExecCommandName: t.ExecCommandName,
	}
	if t.ForkInfo != nil {
		var variableNames []string
		firstLine := 0
		if t.Program != nil {
			variableNames = append(variableNames, t.Program.VarNames...)
			firstLine = t.Program.LineForIP(0)
		} else if body, ok := t.ForkInfo.Body.([3]interface{}); ok {
			if program, ok := body[0].(*bytecode.Program); ok {
				variableNames = append(variableNames, program.VarNames...)
				if bodyIP, ok := body[1].(int); ok {
					firstLine = program.LineForIP(bodyIP)
				}
			}
		}
		snapshot.Fork = &ForkSnapshot{
			Variables:     cloneValueMap(t.ForkInfo.Variables),
			VariableNames: variableNames,
			SourceLines:   append([]string(nil), t.ForkInfo.SourceLines...),
			FirstLine:     firstLine,
		}
	}
	if machine, ok := t.BytecodeVM.(vmSnapshotter); ok && machine != nil {
		snapshot.VM = machine.PersistenceVMSnapshot()
	}
	return snapshot
}

func cloneActivationFrames(frames []ActivationFrame) []ActivationFrame {
	if len(frames) == 0 {
		return nil
	}
	copied := make([]ActivationFrame, len(frames))
	for i, frame := range frames {
		copied[i] = frame
		copied[i].Args = append([]types.Value(nil), frame.Args...)
	}
	return copied
}

func cloneValueMap(values map[string]types.Value) map[string]types.Value {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]types.Value, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}
