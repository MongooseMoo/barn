package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func codeList(lines ...types.Value) types.Value {
	return types.NewList(lines)
}

func wantBuiltinError(t *testing.T, ctx *Execution, name string, args []types.Value, want types.ErrorCode) {
	t.Helper()
	result := admitTestBuiltin(name, ctx, args)
	if !result.IsError() || result.Error != want {
		t.Errorf("%s%v = %v, want %v", name, args, result, want)
	}
}

// refreshSnapshot makes objects added directly to the store visible to ctx.
func refreshSnapshot(ctx *Execution, store *dbstore.Store) {
	ctx.StoreTxn.Release()
	ctx.StoreTxn = store.BeginSnapshot(0)
}

// Toast bf_disassemble (src/disassemble.cc:478) uses find_described_verb, which
// looks only at verbs defined on the object itself.
func TestDisassembleIgnoresInheritedVerbs(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	builder := dbstore.NewObjectBuilder(1)
	builder.SetOwner(0)
	builder.SetParents([]types.ObjID{0})
	if err := store.Add(builder.Build()); err != nil {
		t.Fatalf("add child: %v", err)
	}
	refreshSnapshot(ctx, store)

	if result := admitTestBuiltin("disassemble", ctx, []types.Value{types.NewObj(0), types.NewStr("look")}); result.IsError() {
		t.Fatalf("disassemble(#0, \"look\") = %v, want a listing", result)
	}
	wantBuiltinError(t, ctx, "disassemble", []types.Value{types.NewObj(1), types.NewStr("look")}, types.E_VERBNF)
}

// Toast bf_add_verb (src/verbs.cc:175-214) validates the info list, then the
// args list (E_TYPE for a wrong shape), and only then the object.
func TestAddVerbValidatesListsBeforeObject(t *testing.T) {
	ctx, _ := verbMetadataTxnTestContext(t)
	owner := types.NewObj(0)
	goodInfo := verbInfoList(owner, "rx", "x")
	goodArgs := verbArgsList("this", "none", "this")
	shortInfo := types.NewList([]types.Value{owner, types.NewStr("rx")})
	shortArgs := types.NewList([]types.Value{types.NewStr("this"), types.NewStr("none")})

	wantBuiltinError(t, ctx, "add_verb", []types.Value{types.NewObj(0), shortInfo, goodArgs}, types.E_TYPE)
	wantBuiltinError(t, ctx, "add_verb", []types.Value{types.NewObj(0), goodInfo, shortArgs}, types.E_TYPE)
	wantBuiltinError(t, ctx, "add_verb", []types.Value{types.NewInt(1), verbInfoList(owner, "q", "x"), goodArgs}, types.E_INVARG)
	wantBuiltinError(t, ctx, "add_verb", []types.Value{types.NewObj(99), verbInfoList(types.NewInt(1), "rx", "x"), goodArgs}, types.E_TYPE)
	wantBuiltinError(t, ctx, "add_verb", []types.Value{types.NewObj(99), goodInfo, shortArgs}, types.E_TYPE)
	wantBuiltinError(t, ctx, "add_verb", []types.Value{types.NewObj(99), goodInfo, goodArgs}, types.E_INVARG)

	// Only spaces separate names (validate_verb_info, src/verbs.cc:114-121).
	if result := admitTestBuiltin("add_verb", ctx, []types.Value{types.NewObj(0), verbInfoList(owner, "rx", "\t"), goodArgs}); result.IsError() {
		t.Fatalf("add_verb with a tab name = %v, want success", result)
	}
	if got := mustVerbInfo(t, ctx, types.NewStr("\t"))[2].Str(); got != "\t" {
		t.Fatalf("verb names = %q, want a tab", got)
	}
}

// Toast's verb readers (bf_verb_info src/verbs.cc:285-286, bf_verb_args 395-396,
// bf_verb_code 483-484, bf_set_verb_code 523-524) check the descriptor before
// object validity, and an invalid object is E_INVARG.
func TestVerbReadersCheckDescriptorBeforeObject(t *testing.T) {
	ctx, _ := verbMetadataTxnTestContext(t)
	for _, call := range []struct {
		name  string
		extra []types.Value
	}{
		{"verb_info", nil},
		{"verb_args", nil},
		{"verb_code", nil},
		{"disassemble", nil},
		{"set_verb_code", []types.Value{codeList()}},
	} {
		bad := append([]types.Value{types.NewObj(99), types.NewFloat(1.5)}, call.extra...)
		wantBuiltinError(t, ctx, call.name, bad, types.E_TYPE)
		named := append([]types.Value{types.NewObj(99), types.NewStr("x")}, call.extra...)
		wantBuiltinError(t, ctx, call.name, named, types.E_INVARG)
	}
}

// Toast bf_set_verb_code (src/verbs.cc:515-520) rejects a non-string code line
// before it checks the object, the descriptor, the verb or permission.
func TestSetVerbCodeChecksLinesFirst(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	code := codeList(types.NewStr("return 1;"), types.NewInt(2))

	wantBuiltinError(t, ctx, "set_verb_code", []types.Value{types.NewObj(99), types.NewStr("x"), code}, types.E_TYPE)
	wantBuiltinError(t, ctx, "set_verb_code", []types.Value{types.NewObj(0), types.NewStr("nosuch"), code}, types.E_TYPE)

	addTxnObject(t, store, 1, 1, dbstore.FlagProgrammer)
	refreshSnapshot(ctx, store)
	ctx.IsWizard = false
	ctx.Player = 1
	ctx.Programmer = 1
	wantBuiltinError(t, ctx, "set_verb_code", []types.Value{types.NewObj(0), types.NewStr("look"), code}, types.E_TYPE)
}

// Toast set_prop_info (src/property.cc:149-188) checks the object, then the
// whole info list (validate_prop_info, 109-146), then the lookup, then
// db_property_allows(PF_WRITE): the w flag, the owner or a wizard.
func TestSetPropertyInfoPrecedence(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	if errCode := store.DirectTxn().DefineProperty(0, "private", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty: %v", errCode)
	}
	addTxnObject(t, store, 1, 1, dbstore.FlagProgrammer)
	refreshSnapshot(ctx, store)
	owner := types.NewObj(0)
	info := func(values ...types.Value) types.Value { return types.NewList(values) }
	call := func(obj types.Value, name string, i types.Value) []types.Value {
		return []types.Value{obj, types.NewStr(name), i}
	}

	wantBuiltinError(t, ctx, "set_property_info", call(types.NewObj(99), "marker", info(types.NewInt(1))), types.E_INVARG)
	wantBuiltinError(t, ctx, "set_property_info", call(types.NewAnon(0), "marker", info(owner, types.NewStr("r"))), types.E_TYPE)
	wantBuiltinError(t, ctx, "set_property_info", call(owner, "nosuch", info(owner, types.NewStr("q"))), types.E_INVARG)
	wantBuiltinError(t, ctx, "set_property_info", call(owner, "nosuch", info(owner)), types.E_TYPE)
	wantBuiltinError(t, ctx, "set_property_info", call(owner, "nosuch", info(owner, types.NewStr("r"), types.NewInt(1))), types.E_TYPE)
	wantBuiltinError(t, ctx, "set_property_info", call(owner, "marker", info(types.NewObj(99), types.NewStr("r"))), types.E_INVARG)

	ctx.IsWizard = false
	ctx.Player = 1
	ctx.Programmer = 1
	wantBuiltinError(t, ctx, "set_property_info", call(owner, "private", info(owner, types.NewStr("q"))), types.E_INVARG)
	wantBuiltinError(t, ctx, "set_property_info", call(owner, "private", info(owner, types.NewStr("rw"))), types.E_PERM)

	// "marker" has the w flag, so a non-owner may change its flags.
	if result := admitTestBuiltin("set_property_info", ctx, call(owner, "marker", info(owner, types.NewStr("rwc")))); result.IsError() {
		t.Fatalf("set_property_info on a w-flag property = %v, want success", result)
	}
	got := admitTestBuiltin("property_info", ctx, []types.Value{owner, types.NewStr("marker")})
	if got.IsError() || got.Val.Get(1).ID() != 0 || got.Val.Get(2).Str() != "rwc" {
		t.Fatalf("property_info after = %v, want {#0, \"rwc\"}", got)
	}
}

// Toast set_prop_info renames with a three-element info list (src/property.cc:
// 178-183): db_rename_propdef (src/db_properties.cc:251-280) renames only a
// property the object defines, and unless only the case changes, not to a
// built-in name or one defined on the object, an ancestor or a descendant.
func TestSetPropertyInfoRenames(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	d := store.DirectTxn()
	if errCode := d.DefineProperty(0, "other", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define other: %v", errCode)
	}
	child, errCode := d.CreateObject([]types.ObjID{0}, 0)
	if errCode != types.E_NONE {
		t.Fatalf("create child: %v", errCode)
	}
	grandchild, errCode := d.CreateObject([]types.ObjID{child}, 0)
	if errCode != types.E_NONE {
		t.Fatalf("create grandchild: %v", errCode)
	}
	if errCode := d.DefineProperty(grandchild, "below", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define below: %v", errCode)
	}
	if errCode := d.SetPropertyValue(child, "marker", types.NewInt(5)); errCode != types.E_NONE {
		t.Fatalf("set child marker: %v", errCode)
	}
	refreshSnapshot(ctx, store)
	owner := types.NewObj(0)
	rename := func(obj types.Value, from, to string) []types.Value {
		return []types.Value{obj, types.NewStr(from), types.NewList([]types.Value{owner, types.NewStr("rw"), types.NewStr(to)})}
	}

	for _, to := range []string{"other", "OTHER", "below", "name", "A", "last_move"} {
		wantBuiltinError(t, ctx, "set_property_info", rename(owner, "marker", to), types.E_INVARG)
	}
	wantBuiltinError(t, ctx, "set_property_info", rename(types.NewObj(child), "marker", "renamed"), types.E_INVARG)

	if result := admitTestBuiltin("set_property_info", ctx, rename(owner, "marker", "Renamed")); result.IsError() {
		t.Fatalf("rename = %v, want success", result)
	}
	if got := admitTestBuiltin("properties", ctx, []types.Value{owner}); got.IsError() || got.Val.Len() != 2 || got.Val.Get(1).Str() != "Renamed" {
		t.Fatalf("properties(#0) = %v, want {\"Renamed\", \"other\"}", got)
	}
	wantBuiltinError(t, ctx, "property_info", []types.Value{owner, types.NewStr("marker")}, types.E_PROPNF)
	wantBuiltinError(t, ctx, "property_info", []types.Value{types.NewObj(child), types.NewStr("marker")}, types.E_PROPNF)
	if v, errCode := readTxn(ctx).PropertyValue(child, "renamed"); errCode != types.E_NONE || !v.Identical(types.NewInt(5)) {
		t.Fatalf("child.renamed = %v, %v; want 5", v, errCode)
	}
	if result := admitTestBuiltin("set_property_info", ctx, rename(owner, "renamed", "RENAMED")); result.IsError() {
		t.Fatalf("case-only rename = %v, want success", result)
	}
}

// Toast bf_add_prop (src/property.cc:206-237) validates the info list; then a
// new-name element or anything but a numbered object is E_TYPE, an invalid
// object E_INVARG, a programmer who may not write the object E_PERM, and only
// then a name in use E_INVARG.
func TestAddPropertyPrecedence(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	addTxnObject(t, store, 1, 1, dbstore.FlagProgrammer)
	addTxnObject(t, store, 2, 0)
	if errCode := store.DirectTxn().DefineProperty(2, "taken", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define taken: %v", errCode)
	}
	refreshSnapshot(ctx, store)
	owner := types.NewObj(0)
	info := func(values ...types.Value) types.Value { return types.NewList(values) }
	call := func(obj types.Value, i types.Value) []types.Value {
		return []types.Value{obj, types.NewStr("added"), types.NewInt(1), i}
	}
	three := info(owner, types.NewStr("r"), types.NewStr("y"))

	wantBuiltinError(t, ctx, "add_property", call(owner, three), types.E_TYPE)
	wantBuiltinError(t, ctx, "add_property", call(types.NewObj(99), three), types.E_TYPE)
	wantBuiltinError(t, ctx, "add_property", call(types.NewInt(1), three), types.E_TYPE)
	wantBuiltinError(t, ctx, "add_property", call(types.NewObj(99), info(owner, types.NewStr("q"), types.NewStr("y"))), types.E_INVARG)
	wantBuiltinError(t, ctx, "add_property", call(types.NewObj(99), info(types.NewInt(1), types.NewStr("r"))), types.E_TYPE)
	wantBuiltinError(t, ctx, "add_property", call(types.NewInt(1), info(types.NewObj(99), types.NewStr("r"))), types.E_INVARG)
	wantBuiltinError(t, ctx, "add_property", call(types.NewObj(99), info(owner, types.NewStr("r"))), types.E_INVARG)
	wantBuiltinError(t, ctx, "add_property", call(types.NewAnon(0), info(owner, types.NewStr("r"))), types.E_TYPE)
	if _, exists, errCode := readTxn(ctx).LocalProperty(0, "added"); errCode != types.E_NONE || exists {
		t.Fatalf("a refused add_property added the property: exists=%v %v", exists, errCode)
	}

	ctx.IsWizard = false
	ctx.Player = 1
	ctx.Programmer = 1
	wantBuiltinError(t, ctx, "add_property", []types.Value{types.NewObj(2), types.NewStr("taken"), types.NewInt(1), info(types.NewObj(1), types.NewStr("r"))}, types.E_PERM)
}

// Toast's verb builtins that change verbs, and verb_args, take a numbered
// object only (is_obj: src/verbs.cc:192, 249, 334, 392, 433, 520). Those that
// check the verb descriptor first (delete_verb, set_verb_info, set_verb_args)
// still do. The anonymous reference here carries #0's id, which has "look".
func TestVerbBuiltinsRejectAnonymousObject(t *testing.T) {
	ctx, _ := verbMetadataTxnTestContext(t)
	anon := types.NewAnon(0)
	look, zero := types.NewStr("look"), types.NewInt(0)
	info := verbInfoList(types.NewObj(0), "rx", "look")
	args := verbArgsList("this", "none", "this")
	for _, c := range []struct {
		name string
		args []types.Value
		want types.ErrorCode
	}{
		{"verb_args", []types.Value{anon, look}, types.E_TYPE},
		{"verb_args", []types.Value{anon, zero}, types.E_TYPE},
		{"set_verb_info", []types.Value{anon, look, info}, types.E_TYPE},
		{"set_verb_info", []types.Value{anon, zero, info}, types.E_INVARG},
		{"set_verb_args", []types.Value{anon, look, args}, types.E_TYPE},
		{"set_verb_args", []types.Value{anon, zero, args}, types.E_INVARG},
		{"set_verb_code", []types.Value{anon, look, codeList(types.NewStr("return 2;"))}, types.E_TYPE},
		{"set_verb_code", []types.Value{anon, zero, codeList()}, types.E_TYPE},
		{"delete_verb", []types.Value{anon, look}, types.E_TYPE},
		{"delete_verb", []types.Value{anon, zero}, types.E_INVARG},
		{"add_verb", []types.Value{anon, info, args}, types.E_TYPE},
	} {
		wantBuiltinError(t, ctx, c.name, c.args, c.want)
	}
	if got := mustVerbInfo(t, ctx, look); got[0].ID() != 0 || got[1].Str() != "rwxd" {
		t.Fatalf("verb_info(#0, \"look\") = %v, want it unchanged", got)
	}
	if got := admitTestBuiltin("verb_code", ctx, []types.Value{types.NewObj(0), look}); got.IsError() || got.Val.Len() != 1 || got.Val.Get(1).Str() != "return 1;" {
		t.Fatalf("verb_code(#0, \"look\") = %v, want {\"return 1;\"}", got)
	}
}
