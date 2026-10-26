package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func verbInfoList(owner types.Value, flags, names string) types.Value {
	return types.NewList([]types.Value{owner, types.NewStr(flags), types.NewStr(names)})
}

func verbArgsList(dobj, prep, iobj string) types.Value {
	return types.NewList([]types.Value{types.NewStr(dobj), types.NewStr(prep), types.NewStr(iobj)})
}

func mustVerbInfo(t *testing.T, ctx *Execution, desc types.Value) []types.Value {
	t.Helper()
	result := admitTestBuiltin("verb_info", ctx, []types.Value{types.NewObj(0), desc})
	if result.IsError() {
		t.Fatalf("verb_info(#0, %v) failed: %v", desc, result.Error)
	}
	return []types.Value{result.Val.Get(1), result.Val.Get(2), result.Val.Get(3)}
}

// Toast validate_verb_info (src/verbs.cc:92-112) accepts r/w/x/d in either case.
func TestVerbFlagLettersAcceptEitherCase(t *testing.T) {
	ctx, _ := verbMetadataTxnTestContext(t)

	result := admitTestBuiltin("add_verb", ctx, []types.Value{types.NewObj(0), verbInfoList(types.NewObj(0), "RWXD", "upper"), verbArgsList("this", "none", "this")})
	if result.IsError() {
		t.Fatalf("add_verb failed: %v", result.Error)
	}
	if got := mustVerbInfo(t, ctx, types.NewStr("upper"))[1].Str(); got != "rwxd" {
		t.Fatalf("add_verb flags = %q, want rwxd", got)
	}

	result = admitTestBuiltin("set_verb_info", ctx, []types.Value{types.NewObj(0), types.NewStr("look"), verbInfoList(types.NewObj(0), "Rx", "look")})
	if result.IsError() {
		t.Fatalf("set_verb_info failed: %v", result.Error)
	}
	if got := mustVerbInfo(t, ctx, types.NewStr("look"))[1].Str(); got != "rx" {
		t.Fatalf("set_verb_info flags = %q, want rx", got)
	}
}

// Toast validate_verb_descriptor and find_described_verb (src/verbs.cc:216-237):
// an INT descriptor <= 0 is E_INVARG and an index past the last verb is E_VERBNF.
func TestVerbDescriptorIntegerErrors(t *testing.T) {
	calls := []struct {
		name  string
		extra []types.Value
	}{
		{"verb_info", nil},
		{"verb_args", nil},
		{"verb_code", nil},
		{"disassemble", nil},
		{"delete_verb", nil},
		{"set_verb_info", []types.Value{verbInfoList(types.NewObj(0), "rx", "renamed")}},
		{"set_verb_args", []types.Value{verbArgsList("any", "none", "any")}},
		{"set_verb_code", []types.Value{types.NewList([]types.Value{types.NewStr("return 2;")})}},
	}
	descs := []struct {
		desc int64
		want types.ErrorCode
	}{
		{0, types.E_INVARG},
		{-1, types.E_INVARG},
		{2, types.E_VERBNF},
	}
	for _, call := range calls {
		for _, d := range descs {
			ctx, _ := verbMetadataTxnTestContext(t)
			args := append([]types.Value{types.NewObj(0), types.NewInt(d.desc)}, call.extra...)
			result := admitTestBuiltin(call.name, ctx, args)
			if !result.IsError() || result.Error != d.want {
				t.Errorf("%s(#0, %d) = %v, want %v", call.name, d.desc, result, d.want)
			}
		}
	}
}

// Toast set_verb_info and set_verb_args take an INT descriptor through
// find_described_verb (src/verbs.cc:346, 444).
func TestVerbSettersIntegerDescriptor(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)
	if _, errCode := store.AddVerb(0, dbstore.NewVerb([]string{"second"}, 0, dbstore.VerbRead, dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}
	ctx.StoreTxn.Release()
	ctx.StoreTxn = store.BeginSnapshot(0)

	if result := admitTestBuiltin("set_verb_info", ctx, []types.Value{types.NewObj(0), types.NewInt(2), verbInfoList(types.NewObj(0), "rxd", "renamed")}); result.IsError() {
		t.Fatalf("set_verb_info(#0, 2) failed: %v", result.Error)
	}
	if result := admitTestBuiltin("set_verb_args", ctx, []types.Value{types.NewObj(0), types.NewInt(2), verbArgsList("any", "none", "any")}); result.IsError() {
		t.Fatalf("set_verb_args(#0, 2) failed: %v", result.Error)
	}
	first := mustVerbInfo(t, ctx, types.NewInt(1))
	second := mustVerbInfo(t, ctx, types.NewInt(2))
	if first[2].Str() != "look" || second[2].Str() != "renamed" || second[1].Str() != "rxd" {
		t.Fatalf("verb_info after set_verb_info(#0, 2) = %v, %v", first, second)
	}
	args := admitTestBuiltin("verb_args", ctx, []types.Value{types.NewObj(0), types.NewInt(2)})
	if args.IsError() || args.Val.Get(1).Str() != "any" || args.Val.Get(3).Str() != "any" {
		t.Fatalf("verb_args(#0, 2) = %v, want {any, none, any}", args)
	}
	args = admitTestBuiltin("verb_args", ctx, []types.Value{types.NewObj(0), types.NewInt(1)})
	if args.IsError() || args.Val.Get(1).Str() != "none" {
		t.Fatalf("verb_args(#0, 1) = %v, want unchanged {none, none, none}", args)
	}
}

// Toast db_verb_allows (src/db_verbs.cc:880-885) lets a non-owner change a verb
// whose flags include w; set_verb_info still refuses a new owner from a
// non-wizard (src/verbs.cc:353-354).
func TestVerbSettersHonorWriteFlag(t *testing.T) {
	cases := []struct {
		name  string
		verb  string
		fn    string
		third types.Value
		want  types.ErrorCode
	}{
		{"info w flag", "look", "set_verb_info", verbInfoList(types.NewObj(0), "rw", "look"), types.E_NONE},
		{"args w flag", "look", "set_verb_args", verbArgsList("any", "none", "any"), types.E_NONE},
		{"code w flag", "look", "set_verb_code", types.NewList([]types.Value{types.NewStr("return 2;")}), types.E_NONE},
		{"info w flag owner change", "look", "set_verb_info", verbInfoList(types.NewObj(1), "rw", "look"), types.E_PERM},
		{"info no w flag", "nw", "set_verb_info", verbInfoList(types.NewObj(0), "rx", "nw"), types.E_PERM},
		{"args no w flag", "nw", "set_verb_args", verbArgsList("any", "none", "any"), types.E_PERM},
		{"code no w flag", "nw", "set_verb_code", types.NewList([]types.Value{types.NewStr("return 2;")}), types.E_PERM},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, store := verbMetadataTxnTestContext(t)
			addTxnObject(t, store, 1, 1, dbstore.FlagProgrammer)
			if _, errCode := store.AddVerb(0, dbstore.NewVerb([]string{"nw"}, 0, dbstore.VerbRead|dbstore.VerbExecute, dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, nil)); errCode != types.E_NONE {
				t.Fatalf("AddVerb failed: %v", errCode)
			}
			ctx.StoreTxn.Release()
			ctx.StoreTxn = store.BeginSnapshot(0)
			ctx.IsWizard = false
			ctx.Player = 1
			ctx.Programmer = 1

			result := admitTestBuiltin(tc.fn, ctx, []types.Value{types.NewObj(0), types.NewStr(tc.verb), tc.third})
			if tc.want == types.E_NONE && result.IsError() {
				t.Fatalf("got %v, want success", result)
			}
			if tc.want != types.E_NONE && (!result.IsError() || result.Error != tc.want) {
				t.Fatalf("got %v, want %v", result, tc.want)
			}
			if owner := mustVerbInfo(t, ctx, types.NewStr(tc.verb))[0]; owner.ID() != 0 {
				t.Fatalf("owner = %v, want #0", owner)
			}
		})
	}
}

// Toast validate_verb_info (src/verbs.cc:80-86) requires a TYPE_OBJ owner, so
// an anonymous owner is E_TYPE.
func TestVerbInfoOwnerMustBeNumberedObject(t *testing.T) {
	ctx, _ := verbMetadataTxnTestContext(t)
	anon := types.NewAnon(0)

	if result := admitTestBuiltin("add_verb", ctx, []types.Value{types.NewObj(0), verbInfoList(anon, "rx", "x"), verbArgsList("this", "none", "this")}); !result.IsError() || result.Error != types.E_TYPE {
		t.Errorf("add_verb with anonymous owner = %v, want E_TYPE", result)
	}
	if result := admitTestBuiltin("set_verb_info", ctx, []types.Value{types.NewObj(0), types.NewStr("look"), verbInfoList(anon, "rx", "look")}); !result.IsError() || result.Error != types.E_TYPE {
		t.Errorf("set_verb_info with anonymous owner = %v, want E_TYPE", result)
	}
}
