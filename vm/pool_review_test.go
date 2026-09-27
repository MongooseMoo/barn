package vm

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestPoolReviewFinalizationAlternatingValueShapes(t *testing.T) {
	first := types.NewWaif(1, 1)
	alias := types.NewWaifWithIdentity(1, 1, first.WaifIdentity())
	second := types.NewWaif(1, 1)
	third := types.NewWaif(1, 1)
	anon := types.NewAnon(42)
	nextAnon := types.NewAnon(43)
	nested := types.NewMap([][2]types.Value{
		{types.NewStr("held"), types.NewList([]types.Value{alias, second, anon})},
		{types.NewStr("new"), types.NewList([]types.Value{nextAnon, second})},
	})
	machine := &VM{}
	machine.releaseLocal(first)
	machine.releaseLocal(anon)
	heldRoots := machine.PendingFinalizations
	heldWaifs := machine.PendingWaifs
	for range 30 {
		for _, value := range []types.Value{alias, anon, nested, third, first} {
			machine.releaseLocal(value)
		}
		other := &VM{}
		other.releaseLocal(types.NewWaif(1, 1))
		wantRoots := []types.Value{first, anon, nextAnon, second, third}
		wantWaifs := []types.Value{first, second, third}
		for _, check := range []struct {
			name      string
			got, want []types.Value
		}{
			{"pending roots", machine.PendingFinalizations, wantRoots},
			{"pending waifs", machine.PendingWaifs, wantWaifs},
			{"retained roots", heldRoots, []types.Value{first, anon}},
			{"retained waifs", heldWaifs, []types.Value{first}},
		} {
			if len(check.got) != len(check.want) {
				t.Fatalf("%s: length=%d want %d", check.name, len(check.got), len(check.want))
			}
			for i, want := range check.want {
				if !check.got[i].Equal(want) {
					t.Fatalf("%s[%d]: lost, foreign, or reordered root", check.name, i)
				}
			}
		}
	}
}

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
