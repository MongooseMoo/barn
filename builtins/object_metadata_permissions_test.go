package builtins

import (
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

type metadataPermissionFixture struct {
	ctx      *kernel.TaskContext
	store    *dbstore.Store
	target   types.ObjID
	owner    types.ObjID
	intruder types.ObjID
}

func newMetadataPermissionFixture(t *testing.T, targetFlag dbstore.ObjectFlags) metadataPermissionFixture {
	t.Helper()
	store := dbstore.NewStore()
	create := func(owner types.ObjID) types.ObjID {
		t.Helper()
		id, errCode := store.CreateObject([]types.ObjID{types.ObjNothing}, owner, false)
		if errCode != types.E_NONE {
			t.Fatalf("CreateObject: %s", errCode)
		}
		return id
	}

	owner := create(types.ObjNothing)
	if errCode := store.SetObjectOwner(owner, owner); errCode != types.E_NONE {
		t.Fatalf("SetObjectOwner(owner): %s", errCode)
	}
	target := create(owner)
	intruder := create(owner)
	if errCode := store.SetObjectOwner(intruder, intruder); errCode != types.E_NONE {
		t.Fatalf("SetObjectOwner(intruder): %s", errCode)
	}
	if targetFlag != 0 {
		if errCode := store.SetObjectFlag(target, targetFlag, true); errCode != types.E_NONE {
			t.Fatalf("SetObjectFlag(target): %s", errCode)
		}
	}
	if errCode := store.DefineProperty(
		target,
		"existing",
		dbstore.NewProperty(types.NewInt(1), owner, dbstore.PropRead|dbstore.PropWrite, false, true),
	); errCode != types.E_NONE {
		t.Fatalf("DefineProperty(existing): %s", errCode)
	}
	verb := dbstore.NewVerb(
		"existing",
		[]string{"existing"},
		owner,
		dbstore.VerbRead|dbstore.VerbWrite|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "none", Prep: "none", That: "none"},
		[]string{"return 1;"},
	)
	if _, errCode := store.AddVerb(target, verb); errCode != types.E_NONE {
		t.Fatalf("AddVerb(existing): %s", errCode)
	}

	ctx := kernel.NewTaskContext()
	ctx.Store = store
	ctx.StoreTxn = store.BeginReadOnly(0)
	ctx.Programmer = intruder
	ctx.Player = intruder

	return metadataPermissionFixture{
		ctx:      ctx,
		store:    store,
		target:   target,
		owner:    owner,
		intruder: intruder,
	}
}

type metadataPermissionCase struct {
	name         string
	requiredFlag dbstore.ObjectFlags
	call         func(metadataPermissionFixture) types.Result
}

func metadataPermissionCases() []metadataPermissionCase {
	return []metadataPermissionCase{
		{
			name:         "properties",
			requiredFlag: dbstore.FlagRead,
			call: func(f metadataPermissionFixture) types.Result {
				return builtinProperties(f.ctx, []types.Value{types.NewObj(f.target)})
			},
		},
		{
			name:         "add_property",
			requiredFlag: dbstore.FlagWrite,
			call: func(f metadataPermissionFixture) types.Result {
				return builtinAddProperty(f.ctx, []types.Value{
					types.NewObj(f.target),
					types.NewStr("added"),
					types.NewInt(2),
					types.NewStr("rw"),
				})
			},
		},
		{
			name:         "delete_property",
			requiredFlag: dbstore.FlagWrite,
			call: func(f metadataPermissionFixture) types.Result {
				return builtinDeleteProperty(f.ctx, []types.Value{
					types.NewObj(f.target),
					types.NewStr("existing"),
				})
			},
		},
		{
			name:         "verbs",
			requiredFlag: dbstore.FlagRead,
			call: func(f metadataPermissionFixture) types.Result {
				return builtinVerbs(f.ctx, []types.Value{types.NewObj(f.target)})
			},
		},
		{
			name:         "delete_verb",
			requiredFlag: dbstore.FlagWrite,
			call: func(f metadataPermissionFixture) types.Result {
				return builtinDeleteVerb(f.ctx, []types.Value{
					types.NewObj(f.target),
					types.NewStr("existing"),
				})
			},
		},
	}
}

func TestObjectMetadataBuiltinsDenyUnauthorizedProgrammerWithoutMutation(t *testing.T) {
	for _, test := range metadataPermissionCases() {
		t.Run(test.name, func(t *testing.T) {
			f := newMetadataPermissionFixture(t, 0)
			result := test.call(f)
			if !result.IsError() || result.Error != types.E_PERM {
				t.Fatalf("unauthorized call = %+v, want E_PERM", result)
			}

			if _, exists, errCode := f.ctx.StoreTxn.LocalProperty(f.target, "added"); errCode != types.E_NONE || exists {
				t.Fatalf("denied add_property mutated store: exists=%v error=%s", exists, errCode)
			}
			if _, exists, errCode := f.ctx.StoreTxn.LocalProperty(f.target, "existing"); errCode != types.E_NONE || !exists {
				t.Fatalf("denied delete_property mutated store: exists=%v error=%s", exists, errCode)
			}
			if _, errCode := f.store.FindVerbOnObject(f.target, "existing"); errCode != nil {
				t.Fatalf("denied delete_verb mutated store: %v", errCode)
			}
		})
	}
}

func TestObjectMetadataBuiltinsAllowOwnerObjectFlagAndWizard(t *testing.T) {
	accessors := []struct {
		name      string
		configure func(*metadataPermissionFixture, dbstore.ObjectFlags)
	}{
		{
			name: "owner",
			configure: func(f *metadataPermissionFixture, _ dbstore.ObjectFlags) {
				f.ctx.Programmer = f.owner
				f.ctx.Player = f.owner
			},
		},
		{
			name:      "object_flag",
			configure: func(_ *metadataPermissionFixture, _ dbstore.ObjectFlags) {},
		},
		{
			name: "wizard",
			configure: func(f *metadataPermissionFixture, _ dbstore.ObjectFlags) {
				f.ctx.IsWizard = true
			},
		},
	}

	for _, test := range metadataPermissionCases() {
		for _, accessor := range accessors {
			t.Run(test.name+"/"+accessor.name, func(t *testing.T) {
				flag := dbstore.ObjectFlags(0)
				if accessor.name == "object_flag" {
					flag = test.requiredFlag
				}
				f := newMetadataPermissionFixture(t, flag)
				accessor.configure(&f, test.requiredFlag)
				result := test.call(f)
				if !result.IsNormal() {
					t.Fatalf("authorized call = %+v, want success", result)
				}
			})
		}
	}
}
