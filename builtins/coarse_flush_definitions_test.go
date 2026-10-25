package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// A coarse create reads its parent's slot names from the live store, so a
// property definition staged earlier in the task must reach the live store
// first, or the child is created without a slot for it.
func TestCoarseCreateSeesStagedPropertyDefinition(t *testing.T) {
	ctx, _ := verbMetadataTxnTestContext(t)

	parent := builtinCreate(ctx, []types.Value{types.NewObj(types.ObjNothing)})
	if parent.IsError() {
		t.Fatalf("create parent: %+v", parent)
	}
	// An anonymous create is coarse; the task is live-mutated after it.
	if anon := builtinCreate(ctx, []types.Value{types.NewObj(0), types.NewInt(1)}); anon.IsError() {
		t.Fatalf("create anonymous: %+v", anon)
	}
	if !ctx.LiveStoreMutated {
		t.Fatal("anonymous create did not take the coarse path")
	}
	if result := builtinAddProperty(ctx, []types.Value{
		parent.Val, types.NewStr("x"), types.NewInt(7),
		types.NewList([]types.Value{types.NewObj(0), types.NewStr("rw")}),
	}); result.IsError() {
		t.Fatalf("add_property: %+v", result)
	}
	child := builtinCreate(ctx, []types.Value{parent.Val})
	if child.IsError() {
		t.Fatalf("create child: %+v", child)
	}

	values, errCode := ctx.StoreTxn.PropertyValues(child.Val.ID())
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValues: %v", errCode)
	}
	if len(values) != 1 {
		t.Fatalf("child has %d property slots, want 1 (the inherited x)", len(values))
	}
}
