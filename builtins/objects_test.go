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
