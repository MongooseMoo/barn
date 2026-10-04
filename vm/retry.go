package vm

import (
	"maps"
	"slices"

	"github.com/MongooseMoo/barn/types"
)

// RetryCheckpoint owns the slice-entry execution state. Programs and MOO
// values are immutable; frame/stack storage and native continuations are not.
// This is intentionally independent of the disk-checkpoint representation.
type RetryCheckpoint struct {
	state VM
	roots []types.Value
}

// CheckpointForRetry is called with the task's physical execution lease held,
// before delivering its wake value. A first-run VM needs no continuation copy.
func (machine *VM) CheckpointForRetry() *RetryCheckpoint {
	if machine == nil || !machine.yielded {
		return nil
	}
	checkpoint := &RetryCheckpoint{state: copyRetryVM(machine)}
	checkpoint.state.Context = nil
	checkpoint.state.Task = nil
	if ctx := machine.Context; ctx != nil {
		checkpoint.roots = append(checkpoint.roots, ctx.ThisValue, ctx.MapFirstKey, ctx.MapLastKey, ctx.TaskLocal)
	}
	if machine.Task != nil {
		checkpoint.roots = append(checkpoint.roots, machine.Task.GetTaskLocal())
		for _, activation := range machine.Task.GetCallStack() {
			checkpoint.roots = append(checkpoint.roots, activation.ThisValue, activation.RuntimeVariables)
			checkpoint.roots = append(checkpoint.roots, activation.Args...)
			if snapshot := activation.RuntimeVariableSnapshot; snapshot != nil {
				checkpoint.roots = append(checkpoint.roots, snapshot.Values...)
			}
		}
	}
	machine.retryCheckpoint = checkpoint
	return checkpoint
}

// Restore replaces only execution state; the runtime rebinds the attempt's
// context and services. No session guard is reacquired as on a disk restore.
func (checkpoint *RetryCheckpoint) Restore(machine *VM) {
	ctx, owner := machine.Context, machine.Task
	*machine = copyRetryVM(&checkpoint.state)
	machine.Context, machine.Task = ctx, owner
	machine.retryCheckpoint = checkpoint
}

func (machine *VM) ReleaseRetryCheckpoint() { machine.retryCheckpoint = nil }

// RetainWakeValue keeps a value consumed on the first attempt live for retries.
func (checkpoint *RetryCheckpoint) RetainWakeValue(value types.Value) {
	checkpoint.roots = append(checkpoint.roots, value)
}

func copyRetryVM(source *VM) VM {
	copy := *source
	copy.Stack = slices.Clone(source.Stack[:source.SP])
	copy.Frames = make([]*StackFrame, 0, len(source.Frames))
	copy.frame = nil
	frames := make([]StackFrame, len(source.Frames))
	for i, original := range source.Frames {
		frame := &frames[i]
		*frame = *original
		frame.Locals = slices.Clone(original.Locals)
		frame.Args = slices.Clone(original.Args)
		frame.LoopStack = slices.Clone(original.LoopStack)
		frame.ExceptStack = cloneHandlers(original.ExceptStack)
		frame.MoveContinuation = cloneMoveContinuation(original.MoveContinuation)
		if original.RecycleContinuation != nil {
			continuation := *original.RecycleContinuation
			continuation.request.OldParents = slices.Clone(continuation.request.OldParents)
			continuation.request.OldChildren = slices.Clone(continuation.request.OldChildren)
			continuation.request.OldContents = slices.Clone(continuation.request.OldContents)
			frame.RecycleContinuation = &continuation
		}
		if pending, ok := original.PendingError.(*VMException); ok {
			cloned := *pending
			frame.PendingError = &cloned
		}
		frame.localsOnStack = false
		copy.pushFrame(frame)
	}
	copy.PendingWaifs = slices.Clone(source.PendingWaifs)
	copy.PendingFinalizations = slices.Clone(source.PendingFinalizations)
	copy.pendingWaifIDs = maps.Clone(source.pendingWaifIDs)
	copy.pendingFinalizationWaifIDs = maps.Clone(source.pendingFinalizationWaifIDs)
	copy.pendingFinalizationAnonIDs = maps.Clone(source.pendingFinalizationAnonIDs)
	copy.yieldResult.CallStack = slices.Clone(source.yieldResult.CallStack)
	for i := range copy.yieldResult.CallStack {
		frame := &copy.yieldResult.CallStack[i]
		frame.Args = slices.Clone(frame.Args)
	}
	if original := source.yieldResult.ForkInfo; original != nil {
		fork := *original
		fork.Variables = maps.Clone(original.Variables)
		fork.SourceLines = slices.Clone(original.SourceLines)
		copy.yieldResult.ForkInfo = &fork
	}
	// These belong to the active VM's services or reusable storage, never to
	// frozen execution state. Frame installation above owns its locals.
	copy.localStack = nil
	copy.framePool = nil
	copy.builtinExec = nil
	copy.builtinPendingFinalizations = nil
	copy.Preempt = nil
	copy.retryCheckpoint = nil
	return copy
}
