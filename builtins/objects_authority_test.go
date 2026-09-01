package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func newObjectAuthorityFixture(t *testing.T, targetFlags, parentFlags dbstore.ObjectFlags) (*Execution, types.ObjID, types.ObjID, types.ObjID) {
	t.Helper()

	store := dbstore.NewStore()
	for _, object := range []struct {
		id      types.ObjID
		owner   types.ObjID
		parents []types.ObjID
		flags   dbstore.ObjectFlags
	}{
		{id: 0, owner: 0, flags: dbstore.FlagWizard},
		{id: 2, owner: 2, flags: dbstore.FlagProgrammer},
		{id: 3, owner: 3, flags: dbstore.FlagProgrammer},
		{id: 4, owner: 4, flags: dbstore.FlagProgrammer | dbstore.FlagWizard},
		{id: 10, owner: 2},
		{id: 11, owner: 2, parents: []types.ObjID{10}, flags: targetFlags},
		{id: 12, owner: 2, flags: parentFlags},
	} {
		builder := dbstore.NewObjectBuilder(object.id)
		builder.SetOwner(object.owner)
		builder.SetParents(object.parents)
		builder.SetFlags(object.flags)
		if err := store.Add(builder.Build()); err != nil {
			t.Fatalf("add object #%d: %v", object.id, err)
		}
	}

	ctx := newTestExecution()
	ctx.Store = store
	ctx.Player = 0 // Exercise effective task permissions rather than player authority.
	return ctx, 11, 10, 12
}

func TestChangeParentsRequiresObjectControlAndParentFertility(t *testing.T) {
	tests := []struct {
		name        string
		programmer  types.ObjID
		isWizard    bool
		targetFlags dbstore.ObjectFlags
		parentFlags dbstore.ObjectFlags
		want        types.ErrorCode
	}{
		{name: "owner", programmer: 2, parentFlags: 0, want: types.E_NONE},
		{name: "wizard", programmer: 4, isWizard: true, parentFlags: 0, want: types.E_NONE},
		{name: "writable target and fertile parent", programmer: 3, targetFlags: dbstore.FlagWrite, parentFlags: dbstore.FlagFertile, want: types.E_NONE},
		{name: "uncontrolled target", programmer: 3, parentFlags: dbstore.FlagFertile, want: types.E_PERM},
		{name: "infertile parent", programmer: 3, targetFlags: dbstore.FlagWrite, want: types.E_PERM},
	}

	for _, builtin := range []struct {
		name string
		call func(*Execution, types.ObjID, types.ObjID) types.Result
	}{
		{name: "chparent", call: func(ctx *Execution, target, parent types.ObjID) types.Result {
			return builtinChparent(ctx, []types.Value{types.NewObj(target), types.NewObj(parent)})
		}},
		{name: "chparents", call: func(ctx *Execution, target, parent types.ObjID) types.Result {
			return builtinChparents(ctx, []types.Value{types.NewObj(target), types.NewList([]types.Value{types.NewObj(parent)})})
		}},
	} {
		t.Run(builtin.name, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					ctx, target, oldParent, newParent := newObjectAuthorityFixture(t, test.targetFlags, test.parentFlags)
					ctx.Programmer = test.programmer
					ctx.IsWizard = test.isWizard

					result := builtin.call(ctx, target, newParent)
					if result.Error != test.want {
						t.Fatalf("result = %+v, want error %s", result, test.want)
					}

					parents, errCode := ctx.Store.DirectTxn().Parents(target)
					if errCode != types.E_NONE {
						t.Fatalf("parents: %s", errCode)
					}
					wantParent := newParent
					if test.want != types.E_NONE {
						wantParent = oldParent
					}
					if len(parents) != 1 || parents[0] != wantParent {
						t.Fatalf("live parents after result = %v, want [#%d]", parents, wantParent)
					}
					stagedParents, errCode := readTxn(ctx).Parents(target)
					if errCode != types.E_NONE || len(stagedParents) != 1 || stagedParents[0] != wantParent {
						t.Fatalf("transaction parents after result = %v, %s; want [#%d]", stagedParents, errCode, wantParent)
					}
				})
			}
		})
	}
}

func TestRenumberRequiresEffectiveProgrammerWizardAuthority(t *testing.T) {
	for _, test := range []struct {
		name       string
		programmer types.ObjID
		isWizard   bool
		want       types.ErrorCode
	}{
		{name: "lowered task permissions", programmer: 3, want: types.E_PERM},
		{name: "wizard", programmer: 4, isWizard: true, want: types.E_NONE},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, target, _, _ := newObjectAuthorityFixture(t, 0, 0)
			ctx.Programmer = test.programmer
			ctx.IsWizard = test.isWizard

			result := builtinRenumber(ctx, []types.Value{types.NewObj(target)})
			if result.Error != test.want {
				t.Fatalf("renumber result = %+v, want error %s", result, test.want)
			}
			if test.want == types.E_PERM && (!ctx.Store.DirectTxn().Valid(target) || ctx.Store.DirectTxn().Valid(1)) {
				t.Fatal("permission-denied renumber changed live object IDs")
			}
		})
	}

	ctx, _, _, _ := newObjectAuthorityFixture(t, 0, 0)
	ctx.Programmer = 3
	result := builtinRenumber(ctx, []types.Value{types.NewObj(999)})
	if result.Error != types.E_INVARG {
		t.Fatalf("renumber(invalid) = %+v, want E_INVARG before permission denial", result)
	}
}
