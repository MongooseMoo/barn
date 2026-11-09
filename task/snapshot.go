package task

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
)

// CommandVariableNames are inherited even by verbs that do not reference them.
var CommandVariableNames = [...]string{"argstr", "dobjstr", "iobjstr", "prepstr", "dobj", "iobj"}

// RestoreForkEnvironment reorders the saved parent environment for a recompiled
// isolated fork body. Unmatched program locals remain unbound; unused parent
// locals disappear, while the implicit command environment is retained.
func RestoreForkEnvironment(program *bytecode.Program, saved map[string]types.Value) (map[string]types.Value, error) {
	lookup := func(name string) (types.Value, bool, error) {
		value, matched := types.Unbound, false
		for savedName, savedValue := range saved {
			if strings.EqualFold(savedName, name) {
				// Serialized order is unavailable in the reader's map. Do not
				// guess which ambiguous spelling native's first match would use.
				if matched {
					return types.None, false, fmt.Errorf("ambiguous saved local %q", name)
				}
				value, matched = savedValue, true
			}
		}
		return value, matched, nil
	}
	variables := make(map[string]types.Value, len(program.VarNames))
	for _, name := range program.VarNames {
		value, _, err := lookup(name)
		if err != nil {
			return nil, err
		}
		variables[name] = value
	}
	for _, name := range CommandVariableNames {
		if slices.ContainsFunc(program.VarNames, func(sourceName string) bool { return strings.EqualFold(name, sourceName) }) {
			continue
		}
		value, present, err := lookup(name)
		if err != nil {
			return nil, err
		}
		if present {
			variables[name] = value
		}
	}
	return variables, nil
}

// ForkSnapshot is the persistence-relevant portion of a forked task.
type ForkSnapshot struct {
	Variables     map[string]types.Value
	VariableNames []string
	SourceLines   []string
	FirstLine     int
}

// Snapshot is an immutable copy of the task fields the database writer needs.
type Snapshot struct {
	ID                  int64
	Owner               types.ObjID
	State               TaskState
	StartTime           time.Time
	WakeValue           types.Value
	TaskLocal           types.Value
	CallStack           []types.ActivationFrame
	Fork                *ForkSnapshot
	Programmer          types.ObjID
	VerbLoc             types.ObjID
	VerbName            string
	This                types.ObjID
	ReadingPlayer       types.ObjID
	IsExecSuspended     bool
	ExecCommandName     string
	IsHTTPReadSuspended bool
	VM                  *VMSnapshot
}

// VMSnapshot is the complete resumable state of a yielded bytecode VM.
type VMSnapshot struct {
	MaxStackDepth int
	Frames        []VMFrameSnapshot
	// NativeRootVector is meaningful only when Frames contain native metadata.
	// Toast uses -1 for its main vector and nonnegative fork-vector indexes.
	NativeRootVector int
	NativeFunctionID int
}

// NativeFrameSnapshot retains the state which cannot be interpreted as Barn
// bytecode until the compiler has converted a native call continuation.
// Source, named locals, operand values and PC remain in the owning frame;
// OperandTypes distinguishes native control-stack markers from MOO integers.
type NativeFrameSnapshot struct {
	LanguageVersion int
	Programmer      types.ObjID
	ErrorPC         int
	OperandTypes    []int
	Sentinel        types.Value
	Temporary       types.Value
	Builtin         *NativeBuiltinSnapshot
}

// NativeBuiltinSnapshot is Toast's recursively serialized builtin call state.
// State is kept verbatim until a builtin-specific converter validates it.
type NativeBuiltinSnapshot struct {
	Name  string
	State string
	Inner *NativeBuiltinSnapshot
}

// MoveContinuationSnapshot is the persistent native state retained while a
// lifecycle verb called by move() is suspended on the owning VM.
type MoveContinuationSnapshot struct {
	Stage         int
	What          types.Value
	Where         types.Value
	OldLocation   types.Value
	Position      int64
	Decentralized bool
}

// RecycleContinuationSnapshot is the persistent native state retained while
// an object's recycle verb is suspended on the owning VM.
type RecycleContinuationSnapshot struct {
	ExitCallback bool
	Object       types.Value
	OldParents   []types.ObjID
	OldChildren  []types.ObjID
	OldContents  []types.ObjID
	OldLocation  types.ObjID
}

type PendingReasonKind uint8

const (
	PendingReturn PendingReasonKind = iota + 1
	PendingError
	PendingJump
)

// PendingReasonSnapshot owns one unwind reason while its cleanup executes.
type PendingReasonSnapshot struct {
	Kind         PendingReasonKind
	Value        types.Value
	Error        VMErrorSnapshot
	TargetIP     int
	HandlerDepth int
	FinallyIP    int
	Stack        []types.Value
}

// VMFrameSnapshot is one activation and its operand-stack segment.
type VMFrameSnapshot struct {
	Native              *NativeFrameSnapshot
	Program             bytecode.Program
	IP                  int
	Locals              []types.Value
	CommandEnvironment  [6]types.Value
	Stack               []types.Value
	This                types.ObjID
	ThisValue           types.Value
	Player              types.ObjID
	Verb                string
	StoredVerb          string
	Caller              types.ObjID
	VerbLoc             types.ObjID
	Args                []types.Value
	ExceptStack         []bytecode.Handler
	PendingReasons      []PendingReasonSnapshot
	VerbDebug           bool
	DiscardReturn       bool
	ReturnLocal         int
	BuiltinCaller       string
	BuiltinPC           int
	IsVerbCall          bool
	IsEvalFrame         bool
	ThreadMode          bool // this activation's own set_thread_mode() state; Toast persists it as the threaded flag
	SavedThisObj        types.ObjID
	SavedThisValue      types.Value
	SavedVerb           string
	SavedProgrammer     types.ObjID
	SavedIsWizard       bool
	MoveContinuation    *MoveContinuationSnapshot
	RecycleContinuation *RecycleContinuationSnapshot
}

// VMErrorSnapshot is an error held while a finally block is executing.
type VMErrorSnapshot struct {
	Present bool
	Code    types.ErrorCode
	Value   types.Value
	Reason  types.ExceptionReason
}

// ValidExceptionReason rejects metadata that cannot originate at these guards.
// Legacy or manually raised errors have an unknown reason, for any error code.
func (s VMErrorSnapshot) ValidExceptionReason() bool {
	return s.Reason <= types.ExceptionReasonSecondsBudget &&
		(s.Reason == types.ExceptionReasonUnknown || (s.Present && s.Code == types.E_MAXREC))
}

// IsInterruptedForPersistence reports whether the database writer emits this
// VM snapshot in the interrupted-task section rather than the ordinary
// suspended-task section.
func (s Snapshot) IsInterruptedForPersistence() bool {
	return s.ReadingPlayer != types.ObjNothing || s.IsExecSuspended || s.IsHTTPReadSuspended
}

// TransformPersistenceValues applies transform to every typed value that the
// database writer emits for this queued or suspended task snapshot. Keeping the
// traversal here gives checkpoint reachability seeding and reference rewriting
// one definition of the task persistence surface.
func (s *Snapshot) TransformPersistenceValues(transform func(types.Value) types.Value) {
	if s == nil || transform == nil {
		return
	}

	if s.VM == nil {
		if s.Fork != nil {
			for name, value := range s.Fork.Variables {
				s.Fork.Variables[name] = transform(value)
			}
			if len(s.CallStack) > 0 {
				s.CallStack[0].ThisValue = transform(s.CallStack[0].ThisValue)
			}
		}
		return
	}
	if !s.IsInterruptedForPersistence() {
		s.WakeValue = transform(s.WakeValue)
	}
	s.TaskLocal = transform(s.TaskLocal)
	for frameIndex := range s.VM.Frames {
		frame := &s.VM.Frames[frameIndex]
		if frame.Native != nil {
			frame.Native.Sentinel = transform(frame.Native.Sentinel)
			frame.Native.Temporary = transform(frame.Native.Temporary)
		}
		for index, value := range frame.Program.Constants {
			frame.Program.Constants[index] = transform(value)
		}
		for index, value := range frame.Locals {
			frame.Locals[index] = transform(value)
		}
		for index, value := range frame.Stack {
			frame.Stack[index] = transform(value)
		}
		frame.ThisValue = transform(frame.ThisValue)
		for index, value := range frame.CommandEnvironment {
			frame.CommandEnvironment[index] = transform(value)
		}
		for index, value := range frame.Args {
			frame.Args[index] = transform(value)
		}
		for pendingIndex := range frame.PendingReasons {
			pending := &frame.PendingReasons[pendingIndex]
			pending.Value = transform(pending.Value)
			pending.Error.Value = transform(pending.Error.Value)
			for valueIndex, value := range pending.Stack {
				pending.Stack[valueIndex] = transform(value)
			}
		}
		frame.SavedThisValue = transform(frame.SavedThisValue)
		if frame.MoveContinuation != nil {
			frame.MoveContinuation.What = transform(frame.MoveContinuation.What)
			frame.MoveContinuation.Where = transform(frame.MoveContinuation.Where)
			frame.MoveContinuation.OldLocation = transform(frame.MoveContinuation.OldLocation)
		}
		if frame.RecycleContinuation != nil {
			frame.RecycleContinuation.Object = transform(frame.RecycleContinuation.Object)
		}
	}
}

// PersistenceSnapshotLocked copies task metadata for checkpoint output. The
// caller holds Mu or owns unpublished state; the engine captures its saved
// interpreter under the same lock.
func (t *State) PersistenceSnapshotLocked() Snapshot {
	snapshot := Snapshot{
		ID:                  t.ID,
		Owner:               t.Owner,
		State:               t.Status,
		StartTime:           t.StartTime,
		WakeValue:           t.WakeValue,
		TaskLocal:           t.TaskLocal,
		CallStack:           cloneActivationFrames(t.CallStack),
		Programmer:          t.Programmer,
		VerbLoc:             t.VerbLoc,
		VerbName:            t.VerbName,
		This:                t.This,
		ReadingPlayer:       t.ReadingPlayer,
		IsExecSuspended:     t.IsExecSuspended,
		ExecCommandName:     t.ExecCommandName,
		IsHTTPReadSuspended: t.IsHTTPReadSuspended,
	}
	if t.Status == TaskSuspended && !t.WakeTime.IsZero() {
		snapshot.StartTime = t.WakeTime
	}
	if t.ForkInfo != nil {
		var variableNames []string
		firstLine := 0
		if t.Program != nil {
			variableNames = append(variableNames, t.Program.VarNames...)
			firstLine = t.Program.LineForIP(0)
		} else if body := t.ForkInfo.Body; body != nil {
			variableNames = append(variableNames, body.VariableNames()...)
			firstLine = body.FirstLine()
		}
		for _, name := range CommandVariableNames {
			if _, ok := t.ForkInfo.Variables[name]; ok && !slices.Contains(variableNames, name) {
				variableNames = append(variableNames, name)
			}
		}
		snapshot.Fork = &ForkSnapshot{
			Variables:     cloneValueMap(t.ForkInfo.Variables),
			VariableNames: variableNames,
			SourceLines:   append([]string(nil), t.ForkInfo.SourceLines...),
			FirstLine:     firstLine,
		}
	}
	return snapshot
}

func cloneActivationFrames(frames []types.ActivationFrame) []types.ActivationFrame {
	if len(frames) == 0 {
		return nil
	}
	copied := make([]types.ActivationFrame, len(frames))
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
