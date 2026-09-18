package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

type metadataPermissionFixture struct {
	ctx      *Execution
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
		id, errCode := store.DirectTxn().CreateObject([]types.ObjID{types.ObjNothing}, owner, false)
		if errCode != types.E_NONE {
			t.Fatalf("CreateObject: %s", errCode)
		}
		return id
	}

	owner := create(types.ObjNothing)
	if errCode := store.DirectTxn().SetObjectOwner(owner, owner); errCode != types.E_NONE {
		t.Fatalf("SetObjectOwner(owner): %s", errCode)
	}
	target := create(owner)
	intruder := create(owner)
	if errCode := store.DirectTxn().SetObjectOwner(intruder, intruder); errCode != types.E_NONE {
		t.Fatalf("SetObjectOwner(intruder): %s", errCode)
	}
	if targetFlag != 0 {
		if errCode := store.DirectTxn().SetObjectFlag(target, targetFlag, true); errCode != types.E_NONE {
			t.Fatalf("SetObjectFlag(target): %s", errCode)
		}
	}
	if errCode := store.DirectTxn().DefineProperty(
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

	ctx := newTestExecution()
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
			if _, errCode := f.store.DirectTxn().FindVerbOnObject(f.target, "existing"); errCode != nil {
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

func TestDeleteVerbPreservesNotFoundPrecedenceForUnauthorizedProgrammer(t *testing.T) {
	for _, descriptor := range []types.Value{types.NewStr("missing"), types.NewInt(99)} {
		f := newMetadataPermissionFixture(t, 0)
		result := builtinDeleteVerb(f.ctx, []types.Value{
			types.NewObj(f.target),
			descriptor,
		})
		if !result.IsError() || result.Error != types.E_VERBNF {
			t.Fatalf("missing descriptor %v = %+v, want E_VERBNF", descriptor, result)
		}
	}
}

func TestDeleteVerbWithoutTransactionUsesAtomicLiveFallback(t *testing.T) {
	tests := []struct {
		name       string
		authorized bool
		want       types.ErrorCode
	}{
		{name: "owner", authorized: true, want: types.E_NONE},
		{name: "denied", want: types.E_PERM},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newMetadataPermissionFixture(t, 0)
			f.ctx.StoreTxn.Release()
			f.ctx.StoreTxn = nil
			if test.authorized {
				f.ctx.Programmer = f.owner
				f.ctx.Player = f.owner
			}

			result := builtinDeleteVerb(f.ctx, []types.Value{
				types.NewObj(f.target),
				types.NewStr("existing"),
			})
			if test.want == types.E_NONE {
				if !result.IsNormal() {
					t.Fatalf("delete_verb no-txn fallback = %+v, want success", result)
				}
				if _, err := f.store.DirectTxn().FindVerbOnObject(f.target, "existing"); err == nil {
					t.Fatal("authorized no-txn fallback left verb live")
				}
				return
			}
			if !result.IsError() || result.Error != test.want {
				t.Fatalf("delete_verb no-txn fallback = %+v, want %s", result, test.want)
			}
			if _, err := f.store.DirectTxn().FindVerbOnObject(f.target, "existing"); err != nil {
				t.Fatalf("denied no-txn fallback removed verb: %v", err)
			}
		})
	}
}

func TestDeniedDeleteVerbDoesNotFlushStagedTopology(t *testing.T) {
	f := newMetadataPermissionFixture(t, 0)
	staged, errCode := f.ctx.StoreTxn.CreateObject(
		[]types.ObjID{types.ObjNothing},
		f.intruder,
	)
	if errCode != types.E_NONE {
		t.Fatalf("stage CreateObject: %s", errCode)
	}
	if f.store.DirectTxn().Valid(staged) {
		t.Fatalf("staged object #%d unexpectedly exists in live store before denial", staged)
	}

	result := builtinDeleteVerb(f.ctx, []types.Value{
		types.NewObj(f.target),
		types.NewStr("existing"),
	})
	if !result.IsError() || result.Error != types.E_PERM {
		t.Fatalf("unauthorized delete_verb = %+v, want E_PERM", result)
	}
	if f.store.DirectTxn().Valid(staged) {
		t.Fatalf("denied delete_verb flushed staged object #%d to live store", staged)
	}
}

func TestObjectMetadataBuiltinsUseSameTaskStagedAuthority(t *testing.T) {
	authorityCases := []struct {
		name        string
		initialFlag func(dbstore.ObjectFlags) dbstore.ObjectFlags
		programmer  func(metadataPermissionFixture) types.ObjID
		stage       func(*testing.T, metadataPermissionFixture, dbstore.ObjectFlags)
		want        types.ErrorCode
	}{
		{
			name:        "staged owner grant",
			initialFlag: func(dbstore.ObjectFlags) dbstore.ObjectFlags { return 0 },
			programmer:  func(f metadataPermissionFixture) types.ObjID { return f.intruder },
			stage: func(t *testing.T, f metadataPermissionFixture, _ dbstore.ObjectFlags) {
				t.Helper()
				if errCode := f.ctx.StoreTxn.SetObjectOwner(f.target, f.intruder); errCode != types.E_NONE {
					t.Fatalf("SetObjectOwner(grant): %s", errCode)
				}
			},
			want: types.E_NONE,
		},
		{
			name:        "staged owner revocation",
			initialFlag: func(dbstore.ObjectFlags) dbstore.ObjectFlags { return 0 },
			programmer:  func(f metadataPermissionFixture) types.ObjID { return f.owner },
			stage: func(t *testing.T, f metadataPermissionFixture, _ dbstore.ObjectFlags) {
				t.Helper()
				if errCode := f.ctx.StoreTxn.SetObjectOwner(f.target, f.intruder); errCode != types.E_NONE {
					t.Fatalf("SetObjectOwner(revoke): %s", errCode)
				}
			},
			want: types.E_PERM,
		},
		{
			name:        "staged object flag grant",
			initialFlag: func(dbstore.ObjectFlags) dbstore.ObjectFlags { return 0 },
			programmer:  func(f metadataPermissionFixture) types.ObjID { return f.intruder },
			stage: func(t *testing.T, f metadataPermissionFixture, required dbstore.ObjectFlags) {
				t.Helper()
				if errCode := f.ctx.StoreTxn.SetObjectFlag(f.target, required, true); errCode != types.E_NONE {
					t.Fatalf("SetObjectFlag(grant): %s", errCode)
				}
			},
			want: types.E_NONE,
		},
		{
			name:        "staged object flag revocation",
			initialFlag: func(required dbstore.ObjectFlags) dbstore.ObjectFlags { return required },
			programmer:  func(f metadataPermissionFixture) types.ObjID { return f.intruder },
			stage: func(t *testing.T, f metadataPermissionFixture, required dbstore.ObjectFlags) {
				t.Helper()
				if errCode := f.ctx.StoreTxn.SetObjectFlag(f.target, required, false); errCode != types.E_NONE {
					t.Fatalf("SetObjectFlag(revoke): %s", errCode)
				}
			},
			want: types.E_PERM,
		},
	}

	for _, builtinCase := range metadataPermissionCases() {
		for _, authorityCase := range authorityCases {
			t.Run(builtinCase.name+"/"+authorityCase.name, func(t *testing.T) {
				f := newMetadataPermissionFixture(t, authorityCase.initialFlag(builtinCase.requiredFlag))
				f.ctx.Programmer = authorityCase.programmer(f)
				f.ctx.Player = f.ctx.Programmer
				authorityCase.stage(t, f, builtinCase.requiredFlag)

				result := builtinCase.call(f)
				if authorityCase.want == types.E_NONE {
					if !result.IsNormal() {
						t.Fatalf("%s with staged authority = %+v, want success", builtinCase.name, result)
					}
				} else if !result.IsError() || result.Error != authorityCase.want {
					t.Fatalf("%s with staged authority = %+v, want %s", builtinCase.name, result, authorityCase.want)
				}
			})
		}
	}
}

func addMetadataTestVerb(t *testing.T, f metadataPermissionFixture, name string, names []string) int {
	t.Helper()
	verb := dbstore.NewVerb(
		name,
		names,
		f.owner,
		dbstore.VerbRead|dbstore.VerbWrite|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "none", Prep: "none", That: "none"},
		nil,
	)
	index, errCode := f.store.AddVerb(f.target, verb)
	if errCode != types.E_NONE {
		t.Fatalf("AddVerb(%q): %s", name, errCode)
	}
	if errCode := f.ctx.StoreTxn.AdoptLiveVerbs(f.target); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveVerbs(%q): %s", name, errCode)
	}
	return index
}

func TestAuthorizedDeleteVerbCommitsWithStagedTopologyAndSurvivorCode(t *testing.T) {
	tests := []struct {
		name       string
		descriptor types.Value
	}{
		{name: "string", descriptor: types.NewStr("existing")},
		{name: "index", descriptor: types.NewInt(1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newMetadataPermissionFixture(t, 0)
			f.ctx.Programmer = f.owner
			f.ctx.Player = f.owner
			addMetadataTestVerb(t, f, "survivor", []string{"survivor"})

			if errCode := f.ctx.StoreTxn.SetVerbCode(f.target, "survivor", []string{"return 2;"}); errCode != types.E_NONE {
				t.Fatalf("stage SetVerbCode: %s", errCode)
			}
			staged, errCode := f.ctx.StoreTxn.CreateObject(
				[]types.ObjID{types.ObjNothing},
				f.owner,
			)
			if errCode != types.E_NONE {
				t.Fatalf("stage CreateObject: %s", errCode)
			}
			if f.store.DirectTxn().Valid(staged) {
				t.Fatalf("staged object #%d unexpectedly exists in live store before delete_verb", staged)
			}

			result := builtinDeleteVerb(f.ctx, []types.Value{
				types.NewObj(f.target),
				test.descriptor,
			})
			if !result.IsNormal() {
				t.Fatalf("authorized staged delete_verb = %+v, want success", result)
			}
			if f.store.DirectTxn().Valid(staged) {
				t.Fatalf("delete_verb published staged object #%d before commit", staged)
			}
			if _, err := f.store.DirectTxn().FindVerbOnObject(f.target, "existing"); err != nil {
				t.Fatalf("delete_verb mutated live target before commit: %v", err)
			}
			if _, err := f.ctx.StoreTxn.FindVerbOnObject(f.target, "existing"); err == nil {
				t.Fatal("transaction view retained staged deletion target")
			}
			if errCode := f.ctx.StoreTxn.Commit(); errCode != types.E_NONE {
				t.Fatalf("Commit: %s", errCode)
			}
			if !f.store.DirectTxn().Valid(staged) {
				t.Fatalf("commit did not publish staged object #%d", staged)
			}
			if _, err := f.store.DirectTxn().FindVerbOnObject(f.target, "existing"); err == nil {
				t.Fatal("commit left the resolved target in the store")
			}
			survivor, err := f.store.DirectTxn().FindVerbOnObject(f.target, "survivor")
			if err != nil {
				t.Fatalf("FindVerbOnObject(survivor): %v", err)
			}
			if len(survivor.Code) != 1 || survivor.Code[0] != "return 2;" {
				t.Fatalf("flushed survivor code = %v, want [return 2;]", survivor.Code)
			}
		})
	}
}

func TestDeleteVerbDeletesLoadedMultiAliasByResolvedDescriptor(t *testing.T) {
	tests := []struct {
		name       string
		descriptor func(int) types.Value
	}{
		{
			name: "string",
			descriptor: func(_ int) types.Value {
				return types.NewStr("glance")
			},
		},
		{
			name: "index",
			descriptor: func(index int) types.Value {
				return types.NewInt(int64(index))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newMetadataPermissionFixture(t, 0)
			f.ctx.Programmer = f.owner
			f.ctx.Player = f.owner

			// The database loader retains the complete, space-separated names
			// field as Verb.Name while also storing each alias in Verb.Names.
			index := addMetadataTestVerb(t, f, "look l*ook glance", []string{"look", "l*ook", "glance"})
			result := builtinDeleteVerb(f.ctx, []types.Value{
				types.NewObj(f.target),
				test.descriptor(index),
			})
			if !result.IsNormal() {
				t.Fatalf("delete_verb loaded multi-alias by %s = %+v, want success", test.name, result)
			}
			if _, err := f.store.DirectTxn().FindVerbOnObject(f.target, "glance"); err != nil {
				t.Fatalf("staged delete mutated live multi-alias verb: %v", err)
			}
			if errCode := f.ctx.StoreTxn.Commit(); errCode != types.E_NONE {
				t.Fatalf("Commit: %s", errCode)
			}
			if _, err := f.store.DirectTxn().FindVerbOnObject(f.target, "glance"); err == nil {
				t.Fatal("delete_verb left the resolved loaded multi-alias verb in the store")
			}
			if _, err := f.store.DirectTxn().FindVerbOnObject(f.target, "existing"); err != nil {
				t.Fatalf("delete_verb removed the wrong verb: %v", err)
			}
		})
	}
}

func TestDeleteVerbDeletesExactResolvedVerbWhenAliasesOverlap(t *testing.T) {
	tests := []struct {
		name       string
		descriptor func(int) types.Value
	}{
		{
			name: "string",
			descriptor: func(_ int) types.Value {
				return types.NewStr("peek")
			},
		},
		{
			name: "index",
			descriptor: func(index int) types.Value {
				return types.NewInt(int64(index))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newMetadataPermissionFixture(t, 0)
			f.ctx.Programmer = f.owner
			f.ctx.Player = f.owner

			addMetadataTestVerb(t, f, "look", []string{"look", "glance"})
			targetIndex := addMetadataTestVerb(t, f, "glance", []string{"glance", "peek"})

			result := builtinDeleteVerb(f.ctx, []types.Value{
				types.NewObj(f.target),
				test.descriptor(targetIndex),
			})
			if !result.IsNormal() {
				t.Fatalf("delete_verb overlapping alias by %s = %+v, want success", test.name, result)
			}
			if _, err := f.store.DirectTxn().FindVerbOnObject(f.target, "peek"); err != nil {
				t.Fatalf("staged delete mutated live overlapping target: %v", err)
			}
			if errCode := f.ctx.StoreTxn.Commit(); errCode != types.E_NONE {
				t.Fatalf("Commit: %s", errCode)
			}
			if _, err := f.store.DirectTxn().FindVerbOnObject(f.target, "look"); err != nil {
				t.Fatalf("delete_verb removed the earlier overlapping verb: %v", err)
			}
			if _, err := f.store.DirectTxn().FindVerbOnObject(f.target, "peek"); err == nil {
				t.Fatal("delete_verb left the exactly resolved verb in the store")
			}
		})
	}
}
