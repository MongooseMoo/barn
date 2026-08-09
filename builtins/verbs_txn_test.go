package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func verbMetadataTxnTestContext(t *testing.T) (*kernel.TaskContext, *dbstore.Store) {
	t.Helper()

	store := dbstore.NewStore()
	obj := dbstore.NewObject(0, 0)
	if err := store.Add(obj); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	for _, flag := range []dbstore.ObjectFlags{dbstore.FlagWizard, dbstore.FlagProgrammer, dbstore.FlagRead, dbstore.FlagWrite} {
		if errCode := store.SetObjectFlag(0, flag, true); errCode != types.E_NONE {
			t.Fatalf("SetObjectFlag %v failed: %v", flag, errCode)
		}
	}
	if errCode := store.DefineProperty(0, "marker", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	verb := dbstore.NewVerb(
		"look",
		[]string{"look"},
		0,
		dbstore.VerbRead|dbstore.VerbWrite|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "none", Prep: "none", That: "none"},
		[]string{"return 1;"},
	)
	if _, errCode := store.AddVerb(0, verb); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	ctx.Player = 0
	ctx.Programmer = 0
	ctx.Store = store
	ctx.StoreTxn = store.BeginReadOnly(0)
	return ctx, store
}

func addTxnObject(t *testing.T, store *dbstore.Store, objID types.ObjID, owner types.ObjID, flags ...dbstore.ObjectFlags) {
	t.Helper()

	if err := store.Add(dbstore.NewObject(objID, owner)); err != nil {
		t.Fatalf("Add object #%d failed: %v", objID, err)
	}
	for _, flag := range flags {
		if errCode := store.SetObjectFlag(objID, flag, true); errCode != types.E_NONE {
			t.Fatalf("SetObjectFlag #%d %v failed: %v", objID, flag, errCode)
		}
	}
}

func TestSetVerbInfoRefreshesTransactionVerbView(t *testing.T) {
	ctx, _ := verbMetadataTxnTestContext(t)

	result := builtinSetVerbInfo(ctx, []types.Value{
		types.NewObj(0),
		types.NewStr("look"),
		types.NewList([]types.Value{
			types.NewObj(0),
			types.NewStr("rxd"),
			types.NewStr("look glance"),
		}),
	})
	if result.IsError() {
		t.Fatalf("set_verb_info failed: %v", result.Error)
	}
	if errCode := ctx.StoreTxn.SetPropertyValue(0, "marker", types.NewInt(1)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue failed: %v", errCode)
	}
	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
}

func TestDeleteVerbRefreshesTransactionVerbView(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)

	if _, _, err := ctx.StoreTxn.FindVerb(0, "look"); err != nil {
		t.Fatalf("FindVerb failed: %v", err)
	}
	result := builtinDeleteVerb(ctx, []types.Value{types.NewObj(0), types.NewStr("look")})
	if result.IsError() {
		t.Fatalf("delete_verb failed: %v", result.Error)
	}
	if errCode := ctx.StoreTxn.SetPropertyValue(0, "marker", types.NewInt(1)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue failed: %v", errCode)
	}
	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
	if _, _, err := store.FindVerb(0, "look"); err == nil {
		t.Fatal("deleted verb still exists")
	}
}

func TestDeleteVerbAuthorityRevocationConflictsBeforeMutation(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	ctx.IsWizard = false

	result := builtinDeleteVerb(ctx, []types.Value{types.NewObj(0), types.NewStr("look")})
	if result.IsError() {
		t.Fatalf("delete_verb staging failed: %v", result.Error)
	}
	if _, err := store.FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("staged delete mutated live store before commit: %v", err)
	}
	if errCode := store.SetObjectOwner(0, 1); errCode != types.E_NONE {
		t.Fatalf("SetObjectOwner(revoke): %v", errCode)
	}
	if errCode := store.SetObjectFlag(0, dbstore.FlagWrite, false); errCode != types.E_NONE {
		t.Fatalf("SetObjectFlag(revoke): %v", errCode)
	}
	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_INVARG || !ctx.StoreTxn.ValidationFailed() {
		t.Fatalf("Commit after authority revocation = %v, validationFailed=%v; want E_INVARG, true", errCode, ctx.StoreTxn.ValidationFailed())
	}
	if _, err := store.FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("conflicted delete mutated live store: %v", err)
	}

	retry := kernel.NewTaskContext()
	retry.Store = store
	retry.StoreTxn = store.BeginReadOnly(0)
	retry.Programmer = 0
	retry.Player = 0
	result = builtinDeleteVerb(retry, []types.Value{types.NewObj(0), types.NewStr("look")})
	if !result.IsError() || result.Error != types.E_PERM {
		t.Fatalf("serial retry after authority revocation = %+v, want E_PERM", result)
	}
	if _, err := store.FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("denied retry mutated live store: %v", err)
	}
}

func TestDeleteVerbUnrelatedReadConflictLeavesVerbIntact(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	addTxnObject(t, store, 1, 0)
	if errCode := store.SetObjectName(1, "before"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName(before): %v", errCode)
	}
	ctx.StoreTxn.Release()
	ctx.StoreTxn = store.BeginReadOnly(0)
	if _, errCode := ctx.StoreTxn.ObjectName(1); errCode != types.E_NONE {
		t.Fatalf("ObjectName read: %v", errCode)
	}
	result := builtinDeleteVerb(ctx, []types.Value{types.NewObj(0), types.NewStr("look")})
	if result.IsError() {
		t.Fatalf("delete_verb staging failed: %v", result.Error)
	}
	if errCode := store.SetObjectName(1, "after"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName(conflict): %v", errCode)
	}

	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_INVARG || !ctx.StoreTxn.ValidationFailed() {
		t.Fatalf("Commit after unrelated read conflict = %v, validationFailed=%v; want E_INVARG, true", errCode, ctx.StoreTxn.ValidationFailed())
	}
	if _, err := store.FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("unrelated conflict allowed irreversible verb deletion: %v", err)
	}
}

func TestDeleteVerbStagesShiftedIndicesAndSurvivorCode(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	for _, name := range []string{"second", "survivor"} {
		verb := dbstore.NewVerb(name, []string{name}, 0, dbstore.VerbRead|dbstore.VerbExecute, dbstore.VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 1;"})
		if _, errCode := store.AddVerb(0, verb); errCode != types.E_NONE {
			t.Fatalf("AddVerb(%s): %v", name, errCode)
		}
	}
	if errCode := ctx.StoreTxn.AdoptLiveVerbs(0); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveVerbs: %v", errCode)
	}
	if errCode := ctx.StoreTxn.SetVerbCode(0, "survivor", []string{"return 3;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCode(survivor): %v", errCode)
	}
	for i := 0; i < 2; i++ {
		result := builtinDeleteVerb(ctx, []types.Value{types.NewObj(0), types.NewInt(1)})
		if result.IsError() {
			t.Fatalf("delete_verb shifted index call %d: %v", i+1, result.Error)
		}
	}
	if names, errCode := ctx.StoreTxn.VerbNames(0); errCode != types.E_NONE || len(names) != 1 || names[0] != "survivor" {
		t.Fatalf("transaction verb names after shifted deletes = %v, %v; want [survivor], E_NONE", names, errCode)
	}
	if _, err := store.FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("staged deletes mutated live store before commit: %v", err)
	}
	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit: %v", errCode)
	}
	if _, err := store.FindVerbOnObject(0, "look"); err == nil {
		t.Fatal("first shifted-index target remains after commit")
	}
	if _, err := store.FindVerbOnObject(0, "second"); err == nil {
		t.Fatal("second shifted-index target remains after commit")
	}
	survivor, err := store.FindVerbOnObject(0, "survivor")
	if err != nil {
		t.Fatalf("FindVerbOnObject(survivor): %v", err)
	}
	if len(survivor.Code) != 1 || survivor.Code[0] != "return 3;" {
		t.Fatalf("survivor code after commit = %v, want [return 3;]", survivor.Code)
	}
}

func TestDeleteVerbConcurrentListGenerationDoesNotRetarget(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	second := dbstore.NewVerb("second", []string{"second"}, 0, dbstore.VerbRead|dbstore.VerbExecute, dbstore.VerbArgs{This: "none", Prep: "none", That: "none"}, nil)
	if _, errCode := store.AddVerb(0, second); errCode != types.E_NONE {
		t.Fatalf("AddVerb(second): %v", errCode)
	}
	if errCode := ctx.StoreTxn.AdoptLiveVerbs(0); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveVerbs: %v", errCode)
	}
	result := builtinDeleteVerb(ctx, []types.Value{types.NewObj(0), types.NewInt(2)})
	if result.IsError() {
		t.Fatalf("delete_verb staging failed: %v", result.Error)
	}
	if errCode := store.DeleteVerb(0, "look"); errCode != types.E_NONE {
		t.Fatalf("concurrent DeleteVerb(look): %v", errCode)
	}

	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_INVARG || !ctx.StoreTxn.ValidationFailed() {
		t.Fatalf("Commit after verb generation change = %v, validationFailed=%v; want E_INVARG, true", errCode, ctx.StoreTxn.ValidationFailed())
	}
	if _, err := store.FindVerbOnObject(0, "second"); err != nil {
		t.Fatalf("stale staged index retargeted surviving verb: %v", err)
	}
}

func stagedDeleteFlushConflictContext(t *testing.T) (*kernel.TaskContext, *dbstore.Store) {
	t.Helper()
	ctx, store := verbMetadataTxnTestContext(t)
	if _, errCode := ctx.StoreTxn.ObjectName(0); errCode != types.E_NONE {
		t.Fatalf("ObjectName conflict read: %v", errCode)
	}
	result := builtinDeleteVerb(ctx, []types.Value{types.NewObj(0), types.NewStr("look")})
	if result.IsError() {
		t.Fatalf("delete_verb staging failed: %v", result.Error)
	}
	if errCode := store.SetObjectName(0, "changed concurrently"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName(conflict): %v", errCode)
	}
	return ctx, store
}

func TestVerbCoarseBuiltinPropagatesStagedDeleteFlushConflict(t *testing.T) {
	ctx, store := stagedDeleteFlushConflictContext(t)
	result := builtinAddVerb(ctx, []types.Value{
		types.NewObj(0),
		types.NewList([]types.Value{types.NewObj(0), types.NewStr("rxd"), types.NewStr("added")}),
		types.NewList([]types.Value{types.NewStr("none"), types.NewStr("none"), types.NewStr("none")}),
	})
	if !result.IsError() || result.Error != types.E_INVARG {
		t.Fatalf("add_verb after conflicted staged delete = %+v, want E_INVARG", result)
	}
	if ctx.LiveStoreMutated {
		t.Fatal("failed staged-delete flush marked task live-mutated")
	}
	if _, err := store.FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("failed flush deleted staged target: %v", err)
	}
	if _, err := store.FindVerbOnObject(0, "added"); err == nil {
		t.Fatal("add_verb mutated live store after failed flush")
	}
}

func TestObjectCoarseBuiltinPropagatesStagedDeleteFlushConflict(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	addTxnObject(t, store, 1, 0)
	addTxnObject(t, store, 2, 0)
	if err := store.Recycle(1); err != nil {
		t.Fatalf("Recycle(#1): %v", err)
	}
	ctx.StoreTxn.Release()
	ctx.StoreTxn = store.BeginReadOnly(0)
	if _, errCode := ctx.StoreTxn.ObjectName(0); errCode != types.E_NONE {
		t.Fatalf("ObjectName conflict read: %v", errCode)
	}
	result := builtinDeleteVerb(ctx, []types.Value{types.NewObj(0), types.NewStr("look")})
	if result.IsError() {
		t.Fatalf("delete_verb staging failed: %v", result.Error)
	}
	if errCode := store.SetObjectName(0, "changed concurrently"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName(conflict): %v", errCode)
	}

	result = builtinRenumber(ctx, []types.Value{types.NewObj(2)})
	if !result.IsError() || result.Error != types.E_INVARG {
		t.Fatalf("renumber after conflicted staged delete = %+v, want E_INVARG", result)
	}
	if ctx.LiveStoreMutated {
		t.Fatal("failed staged-delete flush marked task live-mutated")
	}
	if _, err := store.FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("failed flush deleted staged target: %v", err)
	}
	if !store.Valid(2) || !store.IsRecycled(1) {
		t.Fatalf("renumber mutated live object IDs after failed flush: valid(#2)=%v recycled(#1)=%v", store.Valid(2), store.IsRecycled(1))
	}
}

func TestVerbMetadataSetupWithStagedPropertiesCommits(t *testing.T) {
	store := dbstore.NewStore()
	addTxnObject(t, store, 0, 0, dbstore.FlagWizard, dbstore.FlagProgrammer, dbstore.FlagRead, dbstore.FlagWrite)
	addTxnObject(t, store, 6, 0, dbstore.FlagRead, dbstore.FlagWrite)
	addTxnObject(t, store, 10, 0, dbstore.FlagWizard, dbstore.FlagProgrammer, dbstore.FlagUser, dbstore.FlagRead, dbstore.FlagWrite)
	verb := dbstore.NewVerb(
		"do_login_command",
		[]string{"do_login_command"},
		0,
		dbstore.VerbRead|dbstore.VerbWrite|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"return 0;"},
	)
	if _, errCode := store.AddVerb(0, verb); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	ctx.Player = 10
	ctx.Programmer = 10
	ctx.Store = store
	ctx.StoreTxn = store.BeginReadOnly(0)

	for _, name := range []string{"audit_proxy_seen", "audit_proxy_login_saved", "audit_proxy_trusted_saved"} {
		result := builtinAddProperty(ctx, []types.Value{
			types.NewObj(0),
			types.NewStr(name),
			types.NewList(nil),
			types.NewList([]types.Value{types.NewObj(0), types.NewStr("rw")}),
		})
		if result.IsError() {
			t.Fatalf("add_property #%d.%s failed: %v", 0, name, result.Error)
		}
	}
	result := builtinAddProperty(ctx, []types.Value{
		types.NewObj(6),
		types.NewStr("trusted_proxies"),
		types.NewList(nil),
		types.NewList([]types.Value{types.NewObj(0), types.NewStr("rw")}),
	})
	if result.IsError() {
		t.Fatalf("add_property #6.trusted_proxies failed: %v", result.Error)
	}

	if errCode := ctx.StoreTxn.SetPropertyValue(0, "audit_proxy_trusted_saved", types.NewList([]types.Value{
		types.NewInt(0),
		types.NewList(nil),
	})); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue audit_proxy_trusted_saved failed: %v", errCode)
	}
	if errCode := ctx.StoreTxn.SetPropertyValue(6, "trusted_proxies", types.NewList([]types.Value{
		types.NewStr("::1"),
		types.NewStr("127.0.0.1"),
	})); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue trusted_proxies failed: %v", errCode)
	}

	if _, _, err := ctx.StoreTxn.FindVerb(0, "do_login_command"); err != nil {
		t.Fatalf("FindVerb failed: %v", err)
	}
	if errCode := ctx.StoreTxn.SetPropertyValue(0, "audit_proxy_login_saved", types.NewList([]types.Value{
		types.NewInt(1),
		types.NewList([]types.Value{types.NewObj(0), types.NewStr("rxd"), types.NewStr("do_login_command")}),
		types.NewList([]types.Value{types.NewStr("this"), types.NewStr("none"), types.NewStr("this")}),
		types.NewList([]types.Value{types.NewStr("return 0;")}),
	})); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue audit_proxy_login_saved failed: %v", errCode)
	}

	result = builtinSetVerbInfo(ctx, []types.Value{
		types.NewObj(0),
		types.NewStr("do_login_command"),
		types.NewList([]types.Value{
			types.NewObj(10),
			types.NewStr("rxd"),
			types.NewStr("do_login_command"),
		}),
	})
	if result.IsError() {
		t.Fatalf("set_verb_info failed: %v", result.Error)
	}
	result = builtinSetVerbArgs(ctx, []types.Value{
		types.NewObj(0),
		types.NewStr("do_login_command"),
		types.NewList([]types.Value{
			types.NewStr("this"),
			types.NewStr("none"),
			types.NewStr("this"),
		}),
	})
	if result.IsError() {
		t.Fatalf("set_verb_args failed: %v", result.Error)
	}
	result = builtinSetVerbCode(ctx, []types.Value{
		types.NewObj(0),
		types.NewStr("do_login_command"),
		types.NewList([]types.Value{
			types.NewStr("#0.audit_proxy_seen = {args, argstr};"),
			types.NewStr("return 0;"),
		}),
	})
	if result.IsError() {
		t.Fatalf("set_verb_code failed: %v", result.Error)
	}
	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
}

func TestSetVerbArgsRefreshesTransactionVerbView(t *testing.T) {
	ctx, _ := verbMetadataTxnTestContext(t)

	result := builtinSetVerbArgs(ctx, []types.Value{
		types.NewObj(0),
		types.NewStr("look"),
		types.NewList([]types.Value{
			types.NewStr("any"),
			types.NewStr("none"),
			types.NewStr("none"),
		}),
	})
	if result.IsError() {
		t.Fatalf("set_verb_args failed: %v", result.Error)
	}
	if errCode := ctx.StoreTxn.SetPropertyValue(0, "marker", types.NewInt(1)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue failed: %v", errCode)
	}
	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
}
