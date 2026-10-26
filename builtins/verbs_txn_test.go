package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func verbMetadataTxnTestContext(t *testing.T) (*Execution, *dbstore.Store) {
	t.Helper()

	store := dbstore.NewStore()
	obj := dbstore.NewObject(0, 0)
	if err := store.Add(obj); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	for _, flag := range []dbstore.ObjectFlags{dbstore.FlagWizard, dbstore.FlagProgrammer, dbstore.FlagRead, dbstore.FlagWrite} {
		if errCode := store.DirectTxn().SetObjectFlag(0, flag, true); errCode != types.E_NONE {
			t.Fatalf("SetObjectFlag %v failed: %v", flag, errCode)
		}
	}
	if errCode := store.DirectTxn().DefineProperty(0, "marker", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	verb := dbstore.NewVerb(
		[]string{"look"},
		0,
		dbstore.VerbRead|dbstore.VerbWrite|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "none", Prep: "none", That: "none"},
		[]string{"return 1;"},
	)
	if _, errCode := store.AddVerb(0, verb); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	ctx := newTestExecution()
	ctx.IsWizard = true
	ctx.Player = 0
	ctx.Programmer = 0
	ctx.Store = store
	ctx.StoreTxn = store.BeginSnapshot(0)
	return ctx, store
}

func addTxnObject(t *testing.T, store *dbstore.Store, objID types.ObjID, owner types.ObjID, flags ...dbstore.ObjectFlags) {
	t.Helper()

	if err := store.Add(dbstore.NewObject(objID, owner)); err != nil {
		t.Fatalf("Add object #%d failed: %v", objID, err)
	}
	for _, flag := range flags {
		if errCode := store.DirectTxn().SetObjectFlag(objID, flag, true); errCode != types.E_NONE {
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

// Toast validate_verb_info skips leading spaces and rejects an empty remainder
// (src/verbs.cc:115-119).
func TestSetVerbInfoRejectsBlankNames(t *testing.T) {
	for _, names := range []string{"", "   "} {
		ctx, _ := verbMetadataTxnTestContext(t)

		result := builtinSetVerbInfo(ctx, []types.Value{
			types.NewObj(0),
			types.NewStr("look"),
			types.NewList([]types.Value{
				types.NewObj(0),
				types.NewStr("rxd"),
				types.NewStr(names),
			}),
		})
		if !result.IsError() || result.Error != types.E_INVARG {
			t.Fatalf("set_verb_info names %q = %v, want E_INVARG", names, result)
		}
		if _, _, err := ctx.StoreTxn.FindVerb(0, "look"); err != nil {
			t.Fatalf("names %q: verb look no longer found: %v", names, err)
		}
	}
}

// Toast checks the object and the whole info/args list before E_VERBNF and
// E_PERM (src/verbs.cc:330-352, 421-457).
func TestVerbSettersValidateBeforeLookupAndPermission(t *testing.T) {
	list := func(vals ...types.Value) types.Value { return types.NewList(vals) }
	str := types.NewStr
	cases := []struct {
		name       string
		fn         func(*Execution, []types.Value) types.Result
		obj        types.ObjID
		verb       string
		arg        types.Value
		nonWizard  bool
		want       types.ErrorCode
		verbIntact bool
	}{
		{"info missing verb bad flags", builtinSetVerbInfo, 0, "nosuch", list(types.NewObj(0), str("q"), str("x")), false, types.E_INVARG, false},
		{"info missing verb owner type", builtinSetVerbInfo, 0, "nosuch", list(types.NewInt(1), str("rx"), str("x")), false, types.E_TYPE, false},
		{"info non-owner bad flags", builtinSetVerbInfo, 0, "look", list(types.NewObj(1), str("q"), str("x")), true, types.E_INVARG, true},
		{"info short list", builtinSetVerbInfo, 0, "look", list(types.NewObj(0), str("rx")), false, types.E_TYPE, true},
		{"info invalid owner", builtinSetVerbInfo, 0, "look", list(types.NewObj(99), str("rx"), str("x")), false, types.E_INVARG, true},
		{"info invalid object", builtinSetVerbInfo, 99, "look", list(types.NewObj(0), str("rx"), str("x")), false, types.E_INVARG, true},
		{"args missing verb bad prep", builtinSetVerbArgs, 0, "nosuch", list(str("this"), str("bogus"), str("this")), false, types.E_INVARG, false},
		{"args non-owner bad prep", builtinSetVerbArgs, 0, "look", list(str("this"), str("bogus"), str("this")), true, types.E_INVARG, true},
		{"args bad dobj", builtinSetVerbArgs, 0, "look", list(str("bogus"), str("none"), str("this")), false, types.E_INVARG, true},
		{"args object iobj", builtinSetVerbArgs, 0, "look", list(str("this"), str("none"), types.NewObj(1)), false, types.E_TYPE, true},
		{"args short list", builtinSetVerbArgs, 0, "look", list(str("this"), str("none")), false, types.E_TYPE, true},
		{"args invalid object", builtinSetVerbArgs, 99, "look", list(str("this"), str("none"), str("this")), false, types.E_INVARG, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, store := verbMetadataTxnTestContext(t)
			addTxnObject(t, store, 1, 1, dbstore.FlagProgrammer)
			if tc.nonWizard {
				ctx.IsWizard = false
				ctx.Player = 1
				ctx.Programmer = 1
			}
			result := tc.fn(ctx, []types.Value{types.NewObj(tc.obj), str(tc.verb), tc.arg})
			if !result.IsError() || result.Error != tc.want {
				t.Fatalf("got %v, want %v", result, tc.want)
			}
			if tc.verbIntact {
				verb, _, err := ctx.StoreTxn.FindVerb(0, "look")
				if err != nil {
					t.Fatalf("verb look no longer found: %v", err)
				}
				if verb.Owner != 0 || verb.ArgSpec.This != "none" {
					t.Fatalf("verb look changed: owner %v, args %+v", verb.Owner, verb.ArgSpec)
				}
			}
		})
	}
}

// Toast validate_verb_info skips only spaces (src/verbs.cc:115-119) and stores
// the names as given, so a tab is a name.
func TestSetVerbInfoTabOnlyNamesIsOneName(t *testing.T) {
	ctx, _ := verbMetadataTxnTestContext(t)
	result := builtinSetVerbInfo(ctx, []types.Value{
		types.NewObj(0),
		types.NewStr("look"),
		types.NewList([]types.Value{types.NewObj(0), types.NewStr("rx"), types.NewStr("\t")}),
	})
	if result.IsError() {
		t.Fatalf("set_verb_info tab names = %v", result)
	}
	verb, _, err := ctx.StoreTxn.FindVerb(0, "\t")
	if err != nil {
		t.Fatalf("verb named tab not found: %v", err)
	}
	if len(verb.Names) != 1 || verb.Names[0] != "\t" {
		t.Fatalf("names = %q, want [\"\\t\"]", verb.Names)
	}
}

// A non-wizard cannot change a verb's or a property's owner; the owner may keep
// itself as owner. Toast: src/verbs.cc:349-354, src/property.cc:172-175.
func TestSetVerbAndPropertyInfoOwnerChangeRequiresWizard(t *testing.T) {
	list := func(vals ...types.Value) types.Value { return types.NewList(vals) }
	str := types.NewStr
	cases := []struct {
		name      string
		fn        func(*Execution, []types.Value) types.Result
		item      string
		arg       types.Value
		wizard    bool
		want      types.ErrorCode
		wantOwner types.ObjID
	}{
		{"verb non-wizard owner change", builtinSetVerbInfo, "mine", list(types.NewObj(0), str("rx"), str("mine")), false, types.E_PERM, 1},
		{"verb non-wizard same owner", builtinSetVerbInfo, "mine", list(types.NewObj(1), str("r"), str("mine")), false, types.E_NONE, 1},
		{"verb wizard owner change", builtinSetVerbInfo, "mine", list(types.NewObj(0), str("rx"), str("mine")), true, types.E_NONE, 0},
		{"property non-wizard owner change", builtinSetPropertyInfo, "p", list(types.NewObj(0), str("r")), false, types.E_PERM, 1},
		{"property non-wizard same owner", builtinSetPropertyInfo, "p", list(types.NewObj(1), str("rw")), false, types.E_NONE, 1},
		{"property wizard owner change", builtinSetPropertyInfo, "p", list(types.NewObj(0), str("r")), true, types.E_NONE, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, store := verbMetadataTxnTestContext(t)
			addTxnObject(t, store, 1, 1, dbstore.FlagProgrammer)
			verb := dbstore.NewVerb([]string{"mine"}, 1, dbstore.VerbRead|dbstore.VerbExecute, dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, nil)
			if _, errCode := store.AddVerb(0, verb); errCode != types.E_NONE {
				t.Fatalf("AddVerb failed: %v", errCode)
			}
			if errCode := store.DirectTxn().DefineProperty(0, "p", dbstore.NewProperty(types.NewInt(0), 1, dbstore.PropRead, false, true)); errCode != types.E_NONE {
				t.Fatalf("DefineProperty failed: %v", errCode)
			}
			ctx.StoreTxn.Release()
			ctx.StoreTxn = store.BeginSnapshot(0)
			if !tc.wizard {
				ctx.IsWizard = false
				ctx.Player = 1
				ctx.Programmer = 1
			}

			result := tc.fn(ctx, []types.Value{types.NewObj(0), str(tc.item), tc.arg})
			if tc.want == types.E_NONE && result.IsError() {
				t.Fatalf("got %v, want success", result)
			}
			if tc.want != types.E_NONE && (!result.IsError() || result.Error != tc.want) {
				t.Fatalf("got %v, want %v", result, tc.want)
			}

			var owner types.ObjID
			if tc.item == "mine" {
				v, _, err := ctx.StoreTxn.FindVerb(0, "mine")
				if err != nil {
					t.Fatalf("verb mine not found: %v", err)
				}
				owner = v.Owner
			} else {
				p, ok, errCode := localPropertyForRead(ctx, 0, "p")
				if !ok || errCode != types.E_NONE {
					t.Fatalf("property p not found: %v", errCode)
				}
				owner = p.Owner
			}
			if owner != tc.wantOwner {
				t.Fatalf("owner = #%d, want #%d", owner, tc.wantOwner)
			}
		})
	}
}

func TestDeleteVerbRefreshesTransactionVerbView(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)

	if _, _, err := ctx.StoreTxn.FindVerb(0, "look"); err != nil {
		t.Fatalf("FindVerb failed: %v", err)
	}
	result := admitTestBuiltin("delete_verb", ctx, []types.Value{types.NewObj(0), types.NewStr("look")})
	if result.IsError() {
		t.Fatalf("delete_verb failed: %v", result.Error)
	}
	if errCode := ctx.StoreTxn.SetPropertyValue(0, "marker", types.NewInt(1)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue failed: %v", errCode)
	}
	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
	if _, _, err := store.DirectTxn().FindVerb(0, "look"); err == nil {
		t.Fatal("deleted verb still exists")
	}
}

func TestDeleteVerbAuthorityRevocationConflictsBeforeMutation(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	ctx.IsWizard = false

	result := admitTestBuiltin("delete_verb", ctx, []types.Value{types.NewObj(0), types.NewStr("look")})
	if result.IsError() {
		t.Fatalf("delete_verb staging failed: %v", result.Error)
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("staged delete mutated live store before commit: %v", err)
	}
	if errCode := store.DirectTxn().SetObjectOwner(0, 1); errCode != types.E_NONE {
		t.Fatalf("SetObjectOwner(revoke): %v", errCode)
	}
	if errCode := store.DirectTxn().SetObjectFlag(0, dbstore.FlagWrite, false); errCode != types.E_NONE {
		t.Fatalf("SetObjectFlag(revoke): %v", errCode)
	}
	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_INVARG || !ctx.StoreTxn.ValidationFailed() {
		t.Fatalf("Commit after authority revocation = %v, validationFailed=%v; want E_INVARG, true", errCode, ctx.StoreTxn.ValidationFailed())
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("conflicted delete mutated live store: %v", err)
	}

	retry := newTestExecution()
	retry.Store = store
	retry.StoreTxn = store.BeginSnapshot(0)
	retry.Programmer = 0
	retry.Player = 0
	result = admitTestBuiltin("delete_verb", retry, []types.Value{types.NewObj(0), types.NewStr("look")})
	if !result.IsError() || result.Error != types.E_PERM {
		t.Fatalf("serial retry after authority revocation = %+v, want E_PERM", result)
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("denied retry mutated live store: %v", err)
	}
}

func TestDeleteVerbUnrelatedReadConflictLeavesVerbIntact(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	addTxnObject(t, store, 1, 0)
	if errCode := store.DirectTxn().SetObjectName(1, "before"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName(before): %v", errCode)
	}
	ctx.StoreTxn.Release()
	ctx.StoreTxn = store.BeginSnapshot(0)
	if _, errCode := ctx.StoreTxn.ObjectName(1); errCode != types.E_NONE {
		t.Fatalf("ObjectName read: %v", errCode)
	}
	result := admitTestBuiltin("delete_verb", ctx, []types.Value{types.NewObj(0), types.NewStr("look")})
	if result.IsError() {
		t.Fatalf("delete_verb staging failed: %v", result.Error)
	}
	if errCode := store.DirectTxn().SetObjectName(1, "after"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName(conflict): %v", errCode)
	}

	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_INVARG || !ctx.StoreTxn.ValidationFailed() {
		t.Fatalf("Commit after unrelated read conflict = %v, validationFailed=%v; want E_INVARG, true", errCode, ctx.StoreTxn.ValidationFailed())
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("unrelated conflict allowed irreversible verb deletion: %v", err)
	}
}

func TestDeleteVerbStagesShiftedIndicesAndSurvivorCode(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	for _, name := range []string{"second", "survivor"} {
		verb := dbstore.NewVerb([]string{name}, 0, dbstore.VerbRead|dbstore.VerbExecute, dbstore.VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 1;"})
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
		result := admitTestBuiltin("delete_verb", ctx, []types.Value{types.NewObj(0), types.NewInt(1)})
		if result.IsError() {
			t.Fatalf("delete_verb shifted index call %d: %v", i+1, result.Error)
		}
	}
	if names, errCode := ctx.StoreTxn.VerbNames(0); errCode != types.E_NONE || len(names) != 1 || names[0] != "survivor" {
		t.Fatalf("transaction verb names after shifted deletes = %v, %v; want [survivor], E_NONE", names, errCode)
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("staged deletes mutated live store before commit: %v", err)
	}
	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit: %v", errCode)
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "look"); err == nil {
		t.Fatal("first shifted-index target remains after commit")
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "second"); err == nil {
		t.Fatal("second shifted-index target remains after commit")
	}
	survivor, err := store.DirectTxn().FindVerbOnObject(0, "survivor")
	if err != nil {
		t.Fatalf("FindVerbOnObject(survivor): %v", err)
	}
	if len(survivor.Code) != 1 || survivor.Code[0] != "return 3;" {
		t.Fatalf("survivor code after commit = %v, want [return 3;]", survivor.Code)
	}
}

func TestDeleteVerbConcurrentListGenerationDoesNotRetarget(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	second := dbstore.NewVerb([]string{"second"}, 0, dbstore.VerbRead|dbstore.VerbExecute, dbstore.VerbArgs{This: "none", Prep: "none", That: "none"}, nil)
	if _, errCode := store.AddVerb(0, second); errCode != types.E_NONE {
		t.Fatalf("AddVerb(second): %v", errCode)
	}
	if errCode := ctx.StoreTxn.AdoptLiveVerbs(0); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveVerbs: %v", errCode)
	}
	result := admitTestBuiltin("delete_verb", ctx, []types.Value{types.NewObj(0), types.NewInt(2)})
	if result.IsError() {
		t.Fatalf("delete_verb staging failed: %v", result.Error)
	}
	if errCode := store.DeleteVerb(0, "look"); errCode != types.E_NONE {
		t.Fatalf("concurrent DeleteVerb(look): %v", errCode)
	}

	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_INVARG || !ctx.StoreTxn.ValidationFailed() {
		t.Fatalf("Commit after verb generation change = %v, validationFailed=%v; want E_INVARG, true", errCode, ctx.StoreTxn.ValidationFailed())
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "second"); err != nil {
		t.Fatalf("stale staged index retargeted surviving verb: %v", err)
	}
}

func stagedDeleteFlushConflictContext(t *testing.T) (*Execution, *dbstore.Store) {
	t.Helper()
	ctx, store := verbMetadataTxnTestContext(t)
	if _, errCode := ctx.StoreTxn.ObjectName(0); errCode != types.E_NONE {
		t.Fatalf("ObjectName conflict read: %v", errCode)
	}
	result := admitTestBuiltin("delete_verb", ctx, []types.Value{types.NewObj(0), types.NewStr("look")})
	if result.IsError() {
		t.Fatalf("delete_verb staging failed: %v", result.Error)
	}
	if errCode := store.DirectTxn().SetObjectName(0, "changed concurrently"); errCode != types.E_NONE {
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
	if _, err := store.DirectTxn().FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("failed flush deleted staged target: %v", err)
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "added"); err == nil {
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
	ctx.StoreTxn = store.BeginSnapshot(0)
	if _, errCode := ctx.StoreTxn.ObjectName(0); errCode != types.E_NONE {
		t.Fatalf("ObjectName conflict read: %v", errCode)
	}
	result := admitTestBuiltin("delete_verb", ctx, []types.Value{types.NewObj(0), types.NewStr("look")})
	if result.IsError() {
		t.Fatalf("delete_verb staging failed: %v", result.Error)
	}
	if errCode := store.DirectTxn().SetObjectName(0, "changed concurrently"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName(conflict): %v", errCode)
	}

	result = builtinRenumber(ctx, []types.Value{types.NewObj(2)})
	if !result.IsError() || result.Error != types.E_INVARG {
		t.Fatalf("renumber after conflicted staged delete = %+v, want E_INVARG", result)
	}
	if ctx.LiveStoreMutated {
		t.Fatal("failed staged-delete flush marked task live-mutated")
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "look"); err != nil {
		t.Fatalf("failed flush deleted staged target: %v", err)
	}
	if !store.DirectTxn().Valid(2) || !store.DirectTxn().IsRecycled(1) {
		t.Fatalf("renumber mutated live object IDs after failed flush: valid(#2)=%v recycled(#1)=%v", store.DirectTxn().Valid(2), store.DirectTxn().IsRecycled(1))
	}
}

func TestVerbMetadataSetupWithStagedPropertiesCommits(t *testing.T) {
	store := dbstore.NewStore()
	addTxnObject(t, store, 0, 0, dbstore.FlagWizard, dbstore.FlagProgrammer, dbstore.FlagRead, dbstore.FlagWrite)
	addTxnObject(t, store, 6, 0, dbstore.FlagRead, dbstore.FlagWrite)
	addTxnObject(t, store, 10, 0, dbstore.FlagWizard, dbstore.FlagProgrammer, dbstore.FlagUser, dbstore.FlagRead, dbstore.FlagWrite)
	verb := dbstore.NewVerb(
		[]string{"do_login_command"},
		0,
		dbstore.VerbRead|dbstore.VerbWrite|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"return 0;"},
	)
	if _, errCode := store.AddVerb(0, verb); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	ctx := newTestExecution()
	ctx.IsWizard = true
	ctx.Player = 10
	ctx.Programmer = 10
	ctx.Store = store
	ctx.StoreTxn = store.BeginSnapshot(0)

	for _, name := range []string{"audit_proxy_seen", "audit_proxy_login_saved", "audit_proxy_trusted_saved"} {
		result := admitTestBuiltin("add_property", ctx, []types.Value{
			types.NewObj(0),
			types.NewStr(name),
			types.NewList(nil),
			types.NewList([]types.Value{types.NewObj(0), types.NewStr("rw")}),
		})
		if result.IsError() {
			t.Fatalf("add_property #%d.%s failed: %v", 0, name, result.Error)
		}
	}
	result := admitTestBuiltin("add_property", ctx, []types.Value{
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
	result = admitTestBuiltin("set_verb_code", ctx, []types.Value{
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
