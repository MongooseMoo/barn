package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Mirrors repeatedly overwriting locals while walking a large collection of
// distinct waifs. Setup is outside the measured VM bookkeeping.
func BenchmarkPendingWaifLocals(b *testing.B) {
	values := make([]types.Value, 4096)
	for i := range values {
		values[i] = types.NewWaif(1, 1)
	}
	b.ResetTimer()
	for b.Loop() {
		machine := &VM{}
		for range 2 {
			for _, value := range values {
				machine.releaseLocal(value)
			}
		}
		if len(machine.PendingWaifs) != len(values) || len(machine.PendingFinalizations) != len(values) {
			b.Fatal("lost or duplicated finalization roots")
		}
	}
}

func TestPendingWaifIdentityAndDrain(t *testing.T) {
	first := types.NewWaif(1, 1)
	alias := types.NewWaifWithIdentity(1, 1, first.WaifIdentity())
	second := types.NewWaif(1, 1)
	machine := &VM{}
	for _, value := range []types.Value{first, alias, second, first} {
		machine.releaseLocal(value)
	}
	for _, values := range [][]types.Value{machine.PendingWaifs, machine.PendingFinalizations} {
		if len(values) != 2 || !values[0].Equal(first) || !values[1].Equal(second) {
			t.Fatalf("pending identities/order = %v", values)
		}
	}
	machine.TakePendingWaifs()
	machine.releaseLocal(first)
	if len(machine.PendingWaifs) != 1 || !machine.PendingWaifs[0].Equal(first) {
		t.Fatal("drained waif could not be recorded again")
	}
	machine.TakePendingFinalizationValues()
	machine.releaseLocal(first)
	if len(machine.PendingFinalizations) != 1 || !machine.PendingFinalizations[0].Equal(first) {
		t.Fatal("drained finalization root could not be recorded again")
	}
}
