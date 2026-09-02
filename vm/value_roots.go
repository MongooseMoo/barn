package vm

import "github.com/MongooseMoo/barn/types"

type valueRootKind uint8

const (
	valueRootLive valueRootKind = iota
	valueRootPendingFinalization
)

type valueVisitor func(types.Value, valueRootKind)

// visitValues enumerates the complete typed-value surface owned by a frame.
// Root consumers deliberately share this inventory so a newly added frame
// field cannot be visible to one finalizer while remaining invisible to another.
func (frame *StackFrame) visitValues(visit valueVisitor) {
	if frame == nil {
		return
	}
	if frame.Program != nil {
		for _, value := range frame.Program.Constants {
			visit(value, valueRootLive)
		}
	}
	for _, value := range frame.Locals {
		visit(value, valueRootLive)
	}
	for _, value := range frame.Args {
		visit(value, valueRootLive)
	}
	visit(frame.ThisValue, valueRootLive)
	visit(frame.SavedThisValue, valueRootLive)
	if frame.HasPendingReturn {
		visit(frame.PendingReturn, valueRootLive)
	}
	visitErrorValue(frame.PendingError, func(value types.Value) { visit(value, valueRootLive) })
	for _, loop := range frame.LoopStack {
		if value, ok := loop.Iterator.(types.Value); ok {
			visit(value, valueRootLive)
		}
		if value, ok := loop.End.(types.Value); ok {
			visit(value, valueRootLive)
		}
	}
	if continuation := frame.MoveContinuation; continuation != nil {
		visit(continuation.What, valueRootLive)
		visit(continuation.Where, valueRootLive)
		visit(continuation.OldLocation, valueRootLive)
	}
	if continuation := frame.RecycleContinuation; continuation != nil {
		visit(continuation.request.Object, valueRootLive)
	}
}

// visitValues enumerates every typed value retained by a live VM. Values inside
// containers and WAIF properties remain the responsibility of each consumer;
// this method owns only the inventory of VM state that can hold a value.
func (vm *VM) visitValues(visit valueVisitor) {
	if vm == nil || visit == nil {
		return
	}
	for _, frame := range vm.Frames {
		frame.visitValues(visit)
	}
	stackEnd := min(vm.SP, len(vm.Stack))
	for _, value := range vm.Stack[:max(0, stackEnd)] {
		visit(value, valueRootLive)
	}
	for _, value := range vm.PendingWaifs {
		visit(value, valueRootPendingFinalization)
	}
	for _, value := range vm.PendingFinalizations {
		visit(value, valueRootPendingFinalization)
	}
	visit(vm.yieldResult.Val, valueRootLive)
	for _, activation := range vm.yieldResult.CallStack {
		visit(activation.ThisValue, valueRootLive)
		for _, value := range activation.Args {
			visit(value, valueRootLive)
		}
		visit(activation.RuntimeVariables, valueRootLive)
	}
	if fork := vm.yieldResult.ForkInfo; fork != nil {
		visit(fork.ThisValue, valueRootLive)
		for _, value := range fork.Variables {
			visit(value, valueRootLive)
		}
	}
	if vm.Context != nil {
		visit(vm.Context.ThisValue, valueRootLive)
		visit(vm.Context.MapFirstKey, valueRootLive)
		visit(vm.Context.MapLastKey, valueRootLive)
		visit(vm.Context.TaskLocal, valueRootLive)
	}
	if vm.Task != nil {
		visit(vm.Task.GetTaskLocal(), valueRootLive)
	}
}

func visitErrorValue(err error, visit func(types.Value)) {
	for err != nil {
		switch pending := err.(type) {
		case VMException:
			visit(pending.Value)
			return
		case *VMException:
			visit(pending.Value)
			return
		case interface{ Unwrap() error }:
			err = pending.Unwrap()
		default:
			return
		}
	}
}
