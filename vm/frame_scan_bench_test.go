package vm

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Small plain locals dominate the frame-return samples in the Mongoose profile.
func BenchmarkPlainFrameFinalization(b *testing.B) {
	frame := &StackFrame{Locals: make([]types.Value, 32)}
	for i := range frame.Locals {
		frame.Locals[i] = types.NewInt(int64(i))
	}
	frame.Args = []types.Value{types.NewStr("command"), types.NewInt(1)}
	machine := &VM{}
	for b.Loop() {
		machine.collectPendingFinalizationsFromFrame(frame)
	}
}

func BenchmarkTemporaryFrameLifecycle(b *testing.B) {
	program, _ := compileBench(b, "{a, ?b = 2, @rest} = {1, 2, 3}; return rest;")
	for _, slots := range []int{256, program.NumLocals} {
		b.Run(fmt.Sprint(slots), func(b *testing.B) {
			machine := &VM{}
			for b.Loop() {
				frame := machine.frameFrom(StackFrame{Locals: machine.allocLocals(slots), localsOnStack: true})
				machine.collectPendingFinalizationsFromFrame(frame)
				machine.recycleFrame(frame)
			}
		})
	}
}
