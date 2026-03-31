package builtins

import (
	"barn/db"
	"barn/types"
	"testing"
)

func addTestObject(t *testing.T, store *db.Store, obj *db.Object) {
	t.Helper()
	if err := store.Add(obj); err != nil {
		t.Fatalf("store.Add(%d) failed: %v", obj.ID, err)
	}
}

func newPermissionTestStore(t *testing.T) *db.Store {
	t.Helper()

	store := db.NewStore()

	owner := db.NewObject(1, 1)
	owner.Flags = db.FlagWrite // intentionally not readable for properties() permission test
	owner.Verbs["test"] = &db.Verb{
		Name:  "test",
		Names: []string{"test"},
		Owner: 1,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return 1;"},
	}
	owner.VerbList = append(owner.VerbList, owner.Verbs["test"])
	owner.Properties["p"] = &db.Property{
		Name:    "p",
		Value:   types.NewInt(1),
		Owner:   1,
		Perms:   db.PropRead | db.PropWrite,
		Defined: true,
	}
	addTestObject(t, store, owner)

	nonOwner := db.NewObject(2, 2)
	addTestObject(t, store, nonOwner)

	wizard := db.NewObject(99, 99)
	wizard.Flags = db.FlagWizard
	addTestObject(t, store, wizard)

	return store
}

func makeCtx(player, programmer types.ObjID, wizard bool) *types.TaskContext {
	ctx := types.NewTaskContext()
	ctx.Player = player
	ctx.Programmer = programmer
	ctx.IsWizard = wizard
	return ctx
}

func TestVerbMutatorsRequireOwnerOrWizard(t *testing.T) {
	store := newPermissionTestStore(t)
	otherCtx := makeCtx(2, 2, false)
	ownerCtx := makeCtx(1, 1, false)

	obj := types.NewObj(1)

	t.Run("set_verb_info_denied_for_non_owner", func(t *testing.T) {
		info := types.NewList([]types.Value{types.NewObj(1), types.NewStr("rxd"), types.NewStr("test")})
		res := builtinSetVerbInfo(otherCtx, []types.Value{obj, types.NewStr("test"), info}, store)
		if !res.IsError() || res.Error != types.E_PERM {
			t.Fatalf("expected E_PERM, got flow=%v err=%v", res.Flow, res.Error)
		}
	})

	t.Run("set_verb_args_denied_for_non_owner", func(t *testing.T) {
		argspec := types.NewList([]types.Value{types.NewStr("this"), types.NewStr("none"), types.NewStr("none")})
		res := builtinSetVerbArgs(otherCtx, []types.Value{obj, types.NewStr("test"), argspec}, store)
		if !res.IsError() || res.Error != types.E_PERM {
			t.Fatalf("expected E_PERM, got flow=%v err=%v", res.Flow, res.Error)
		}
	})

	t.Run("set_verb_code_denied_for_non_owner", func(t *testing.T) {
		code := types.NewList([]types.Value{types.NewStr("return 2;")})
		res := builtinSetVerbCode(otherCtx, []types.Value{obj, types.NewStr("test"), code}, store)
		if !res.IsError() || res.Error != types.E_PERM {
			t.Fatalf("expected E_PERM, got flow=%v err=%v", res.Flow, res.Error)
		}
	})

	t.Run("delete_verb_denied_for_non_owner", func(t *testing.T) {
		res := builtinDeleteVerb(otherCtx, []types.Value{obj, types.NewStr("test")}, store)
		if !res.IsError() || res.Error != types.E_PERM {
			t.Fatalf("expected E_PERM, got flow=%v err=%v", res.Flow, res.Error)
		}
	})

	t.Run("set_verb_code_allowed_for_owner", func(t *testing.T) {
		code := types.NewList([]types.Value{types.NewStr("return 3;")})
		res := builtinSetVerbCode(ownerCtx, []types.Value{obj, types.NewStr("test"), code}, store)
		if res.IsError() {
			t.Fatalf("expected success, got error=%v", res.Error)
		}
	})
}

func TestPropertyMutatorsRequireOwnerOrWizard(t *testing.T) {
	store := newPermissionTestStore(t)
	otherCtx := makeCtx(2, 2, false)
	ownerCtx := makeCtx(1, 1, false)
	obj := types.NewObj(1)

	t.Run("properties_requires_readable_or_owner", func(t *testing.T) {
		res := builtinProperties(otherCtx, []types.Value{obj}, store)
		if !res.IsError() || res.Error != types.E_PERM {
			t.Fatalf("expected E_PERM, got flow=%v err=%v", res.Flow, res.Error)
		}
	})

	t.Run("set_property_info_denied_for_non_owner", func(t *testing.T) {
		info := types.NewList([]types.Value{types.NewObj(1), types.NewStr("rw")})
		res := builtinSetPropertyInfo(otherCtx, []types.Value{obj, types.NewStr("p"), info}, store)
		if !res.IsError() || res.Error != types.E_PERM {
			t.Fatalf("expected E_PERM, got flow=%v err=%v", res.Flow, res.Error)
		}
	})

	t.Run("delete_property_denied_for_non_owner", func(t *testing.T) {
		res := builtinDeleteProperty(otherCtx, []types.Value{obj, types.NewStr("p")}, store)
		if !res.IsError() || res.Error != types.E_PERM {
			t.Fatalf("expected E_PERM, got flow=%v err=%v", res.Flow, res.Error)
		}
	})

	t.Run("owner_can_set_and_delete_property", func(t *testing.T) {
		info := types.NewList([]types.Value{types.NewObj(1), types.NewStr("r")})
		setRes := builtinSetPropertyInfo(ownerCtx, []types.Value{obj, types.NewStr("p"), info}, store)
		if setRes.IsError() {
			t.Fatalf("expected set_property_info success, got %v", setRes.Error)
		}

		delRes := builtinDeleteProperty(ownerCtx, []types.Value{obj, types.NewStr("p")}, store)
		if delRes.IsError() {
			t.Fatalf("expected delete_property success, got %v", delRes.Error)
		}
	})
}

func TestRenumberRequiresWizard(t *testing.T) {
	store := db.NewStore()
	obj := db.NewObject(1, 1)
	addTestObject(t, store, obj)

	nonWizard := makeCtx(1, 1, false)
	res := builtinRenumber(nonWizard, []types.Value{types.NewObj(1)}, store)
	if !res.IsError() || res.Error != types.E_PERM {
		t.Fatalf("expected E_PERM, got flow=%v err=%v", res.Flow, res.Error)
	}

	wizard := makeCtx(99, 99, true)
	res = builtinRenumber(wizard, []types.Value{types.NewObj(1)}, store)
	if res.IsError() {
		t.Fatalf("expected success for wizard, got %v", res.Error)
	}
}
