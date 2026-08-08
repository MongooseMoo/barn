package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestGenericCoarseBuiltinPropagatesFailedLegacyTopologyFlush(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	addTxnObject(t, store, 1, 0)
	addTxnObject(t, store, 2, 0)
	ctx.StoreTxn.Release()
	ctx.StoreTxn = store.BeginReadOnly(0)

	if errCode := ctx.StoreTxn.SetObjectName(0, "private"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName stage: %v", errCode)
	}
	if errCode := ctx.StoreTxn.MoveObject(1, 2, 0); errCode != types.E_NONE {
		t.Fatalf("MoveObject stage: %v", errCode)
	}
	if err := store.Recycle(2); err != nil {
		t.Fatalf("Recycle staged topology target: %v", err)
	}

	result := builtinAddVerb(ctx, []types.Value{
		types.NewObj(0),
		types.NewList([]types.Value{types.NewObj(0), types.NewStr("rxd"), types.NewStr("added")}),
		types.NewList([]types.Value{types.NewStr("none"), types.NewStr("none"), types.NewStr("none")}),
	})
	if !result.IsError() || result.Error != types.E_INVIND {
		t.Fatalf("add_verb after failed legacy topology flush = %+v, want E_INVIND", result)
	}
	if ctx.LiveStoreMutated {
		t.Error("failed legacy topology flush marked the task live-mutated")
	}
	if got, errCode := store.ObjectName(0); errCode != types.E_NONE || got != "" {
		t.Errorf("failed legacy topology flush changed live name = %q, %v; want empty, E_NONE", got, errCode)
	}
	if _, err := store.FindVerbOnObject(0, "added"); err == nil {
		t.Error("coarse builtin ran after its topology flush failed")
	}
	if got, errCode := ctx.StoreTxn.ObjectName(0); errCode != types.E_NONE || got != "private" {
		t.Errorf("failed legacy topology flush private name = %q, %v; want private, E_NONE", got, errCode)
	}
	if got, errCode := ctx.StoreTxn.Location(1); errCode != types.E_NONE || got != 2 {
		t.Errorf("failed legacy topology flush private location = #%d, %v; want #2, E_NONE", got, errCode)
	}
	if got, errCode := store.Location(1); errCode != types.E_NONE || got != types.ObjNothing {
		t.Errorf("failed legacy topology flush live location = #%d, %v; want #%d, E_NONE", got, errCode, types.ObjNothing)
	}
	if ctx.StoreTxn.HasWrites() {
		t.Fatal("failed legacy topology flush remained recommittable")
	}
}
