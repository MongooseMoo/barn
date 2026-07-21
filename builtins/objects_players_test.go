package builtins

import (
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

func TestSetPlayerFlagStagesThroughTransaction(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	obj, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	ctx := kernel.NewTaskContext()
	ctx.Store = store
	ctx.StoreTxn = store.BeginReadOnly(0)
	ctx.IsWizard = true
	if errCode := ctx.StoreTxn.AdoptLiveObject(obj); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveObject failed: %v", errCode)
	}

	res := builtinSetPlayerFlag(ctx, []types.Value{types.NewObj(obj), types.NewInt(1)})
	if res.IsError() {
		t.Fatalf("set_player_flag failed: %v", res.Error)
	}

	liveFlag, errCode := store.HasObjectFlag(obj, dbstore.FlagUser)
	if errCode != types.E_NONE {
		t.Fatalf("live HasObjectFlag failed: %v", errCode)
	}
	if liveFlag {
		t.Fatalf("live player flag before commit = true, want false")
	}
	txFlag, errCode := ctx.StoreTxn.HasObjectFlag(obj, dbstore.FlagUser)
	if errCode != types.E_NONE {
		t.Fatalf("tx HasObjectFlag failed: %v", errCode)
	}
	if !txFlag {
		t.Fatalf("tx player flag = false, want true")
	}

	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
	liveFlag, errCode = store.HasObjectFlag(obj, dbstore.FlagUser)
	if errCode != types.E_NONE {
		t.Fatalf("live HasObjectFlag after commit failed: %v", errCode)
	}
	if !liveFlag {
		t.Fatalf("live player flag after commit = false, want true")
	}
}

func TestSetPlayerFlagClearDefersBootThroughTransaction(t *testing.T) {
	manager := &stubConnManager{conn: &stubConn{}}

	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	obj, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}
	if errCode := store.SetObjectFlag(obj, dbstore.FlagUser, true); errCode != types.E_NONE {
		t.Fatalf("SetObjectFlag failed: %v", errCode)
	}

	ctx := ctxWithConnManager(manager)
	ctx.Store = store
	ctx.StoreTxn = store.BeginReadOnly(0)
	ctx.IsWizard = true

	res := builtinSetPlayerFlag(ctx, []types.Value{types.NewObj(obj), types.NewInt(0)})
	if res.IsError() {
		t.Fatalf("set_player_flag failed: %v", res.Error)
	}
	if len(manager.boots) != 0 {
		t.Fatalf("boots before flush = %#v, want none", manager.boots)
	}
	if len(ctx.PendingEffects) != 1 || ctx.PendingEffects[0].Kind != kernel.PendingEffectBootPlayer || ctx.PendingEffects[0].BootPlayer != obj {
		t.Fatalf("pending effects = %#v, want boot of %d", ctx.PendingEffects, obj)
	}

	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
	if errCode := FlushPendingEffects(ctx); errCode != types.E_NONE {
		t.Fatalf("FlushPendingEffects failed: %v", errCode)
	}
	if len(manager.boots) != 1 || manager.boots[0] != obj {
		t.Fatalf("boots after flush = %#v, want [%d]", manager.boots, obj)
	}
}
