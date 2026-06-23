package builtins

import (
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

func TestMaxObjectReturnsObjectValue(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	ctx := kernel.NewTaskContext()
	ctx.Store = store

	res := builtinMaxObject(ctx, nil)
	if res.IsError() {
		t.Fatalf("max_object failed: %v", res.Error)
	}
	obj, ok := res.Val.(types.ObjValue)
	if !ok {
		t.Fatalf("max_object = %T, want ObjValue", res.Val)
	}
	if obj.ID() != 0 {
		t.Fatalf("max_object id = %d, want 0", obj.ID())
	}
}

func TestMoveInvalidObjectsReturnInvarg(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	obj, errCode := store.CreateObject([]types.ObjID{types.ObjNothing}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	ctx := kernel.NewTaskContext()
	ctx.Store = store
	ctx.Registry = NewRegistry()

	res := builtinMove(ctx, []types.Value{types.NewObj(99999), types.NewObj(obj)})
	if !res.IsError() || res.Error != types.E_INVARG {
		t.Fatalf("move(invalid, valid) = %#v, want E_INVARG", res)
	}

	res = builtinMove(ctx, []types.Value{types.NewObj(obj), types.NewObj(99999)})
	if !res.IsError() || res.Error != types.E_INVARG {
		t.Fatalf("move(valid, invalid) = %#v, want E_INVARG", res)
	}

	res = builtinMove(ctx, []types.Value{types.NewObj(obj), types.NewObj(-5)})
	if !res.IsError() || res.Error != types.E_INVARG {
		t.Fatalf("move(valid, #-5) = %#v, want E_INVARG", res)
	}
}
