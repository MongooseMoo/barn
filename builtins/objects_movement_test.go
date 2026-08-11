package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func newMoveTestContext(t *testing.T) (*Execution, *dbstore.Store) {
	t.Helper()

	store := dbstore.NewStore()
	registry := NewRegistry()
	ctx := newTestExecution()
	ctx.Store = store
	ctx.Registry = registry
	ctx.IsWizard = true
	ctx.Programmer = 0
	ctx.Player = 0
	return ctx, store
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

func TestMoveInvalidArgumentsMatchToast(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*testing.T, *dbstore.Store) []types.Value
		wantCode types.ErrorCode
	}{
		{
			name: "nonexistent positive destination",
			prepare: func(t *testing.T, store *dbstore.Store) []types.Value {
				t.Helper()
				what, errCode := store.CreateObject([]types.ObjID{types.ObjNothing}, types.ObjNothing, false)
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
				what, errCode := store.CreateObject([]types.ObjID{types.ObjNothing}, types.ObjNothing, false)
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
				what, errCode := store.CreateObject([]types.ObjID{types.ObjNothing}, types.ObjNothing, false)
				if errCode != types.E_NONE {
					t.Fatalf("CreateObject returned %s", errCode)
				}
				where, errCode := store.CreateObject([]types.ObjID{types.ObjNothing}, types.ObjNothing, false)
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
				where, errCode := store.CreateObject([]types.ObjID{types.ObjNothing}, types.ObjNothing, false)
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
