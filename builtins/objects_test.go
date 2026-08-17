package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestMaxObjectReturnsObjectValue(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	ctx := newTestExecution()
	ctx.Store = store

	res := builtinMaxObject(ctx, nil)
	if res.IsError() {
		t.Fatalf("max_object failed: %v", res.Error)
	}
	if res.Val.Type() != types.TYPE_OBJ {
		t.Fatalf("max_object = %T, want ObjValue", res.Val)
	}
	if res.Val.ID() != 0 {
		t.Fatalf("max_object id = %d, want 0", res.Val.ID())
	}
}

func TestMoveInvalidObjectsReturnInvarg(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	obj, errCode := store.DirectTxn().CreateObject([]types.ObjID{types.ObjNothing}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	ctx := newTestExecutionForSession(NewSession(NewRegistry(), NoHost()))
	ctx.Store = store

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

func TestRecycleRequiresObjectControl(t *testing.T) {
	store := dbstore.NewStore()
	for _, object := range []struct {
		id    types.ObjID
		owner types.ObjID
		flags dbstore.ObjectFlags
	}{
		{id: 0, owner: 0, flags: dbstore.FlagWizard},
		{id: 2, owner: 2},
		{id: 3, owner: 3, flags: dbstore.FlagProgrammer},
		{id: 4, owner: 2},
	} {
		builder := dbstore.NewObjectBuilder(object.id)
		builder.SetOwner(object.owner)
		builder.SetFlags(object.flags)
		if err := store.Add(builder.Build()); err != nil {
			t.Fatalf("add object #%d: %v", object.id, err)
		}
	}

	ctx := newTestExecutionForSession(NewSession(NewRegistry(), NoHost()))
	ctx.Store = store
	ctx.Programmer = 3
	ctx.Player = 3

	result := builtinRecycle(ctx, []types.Value{types.NewObj(4)})
	if !result.IsError() || result.Error != types.E_PERM {
		t.Fatalf("nonowner recycle = %+v, want E_PERM", result)
	}
	if !store.DirectTxn().Valid(4) {
		t.Fatal("permission-denied recycle invalidated target")
	}

	ctx.Programmer = 0
	ctx.Player = 0
	ctx.IsWizard = true
	result = builtinRecycle(ctx, []types.Value{types.NewObj(4)})
	if result.IsError() {
		t.Fatalf("wizard recycle failed: %s", result.Error)
	}
}

func TestRecyclePropagatesHookErrorAfterDestroyingObject(t *testing.T) {
	store := dbstore.NewStore()
	for _, id := range []types.ObjID{0, 2, 4} {
		builder := dbstore.NewObjectBuilder(id)
		builder.SetOwner(2)
		if err := store.Add(builder.Build()); err != nil {
			t.Fatalf("add object #%d: %v", id, err)
		}
	}

	registry := NewRegistry()
	session := NewSession(registry, NoHost())
	configureTestHost(session, func(host *Host) {
		host.VerbCaller = func(objID types.ObjID, verbName string, args []types.Value, ctx *Execution) types.Result {
			if objID != 4 || verbName != "recycle" {
				t.Fatalf("hook call = #%d:%s, want #4:recycle", objID, verbName)
			}
			return types.Err(types.E_DIV)
		}
	})

	ctx := newTestExecutionForSession(session)
	ctx.Store = store
	ctx.Programmer = 2
	ctx.Player = 2

	result := builtinRecycle(ctx, []types.Value{types.NewObj(4)})
	if !result.IsError() || result.Error != types.E_DIV {
		t.Fatalf("recycle result = %+v, want E_DIV", result)
	}
	if store.DirectTxn().Valid(4) {
		t.Fatal("recycle hook error left target valid")
	}
}

func TestObjectBytesSeesStagedProperties(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	obj, errCode := store.DirectTxn().CreateObject([]types.ObjID{types.ObjNothing}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	ctx := newTestExecution()
	ctx.Store = store
	ctx.StoreTxn = store.BeginReadOnly(0)
	ctx.IsWizard = true
	ctx.Programmer = 0
	ctx.Player = 0

	before := builtinObjectBytes(ctx, []types.Value{types.NewObj(obj)})
	if before.IsError() {
		t.Fatalf("object_bytes before failed: %v", before.Error)
	}
	beforeVal := before.Val.Int()

	if errCode := ctx.StoreTxn.DefineProperty(obj, "test1", dbstore.NewProperty(types.NewStr("hello world"), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	after := builtinObjectBytes(ctx, []types.Value{types.NewObj(obj)})
	if after.IsError() {
		t.Fatalf("object_bytes after failed: %v", after.Error)
	}
	afterVal := after.Val.Int()
	if afterVal <= beforeVal {
		t.Fatalf("object_bytes after staged property = %d, before = %d", afterVal, beforeVal)
	}
}
