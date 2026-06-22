package task

import (
	"time"

	"barn/types"
)

// ForkSnapshot is the persistence-relevant portion of a forked task.
type ForkSnapshot struct {
	Variables   map[string]types.Value
	SourceLines []string
}

// Snapshot is an immutable copy of the task fields the database writer needs.
type Snapshot struct {
	ID            int64
	Owner         types.ObjID
	State         TaskState
	StartTime     time.Time
	WakeValue     types.Value
	TaskLocal     types.Value
	CallStack     []ActivationFrame
	Fork          *ForkSnapshot
	Programmer    types.ObjID
	VerbLoc       types.ObjID
	VerbName      string
	This          types.ObjID
	ReadingPlayer types.ObjID
}

// PersistenceSnapshot copies the task fields needed for checkpoint output while
// holding the task lock.
func (t *Task) PersistenceSnapshot() Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	snapshot := Snapshot{
		ID:            t.ID,
		Owner:         t.Owner,
		State:         t.State,
		StartTime:     t.StartTime,
		WakeValue:     t.WakeValue,
		TaskLocal:     t.TaskLocal,
		CallStack:     cloneActivationFrames(t.CallStack),
		Programmer:    t.Programmer,
		VerbLoc:       t.VerbLoc,
		VerbName:      t.VerbName,
		This:          t.This,
		ReadingPlayer: t.ReadingPlayer,
	}
	if t.ForkInfo != nil {
		snapshot.Fork = &ForkSnapshot{
			Variables:   cloneValueMap(t.ForkInfo.Variables),
			SourceLines: append([]string(nil), t.ForkInfo.SourceLines...),
		}
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
