package vm

import (
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
