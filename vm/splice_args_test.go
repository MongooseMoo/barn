package vm

import (
	"fmt"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// spliceStore has #0 with verbs that return or rewrite their args, and #1, a
// child of #0, whose relay verbs pass() to #0's.
func spliceStore(t *testing.T) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagProgrammer)
	root.SetChildren([]types.ObjID{1})
	store.Add(root.Build())
	child := dbstore.NewObjectBuilder(1)
	child.SetOwner(0)
	child.SetParents([]types.ObjID{0})
	store.Add(child.Build())

	perms := dbstore.VerbRead | dbstore.VerbWrite | dbstore.VerbExecute | dbstore.VerbDebug
	for _, verb := range []struct {
		on   types.ObjID
		name string
		code string
	}{
		{0, "echo", "return args;"},
		{0, "mutate", "args[1] = 99; args = {@args, 100}; return args;"},
		{0, "relay", "return args;"},
		{1, "relay", "return pass(@args);"},
		{1, "relay_bare", "return pass();"},
		{0, "relay_bare", "return args;"},
	} {
		if _, errCode := store.AddVerb(verb.on, dbstore.NewVerb([]string{verb.name}, 0, perms, dbstore.VerbArgs{}, []string{verb.code})); errCode != types.E_NONE {
			t.Fatalf("AddVerb(#%d:%s) = %v", verb.on, verb.name, errCode)
		}
	}
	return store
}

// A spliced call hands the callee the caller's list. Whatever the callee does
// to its args, and whatever the caller appends afterwards, neither sees the
// other's change.
func TestSplicedCallsLeaveTheCallersListUnchanged(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{
			name: "verb call",
			code: `l = {1, 2, 3}; a = #0:mutate(@l); b = #0:echo(@l); return {l, a, b};`,
			want: `{{1, 2, 3}, {99, 2, 3, 100}, {1, 2, 3}}`,
		},
		{
			name: "verb call on an appended list",
			code: `l = {}; for i in [1..3] l = {@l, i}; endfor a = #0:mutate(@l); m = {@l, "m"}; return {l, a, m};`,
			want: `{{1, 2, 3}, {99, 2, 3, 100}, {1, 2, 3, "m"}}`,
		},
		{
			name: "pass",
			code: `l = {"a", "b"}; a = #1:relay(@l); l[1] = "z"; return {l, a, #1:relay_bare(@a)};`,
			want: `{{"z", "b"}, {"a", "b"}, {"a", "b"}}`,
		},
		{
			name: "builtin",
			code: `l = {{1}, 2}; x = listappend(@l); return {l, x};`,
			want: `{{{1}, 2}, {1, 2}}`,
		},
		{
			name: "empty lists stay empty",
			code: `a = {}; a = {@a, 1}; b = {}; b = listappend(b, 2); c = {}; c[1..0] = {3}; d = {}; return {a, b, c, d, length(d)};`,
			want: `{{1}, {2}, {3}, {}, 0}`,
		},
		{
			name: "two appends from one base",
			code: `l = {1}; for i in [2..4] l = {@l, i}; endfor m = {@l, "x"}; n = {@l, "y"}; e = {@{}, @l}; return {l, m, n, e};`,
			want: `{{1, 2, 3, 4}, {1, 2, 3, 4, "x"}, {1, 2, 3, 4, "y"}, {1, 2, 3, 4}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := runBytecodeProgram(t, tc.code, spliceStore(t), nil)
			if result.Flow != types.FlowReturn || result.Val.String() != tc.want {
				t.Fatalf("result = flow %v value %v error %v, want %s", result.Flow, result.Val, result.Error, tc.want)
			}
		})
	}
}

// loopAllocsPerIteration runs body in a loop of n and 2n iterations and
// returns the extra allocations per iteration, which excludes VM setup.
func loopAllocsPerIteration(t *testing.T, store *dbstore.Store, setup, body string) float64 {
	t.Helper()
	registry := BuildVMRegistry()
	session := newTestSession(registry)
	run := func(n int) float64 {
		program, diagnostics := registry.Compiler().CompileMOO([]string{
			fmt.Sprintf("%s for i in [1..%d] %s endfor return 1;", setup, n, body),
		})
		if len(diagnostics) != 0 {
			t.Fatal(diagnostics)
		}
		return testing.AllocsPerRun(5, func() {
			ctx := task.NewContext()
			ctx.Store = store
			ctx.TicksRemaining = 1 << 60
			machine := NewVM(store, session)
			machine.Context = ctx
			machine.TickLimit = 1 << 60
			machine.Task = task.NewState(1, 0, ctx.TicksRemaining, 1)
			if result := machine.Run(program); result.Flow != types.FlowReturn {
				t.Fatalf("result = flow %v value %v error %v", result.Flow, result.Val, result.Error)
			}
		})
	}
	const n = 1000
	return (run(2*n) - run(n)) / n
}

// A spliced call costs no allocation beyond the same call with no arguments;
// building {@l} or entering a one-code try costs none at all.
func TestSplicedCallsAndOneCodeTryDoNotAllocate(t *testing.T) {
	store := spliceStore(t)
	setup := `l = {1, 2}; k = {1};`
	perIteration := func(body string) float64 {
		return loopAllocsPerIteration(t, store, setup, body)
	}

	for _, tc := range []struct{ name, spliced, bare string }{
		{"verb call", "x = #0:echo(@l);", "x = #0:echo();"},
		{"pass", "x = #1:relay(@l);", "x = #1:relay();"},
		{"builtin", "x = typeof(@k);", "x = typeof(k);"},
	} {
		spliced, bare := perIteration(tc.spliced), perIteration(tc.bare)
		t.Logf("%s: spliced %.2f, bare %.2f allocations per call", tc.name, spliced, bare)
		if spliced > bare {
			t.Errorf("%s: spliced call allocates %.2f per call, more than the same call without the splice (%.2f)", tc.name, spliced, bare)
		}
	}
	for _, tc := range []struct{ name, body string }{
		{"empty list extended", "x = {@l};"},
		{"list extended by empty", "x = {@l, @{}};"},
		{"one-code try", "try x = 1; except (E_TYPE) x = 2; endtry"},
	} {
		if got := perIteration(tc.body); got != 0 {
			t.Errorf("%s: %.2f allocations per iteration, want 0", tc.name, got)
		}
	}
}
