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

// Toast checks that every parent is an object before any validity or
// recursion check (objects.cc bf_chparent_chparents), so a non-object after
// a self-parent is E_TYPE, not E_RECMOVE (issue #1151).
func TestChparentsTypeChecksWholeListFirst(t *testing.T) {
	for _, test := range []struct {
		name   string
		target types.ObjID
		list   []types.Value
	}{
		{name: "non-object after self", target: 11, list: []types.Value{types.NewObj(11), types.NewFloat(3e300)}},
		{name: "integer after self", target: 11, list: []types.Value{types.NewObj(11), types.NewInt(2)}},
		{name: "invalid target", target: 999, list: []types.Value{types.NewInt(1)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, _, oldParent, _ := newObjectAuthorityFixture(t, 0, 0)
			ctx.Programmer = 4
			ctx.IsWizard = true

			result := builtinChparents(ctx, []types.Value{types.NewObj(test.target), types.NewList(test.list)})
			if result.Error != types.E_TYPE {
				t.Fatalf("result = %+v, want error E_TYPE", result)
			}
			parents, errCode := readTxn(ctx).Parents(11)
			if errCode != types.E_NONE || len(parents) != 1 || parents[0] != oldParent {
				t.Fatalf("parents after E_TYPE = %v, %s; want [#%d]", parents, errCode, oldParent)
			}
		})
	}
}

// Toast checks chparents in whole-list phases: validity (E_INVARG), then
// permission (E_PERM), then recursion (E_RECMOVE), then duplicates (E_INVARG).
// chparents(o, {#2, #2, o, o}) is E_RECMOVE on Toast, not the duplicate's
// E_INVARG (issue #1186).
func TestChparentsChecksInToastPhaseOrder(t *testing.T) {
	obj := func(id types.ObjID) types.Value { return types.NewObj(id) }
	for _, test := range []struct {
		name       string
		programmer types.ObjID
		isWizard   bool
		target     types.ObjID
		list       []types.Value
		want       types.ErrorCode
	}{
		{name: "self after duplicate", programmer: 4, isWizard: true, target: 11, list: []types.Value{obj(12), obj(12), obj(11), obj(11)}, want: types.E_RECMOVE},
		{name: "invalid parent after self", programmer: 4, isWizard: true, target: 11, list: []types.Value{obj(11), obj(999)}, want: types.E_INVARG},
		{name: "invalid target", programmer: 4, isWizard: true, target: 999, list: []types.Value{obj(12)}, want: types.E_INVARG},
		{name: "permission before recursion", programmer: 3, target: 11, list: []types.Value{obj(11)}, want: types.E_PERM},
		{name: "duplicate without recursion", programmer: 4, isWizard: true, target: 11, list: []types.Value{obj(12), obj(12)}, want: types.E_INVARG},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, _, oldParent, _ := newObjectAuthorityFixture(t, 0, 0)
			ctx.Programmer = test.programmer
			ctx.IsWizard = test.isWizard

			result := builtinChparents(ctx, []types.Value{obj(test.target), types.NewList(test.list)})
			if result.Error != test.want {
				t.Fatalf("result = %+v, want error %s", result, test.want)
			}
			parents, errCode := readTxn(ctx).Parents(11)
			if errCode != types.E_NONE || len(parents) != 1 || parents[0] != oldParent {
				t.Fatalf("parents after %s = %v, %s; want [#%d]", test.want, parents, errCode, oldParent)
			}
		})
	}
}

func TestChparentChecksPermissionBeforeRecursion(t *testing.T) {
	ctx, _, _, _ := newObjectAuthorityFixture(t, 0, 0)
	ctx.Programmer = 3
	ctx.IsWizard = false
	target := types.NewObj(11)
	result := builtinChparent(ctx, []types.Value{target, target})
	if result.Error != types.E_PERM {
		t.Fatalf("unauthorized self-parent change = %+v, want E_PERM", result)
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
