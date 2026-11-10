package vm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// maphaskeyReader tests one entry of this.state and writes its answer to
// another entry, so its commit validates what it read.
const maphaskeyReader = "r = maphaskey(this.state, #7); this.state[#8] = r;"

// maphaskey(this.state, key) depends on the one entry it tests, as
// this.state[key] does: a commit that changes another entry leaves the reader
// valid, and one that inserts or deletes the tested key makes it lose.
func TestMaphaskeyOfPropertyDependsOnTheOneEntry(t *testing.T) {
	for _, test := range []struct {
		name    string
		present []types.ObjID
		other   string
		lose    bool
		want    int64 // the reader's r, published at #8 when it commits
	}{
		{name: "other key inserted", other: "this.state[#6] = 1;"},
		{name: "other key changed", present: []types.ObjID{6}, other: "this.state[#6] = 2;"},
		{name: "other key changed, tested key present", present: []types.ObjID{6, 7}, other: "this.state[#6] = 2;", want: 1},
		{name: "tested key inserted", other: "this.state[#7] = 1;", lose: true},
		{name: "tested key deleted", present: []types.ObjID{7}, other: "this.state = mapdelete(this.state, #7);", lose: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := playerStateStore(t, test.present...)
			reader, other := store.BeginSnapshot(0), store.BeginSnapshot(0)
			defer reader.Release()
			defer other.Release()
			runOnThis(t, store, reader, 0, maphaskeyReader)
			runOnThis(t, store, other, 0, test.other)
			if ec := other.Commit(); ec != types.E_NONE {
				t.Fatalf("other commit: %v %v", ec, other.Conflicts())
			}
			ec := reader.Commit()
			if test.lose {
				if ec == types.E_NONE || !reader.ValidationFailed() {
					t.Fatalf("reader commit = %v; want a lost validation", ec)
				}
				return
			}
			if ec != types.E_NONE {
				t.Fatalf("reader commit: %v %v", ec, reader.Conflicts())
			}
			if v, ok := playerState(t, store, 8); !ok || v != test.want {
				t.Fatalf("state[#8] = %d (present %v); want %d", v, ok, test.want)
			}
		})
	}
}

// The entry maphaskey tested is validated and carried per entry across the
// irreversible-effect boundary (CommitAndRenewCarryingReads): commits to other
// entries on either side of it leave the reader valid, and an insertion of the
// tested key loses the boundary or the final commit.
func TestMaphaskeyOfPropertyAcrossTheBoundary(t *testing.T) {
	for _, test := range []struct {
		name, before, after string
		loseAtBoundary      bool
		lose                bool
	}{
		{name: "other keys", before: "this.state[#6] = 1;", after: "this.state[#9] = 1;"},
		{name: "tested key inserted before", before: "this.state[#7] = 1;", loseAtBoundary: true},
		{name: "tested key inserted after", after: "this.state[#7] = 1;", lose: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := playerStateStore(t)
			commit := func(code string) {
				t.Helper()
				if code == "" {
					return
				}
				txn := store.BeginSnapshot(0)
				defer txn.Release()
				runOnThis(t, store, txn, 0, code)
				if ec := txn.Commit(); ec != types.E_NONE {
					t.Fatalf("%q commit: %v %v", code, ec, txn.Conflicts())
				}
			}
			reader := store.BeginSnapshot(0)
			defer reader.Release()
			runOnThis(t, store, reader, 0, maphaskeyReader)
			commit(test.before)
			next, _, ec := reader.CommitAndRenewCarryingReads()
			if test.loseAtBoundary {
				if ec == types.E_NONE || !reader.ValidationFailed() {
					t.Fatalf("boundary = %v; want a lost validation", ec)
				}
				return
			}
			if ec != types.E_NONE {
				t.Fatalf("boundary = %v, conflicts %v", ec, reader.Conflicts())
			}
			defer next.Release()
			commit(test.after)
			// The rest of the task writes after the boundary, so its final
			// commit validates the reads carried across it.
			runOnThis(t, store, next, 0, "this.state[#10] = 1;")
			ec = next.Commit()
			if test.lose {
				if ec == types.E_NONE || !next.ValidationFailed() {
					t.Fatalf("final commit = %v; want a lost validation", ec)
				}
				return
			}
			if ec != types.E_NONE {
				t.Fatalf("final commit: %v %v", ec, next.Conflicts())
			}
			for who, want := range map[types.ObjID]int64{6: 1, 8: 0, 9: 1, 10: 1} {
				if v, ok := playerState(t, store, who); !ok || v != want {
					t.Fatalf("state[#%d] = %d (present %v); want %d", who, v, ok, want)
				}
			}
		})
	}
}

// A maphaskey call the task would hand to #0:bf_maphaskey passes the whole map,
// so it reads the whole property: a commit to any entry makes it lose. That
// holds even when, as here, no wrapper exists and the wizard caller runs the
// builtin itself.
func TestProtectedMaphaskeyOfPropertyReadsTheWholeMap(t *testing.T) {
	store := playerStateStore(t)
	reader, other := store.BeginSnapshot(0), store.BeginSnapshot(0)
	defer reader.Release()
	defer other.Release()
	runOnThisWith(t, store, reader, 0, maphaskeyReader, func(ctx *task.Context) {
		ctx.IsWizard = true
		ctx.ThisObj = 1
		ctx.ServerOptions = &task.PendingServerOptions{ProtectedBuiltins: map[string]bool{"maphaskey": true}}
	})
	runOnThis(t, store, other, 0, "this.state[#6] = 1;")
	if ec := other.Commit(); ec != types.E_NONE {
		t.Fatalf("other commit: %v %v", ec, other.Conflicts())
	}
	if ec := reader.Commit(); ec == types.E_NONE || !reader.ValidationFailed() {
		t.Fatalf("reader commit = %v; want a lost validation", ec)
	}
}

// maphaskey's lookup of a boolean key depends on the other keys:
// [true -> 1, false -> 2, 1 -> 3] has m[true], yet maphaskey(m, true) is 0, and
// it is 1 once the key 1 is gone. So that call reads the whole map: a commit
// that removes another key makes it lose.
func TestMaphaskeyOfPropertyWithAnUnorderedKeyReadsTheWholeMap(t *testing.T) {
	store := mapKeyVMStore(t)
	setup := store.BeginSnapshot(0)
	runOn(t, store, setup, "#0.m = [true -> 1, false -> 2, 1 -> 3];")
	if ec := setup.Commit(); ec != types.E_NONE {
		t.Fatalf("setup commit: %v %v", ec, setup.Conflicts())
	}
	setup.Release()
	reader, other := store.BeginSnapshot(0), store.BeginSnapshot(0)
	defer reader.Release()
	defer other.Release()
	runOn(t, store, reader, `r = maphaskey(#0.m, true); if (r != 0) raise(E_INVARG); endif #0.m["r"] = r;`)
	runOn(t, store, other, "#0.m = mapdelete(#0.m, 1);")
	if ec := other.Commit(); ec != types.E_NONE {
		t.Fatalf("other commit: %v %v", ec, other.Conflicts())
	}
	if ec := reader.Commit(); ec == types.E_NONE || !reader.ValidationFailed() {
		t.Fatalf("reader commit = %v; want a lost validation", ec)
	}
}

// maphaskeyEquivalenceStore gives #0 the properties the equivalence cases read.
func maphaskeyEquivalenceStore(t *testing.T) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	system := dbstore.NewObjectBuilder(0)
	system.SetFlags(dbstore.FlagRead | dbstore.FlagWrite)
	rw := dbstore.PropRead | dbstore.PropWrite
	m := types.NewMap([][2]types.Value{
		{types.NewStr("a"), types.NewInt(1)},
		{types.NewObj(5), types.NewInt(2)},
		{types.NewInt(3), types.NewInt(4)},
		{types.NewFloat(1.5), types.NewInt(5)},
		{types.NewErr(types.E_PERM), types.NewInt(6)},
	})
	system.SetProperty("m", dbstore.NewProperty(m, 0, rw, false, true))
	bools := types.NewMap([][2]types.Value{
		{types.NewBool(true), types.NewInt(1)},
		{types.NewBool(false), types.NewInt(2)},
		{types.NewInt(1), types.NewInt(3)},
	})
	system.SetProperty("bools", dbstore.NewProperty(bools, 0, rw, false, true))
	system.SetProperty("empty", dbstore.NewProperty(types.NewMap(nil), 0, rw, false, true))
	system.SetProperty("notmap", dbstore.NewProperty(types.NewInt(5), 0, rw, false, true))
	system.SetProperty("lst", dbstore.NewProperty(types.NewList([]types.Value{types.NewInt(1), types.NewInt(2)}), 0, rw, false, true))
	system.SetProperty("secret", dbstore.NewProperty(types.NewMap([][2]types.Value{{types.NewStr("a"), types.NewInt(1)}}), 0, 0, false, true))
	system.SetProperty("self", dbstore.NewProperty(types.NewObj(0), 0, rw, false, true))
	if err := store.Add(system.Build()); err != nil {
		t.Fatalf("store.Add: %v", err)
	}
	return store
}

// The keyed form returns exactly what the builtin returns on the same value:
// each case runs maphaskey(obj.prop, ...) and maphaskey(obj.("prop"), ...),
// whose dynamic name reads the whole property through the unchanged path, with
// the same evaluation order (object, property, key, call).
func TestMaphaskeyOfPropertyMatchesTheBuiltin(t *testing.T) {
	type caller int
	const (
		owner caller = iota
		stranger
		protectedWizard
		protectedStranger
	)
	for _, test := range []struct {
		obj, prop, rest string
		as              caller
	}{
		{obj: "#0", prop: "m", rest: `"a"`},
		{obj: "#0", prop: "m", rest: `"A"`},
		{obj: "#0", prop: "m", rest: `"z"`},
		{obj: "#0", prop: "m", rest: `#5`},
		{obj: "#0", prop: "m", rest: `#6`},
		{obj: "#0", prop: "m", rest: `3`},
		{obj: "#0", prop: "m", rest: `3.0`},
		{obj: "#0", prop: "m", rest: `1.5`},
		{obj: "#0", prop: "m", rest: `E_PERM`},
		{obj: "#0", prop: "m", rest: `E_TYPE`},
		{obj: "#0", prop: "m", rest: `{1}`},
		{obj: "#0", prop: "m", rest: `[1 -> 2]`},
		{obj: "#0", prop: "m", rest: `1/0`},
		{obj: "#0", prop: "m", rest: `"A", 1`},
		{obj: "#0", prop: "m", rest: `"a", 1`},
		{obj: "#0", prop: "m", rest: `"a", "x"`},
		{obj: "#0", prop: "bools", rest: `true`},
		{obj: "#0", prop: "bools", rest: `false`},
		{obj: "#0", prop: "bools", rest: `1`},
		{obj: "#0", prop: "empty", rest: `"a"`},
		{obj: "#0", prop: "notmap", rest: `"a"`},
		{obj: "#0", prop: "lst", rest: `1`},
		{obj: "#0", prop: "name", rest: `"a"`},
		{obj: "#0", prop: "missing", rest: `"a"`},
		{obj: "#0", prop: "missing", rest: `1/0`},
		{obj: "#-5", prop: "m", rest: `"a"`},
		{obj: "#99", prop: "m", rest: `"a"`},
		{obj: "(5)", prop: "m", rest: `"a"`},
		{obj: `"str"`, prop: "m", rest: `"a"`},
		{obj: "$self", prop: "m", rest: `"a"`},
		{obj: "$self", prop: "m", rest: `"q"`},
		{obj: "#0", prop: "secret", rest: `"a"`},
		{obj: "#0", prop: "secret", rest: `"a"`, as: stranger},
		{obj: "#0", prop: "m", rest: `"a"`, as: stranger},
		{obj: "#0", prop: "m", rest: `"a"`, as: protectedWizard},
		{obj: "#0", prop: "m", rest: `"z"`, as: protectedWizard},
		{obj: "#0", prop: "m", rest: `"a"`, as: protectedStranger},
	} {
		keyedSource := fmt.Sprintf("return maphaskey(%s.%s, %s);", test.obj, test.prop, test.rest)
		plainSource := fmt.Sprintf("return maphaskey(%s.(%q), %s);", test.obj, test.prop, test.rest)
		t.Run(fmt.Sprintf("%s/as%d", keyedSource, test.as), func(t *testing.T) {
			run := func(code string) (types.Result, bool) {
				registry := BuildVMRegistry()
				prog, diagnostics := registry.Compiler().CompileMOO([]string{code})
				if len(diagnostics) > 0 {
					t.Fatalf("CompileMOO(%q): %v", code, diagnostics[0])
				}
				keyed := strings.Contains(strings.Join(bytecode.Disassemble(prog), "\n"), " MAPHASKEY_KEYED")
				store := maphaskeyEquivalenceStore(t)
				txn := store.BeginSnapshot(0)
				defer txn.Release()
				ctx := task.NewContext()
				ctx.StoreTxn = txn
				ctx.ThisObj = 1
				switch test.as {
				case owner:
					ctx.Programmer = 0
				case stranger:
					ctx.Programmer = 1
				case protectedWizard, protectedStranger:
					ctx.Programmer = 1
					ctx.IsWizard = test.as == protectedWizard
					ctx.ServerOptions = &task.PendingServerOptions{ProtectedBuiltins: map[string]bool{"maphaskey": true}}
				}
				return runCompiledBytecodeProgram(prog, registry, store, ctx), keyed
			}
			got, keyed := run(keyedSource)
			want, plainKeyed := run(plainSource)
			if plainKeyed {
				t.Fatalf("%q compiled keyed", plainSource)
			}
			if wantKeyed := !strings.Contains(test.rest, ","); keyed != wantKeyed {
				t.Fatalf("%q compiled keyed=%v; want %v", keyedSource, keyed, wantKeyed)
			}
			if got.Flow != want.Flow || got.Error != want.Error || !got.Val.Identical(want.Val) {
				t.Fatalf("keyed %q = {%v %v %v}; builtin %q = {%v %v %v}", keyedSource, got.Flow, got.Error, got.Val, plainSource, want.Flow, want.Error, want.Val)
			}
		})
	}
}
