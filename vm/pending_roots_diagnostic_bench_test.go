package vm

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// These rows cover repeated roots and anonymous-only scratch, which the
// distinct-WAIF development benchmark does not measure. Each VM is warmed once
// so the timed operation measures temporary collection rather than root growth.
func BenchmarkPendingFinalizationDiagnostic(b *testing.B) {
	for _, kind := range []string{"Anonymous", "Waif"} {
		counts := []int{1, 8}
		if kind == "Anonymous" {
			counts = []int{1, 8, 256, 257}
		}
		for _, count := range counts {
			values := make([]types.Value, count)
			for i := range values {
				if kind == "Anonymous" {
					values[i] = types.NewAnon(types.ObjID(100 + i))
				} else {
					values[i] = types.NewWaif(1, 1)
				}
			}
			value := values[0]
			if count > 1 {
				value = types.NewList(values)
			}
			for _, path := range []string{"Value", "Frame"} {
				b.Run(fmt.Sprintf("%s/%d/%s", kind, count, path), func(b *testing.B) {
					machine := &VM{}
					frame := &StackFrame{Locals: values}
					if path == "Frame" {
						machine.collectPendingFinalizationsFromFrame(frame)
						for b.Loop() {
							machine.collectPendingFinalizationsFromFrame(frame)
						}
					} else {
						machine.collectPendingFinalizationsFromValue(value)
						for b.Loop() {
							machine.collectPendingFinalizationsFromValue(value)
						}
					}
					if len(machine.PendingFinalizations) != count {
						b.Fatalf("roots=%d want %d", len(machine.PendingFinalizations), count)
					}
				})
			}
		}
	}
	b.Run("ParallelMixed", func(b *testing.B) {
		values := make([]types.Value, 16)
		for i := range 8 {
			values[i] = types.NewAnon(types.ObjID(100 + i))
			values[i+8] = types.NewWaif(1, 1)
		}
		value := types.NewList(values)
		value.MayHoldFinalizable() // Resolve the immutable list cache before sharing.
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			machine := &VM{}
			frame := &StackFrame{Locals: values}
			machine.collectPendingFinalizationsFromValue(value)
			for pb.Next() {
				machine.collectPendingFinalizationsFromValue(value)
				machine.collectPendingFinalizationsFromFrame(frame)
			}
			if len(machine.PendingFinalizations) != len(values) {
				b.Errorf("roots=%d want %d", len(machine.PendingFinalizations), len(values))
			}
		})
	})
}
