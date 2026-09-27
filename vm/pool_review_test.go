package vm

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestPoolReviewFinalizationConcurrentIsolation(t *testing.T) {
	for worker := range 8 {
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			for iteration := range 100 {
				anon := types.NewAnon(types.ObjID(1000 + worker*100 + iteration))
				waif := types.NewWaif(1, 1)
				value := types.NewList([]types.Value{waif, anon, waif, anon})
				machine := &VM{}
				if iteration%2 == 0 {
					machine.releaseLocal(value)
				} else {
					machine.collectPendingFinalizationsFromFrame(&StackFrame{Locals: []types.Value{value}})
				}
				other := &VM{}
				other.releaseLocal(types.NewWaif(1, 1))
				if len(machine.PendingFinalizations) != 2 || !machine.PendingFinalizations[0].Equal(anon) || !machine.PendingFinalizations[1].Equal(waif) {
					t.Fatal("foreign, stale, or reordered pending roots")
				}
				if len(machine.PendingWaifs) != 1 || !machine.PendingWaifs[0].Equal(waif) {
					t.Fatal("foreign or missing waif roots")
				}
			}
		})
	}
}
