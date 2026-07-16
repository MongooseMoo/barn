package builtins

import (
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
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
