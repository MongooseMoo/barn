package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestFinalizationScratchCollection(t *testing.T) {
	first := types.NewWaif(1, 1)
	alias := types.NewWaifWithIdentity(1, 1, first.WaifIdentity())
	second := types.NewWaif(1, 1)
	anon := types.NewAnon(42)
	nested := types.NewList([]types.Value{first, types.NewMap([][2]types.Value{
		{types.NewStr("alias"), alias}, {types.NewStr("anon"), anon},
	}), second, anon})
	for _, framePath := range []bool{false, true} {
		machine := &VM{PendingFinalizations: []types.Value{first}}
		for range 3 {
			if framePath {
				machine.collectPendingFinalizationsFromFrame(&StackFrame{Locals: []types.Value{nested}, Args: []types.Value{alias}})
			} else {
				machine.collectPendingFinalizationsFromValue(nested)
			}
		}
		want := []types.Value{first, anon, second}
		if len(machine.PendingFinalizations) != len(want) {
			t.Fatalf("frame=%v roots=%v", framePath, machine.PendingFinalizations)
		}
		for i, value := range want {
			if !machine.PendingFinalizations[i].Equal(value) {
				t.Fatalf("frame=%v root[%d]=%v want %v", framePath, i, machine.PendingFinalizations[i], value)
			}
		}
		other := &VM{}
		other.collectPendingFinalizationsFromValue(second)
		if len(other.PendingFinalizations) != 1 || !other.PendingFinalizations[0].Equal(second) {
			t.Fatal("scratch leaked roots between VMs")
		}
	}
}

func TestFinalizationScratchResetAndBounds(t *testing.T) {
	for _, oversized := range []string{"none", "refs", "waifs"} {
		t.Run(oversized, func(t *testing.T) {
			scratch := pendingFinalizationScratchPool.New().(*pendingFinalizationScratch)
			scratch.refs[42] = struct{}{}
			scratch.waifs = append(scratch.waifs, types.NewWaif(1, 1))
			if oversized == "refs" {
				for i := range maxPendingFinalizationScratch + 1 {
					scratch.refs[types.ObjID(i)] = struct{}{}
				}
			}
			if oversized == "waifs" {
				scratch.waifs = make([]types.Value, maxPendingFinalizationScratch+1)
				scratch.waifs[len(scratch.waifs)-1] = types.NewWaif(1, 1)
			}
			backing := scratch.waifs[:cap(scratch.waifs)]
			if reusable := scratch.reset(); reusable != (oversized == "none") {
				t.Fatalf("reusable=%v", reusable)
			}
			if len(scratch.refs) != 0 || len(scratch.waifs) != 0 {
				t.Fatal("reset retained roots")
			}
			for _, value := range backing {
				if value != (types.Value{}) {
					t.Fatal("reset retained a waif in backing storage")
				}
			}
		})
	}
}
