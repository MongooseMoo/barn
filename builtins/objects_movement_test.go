package builtins

import (
	"reflect"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func newMoveTestContext(t *testing.T) (*Execution, *dbstore.Store) {
	t.Helper()

	store := dbstore.NewStore()
	registry := NewRegistry()
	ctx := newTestExecutionForSession(NewSession(registry, NoHost()))
	ctx.Store = store
	ctx.IsWizard = true
	ctx.Programmer = 0
	ctx.Player = 0
	return ctx, store
}

func createMoveTestObject(t *testing.T, store *dbstore.Store) types.ObjID {
	t.Helper()
	id, errCode := store.DirectTxn().CreateObject([]types.ObjID{types.ObjNothing}, types.ObjNothing, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject returned %s", errCode)
	}
	return id
}

func TestMoveCallsAcceptExitfuncAndEnterfuncAfterChangingLocation(t *testing.T) {
	ctx, store := newMoveTestContext(t)
	source := createMoveTestObject(t, store)
	destination := createMoveTestObject(t, store)
	what := createMoveTestObject(t, store)
	if errCode := store.DirectTxn().MoveObject(what, source, 0); errCode != types.E_NONE {
		t.Fatalf("initial MoveObject returned %s", errCode)
	}

	type callback struct {
		object   types.ObjID
		verb     string
		argument types.ObjID
		location types.ObjID
	}
	var callbacks []callback
	configureTestHost(ctx.Session, func(host *Host) {
		host.VerbCaller = func(objID types.ObjID, verbName string, args []types.Value, callCtx *Execution) types.Result {
			location, errCode := locationForRead(callCtx, what)
			if errCode != types.E_NONE {
				t.Fatalf("location during %s returned %s", verbName, errCode)
			}
			callbacks = append(callbacks, callback{objID, verbName, args[0].ID(), location})
			if verbName == "accept" {
				return types.Ok(types.NewInt(1))
			}
			return types.Ok(types.NewInt(0))
		}
	})

	result := builtinMove(ctx, []types.Value{types.NewObj(what), types.NewObj(destination)})
	if result.IsError() {
		t.Fatalf("move returned %s", result.Error)
	}
	want := []callback{
		{destination, "accept", what, source},
		{source, "exitfunc", what, destination},
		{destination, "enterfunc", what, destination},
	}
	if !reflect.DeepEqual(callbacks, want) {
		t.Fatalf("callbacks = %#v, want %#v", callbacks, want)
	}
}

func TestMoveWithinSameLocationSkipsAllCallbacks(t *testing.T) {
	ctx, store := newMoveTestContext(t)
	location := createMoveTestObject(t, store)
	what := createMoveTestObject(t, store)
	if errCode := store.DirectTxn().MoveObject(what, location, 0); errCode != types.E_NONE {
		t.Fatalf("initial MoveObject returned %s", errCode)
	}

	var callbacks []string
	configureTestHost(ctx.Session, func(host *Host) {
		host.VerbCaller = func(_ types.ObjID, verbName string, _ []types.Value, _ *Execution) types.Result {
			callbacks = append(callbacks, verbName)
			return types.Ok(types.NewInt(1))
		}
	})

	for _, args := range [][]types.Value{
		{types.NewObj(what), types.NewObj(location)},
		{types.NewObj(what), types.NewObj(location), types.NewInt(1)},
	} {
		if result := builtinMove(ctx, args); result.IsError() {
			t.Fatalf("same-location move returned %s", result.Error)
		}
	}
	if len(callbacks) != 0 {
		t.Fatalf("same-location callbacks = %v, want none", callbacks)
	}
}

func TestMoveSkipsOriginalEnterfuncWhenExitfuncRelocatesObject(t *testing.T) {
	ctx, store := newMoveTestContext(t)
	source := createMoveTestObject(t, store)
	destination := createMoveTestObject(t, store)
	alternate := createMoveTestObject(t, store)
	what := createMoveTestObject(t, store)
	if errCode := store.DirectTxn().MoveObject(what, source, 0); errCode != types.E_NONE {
		t.Fatalf("initial MoveObject returned %s", errCode)
	}

	var callbacks []string
	configureTestHost(ctx.Session, func(host *Host) {
		host.VerbCaller = func(_ types.ObjID, verbName string, _ []types.Value, callCtx *Execution) types.Result {
			callbacks = append(callbacks, verbName)
			if verbName == "accept" {
				return types.Ok(types.NewInt(1))
			}
			if verbName == "exitfunc" {
				if errCode := readTxn(callCtx).MoveObject(what, alternate, 0); errCode != types.E_NONE {
					t.Fatalf("exitfunc relocation returned %s", errCode)
				}
			}
			return types.Ok(types.NewInt(0))
		}
	})

	if result := builtinMove(ctx, []types.Value{types.NewObj(what), types.NewObj(destination)}); result.IsError() {
		t.Fatalf("move returned %s", result.Error)
	}
	if want := []string{"accept", "exitfunc"}; !reflect.DeepEqual(callbacks, want) {
		t.Fatalf("callbacks = %v, want %v", callbacks, want)
	}
	location, errCode := locationForRead(ctx, what)
	if errCode != types.E_NONE || location != alternate {
		t.Fatalf("location = #%d (%s), want #%d", location, errCode, alternate)
	}
}

func requireMoveError(t *testing.T, result types.Result, want types.ErrorCode) {
	t.Helper()

	if !result.IsError() {
		t.Fatalf("move returned %v, want error %s", result.Val, want)
	}
	if result.Error != want {
		t.Fatalf("move returned %s, want %s", result.Error, want)
	}
}

func TestMoveInvalidArgumentsMatchMOO(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*testing.T, *dbstore.Store) []types.Value
		wantCode types.ErrorCode
	}{
		{
			name: "nonexistent positive destination",
			prepare: func(t *testing.T, store *dbstore.Store) []types.Value {
				t.Helper()
				what, errCode := store.DirectTxn().CreateObject([]types.ObjID{types.ObjNothing}, types.ObjNothing, false)
				if errCode != types.E_NONE {
					t.Fatalf("CreateObject returned %s", errCode)
				}
				return []types.Value{types.NewObj(what), types.NewObj(99999)}
			},
			wantCode: types.E_INVARG,
		},
		{
			name: "negative destination other than nothing",
			prepare: func(t *testing.T, store *dbstore.Store) []types.Value {
				t.Helper()
				what, errCode := store.DirectTxn().CreateObject([]types.ObjID{types.ObjNothing}, types.ObjNothing, false)
				if errCode != types.E_NONE {
					t.Fatalf("CreateObject returned %s", errCode)
				}
				return []types.Value{types.NewObj(what), types.NewObj(-5)}
			},
			wantCode: types.E_INVARG,
		},
		{
			name: "recycled destination",
			prepare: func(t *testing.T, store *dbstore.Store) []types.Value {
				t.Helper()
				what, errCode := store.DirectTxn().CreateObject([]types.ObjID{types.ObjNothing}, types.ObjNothing, false)
				if errCode != types.E_NONE {
					t.Fatalf("CreateObject returned %s", errCode)
				}
				where, errCode := store.DirectTxn().CreateObject([]types.ObjID{types.ObjNothing}, types.ObjNothing, false)
				if errCode != types.E_NONE {
					t.Fatalf("CreateObject returned %s", errCode)
				}
				if err := store.Recycle(where); err != nil {
					t.Fatalf("Recycle returned %v", err)
				}
				return []types.Value{types.NewObj(what), types.NewObj(where)}
			},
			wantCode: types.E_INVARG,
		},
		{
			name: "invalid object",
			prepare: func(t *testing.T, store *dbstore.Store) []types.Value {
				t.Helper()
				where, errCode := store.DirectTxn().CreateObject([]types.ObjID{types.ObjNothing}, types.ObjNothing, false)
				if errCode != types.E_NONE {
					t.Fatalf("CreateObject returned %s", errCode)
				}
				return []types.Value{types.NewObj(99999), types.NewObj(where)}
			},
			wantCode: types.E_INVARG,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, store := newMoveTestContext(t)
			requireMoveError(t, builtinMove(ctx, tt.prepare(t, store)), tt.wantCode)
		})
	}
}
