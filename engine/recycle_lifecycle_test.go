package engine

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestNumberedRecycleStaysTransactional(t *testing.T) {
	for _, hooks := range []bool{false, true} {
		t.Run(fmt.Sprintf("hooks=%t", hooks), func(t *testing.T) {
			store := protectedRedirectStore(t)
			for _, name := range []string{"recycles", "exits"} {
				if ec := store.DirectTxn().DefineProperty(0, name, dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); ec != types.E_NONE {
					t.Fatal(ec)
				}
			}
			room, ec := store.DirectTxn().CreateObject([]types.ObjID{1}, 2)
			if ec != types.E_NONE {
				t.Fatal(ec)
			}
			addVerb := func(id types.ObjID, name, body string) {
				t.Helper()
				if _, ec := store.AddVerb(id, dbstore.NewVerb([]string{name}, 2,
					dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
					dbstore.VerbArgs{This: "this", Prep: "none", That: "none"}, []string{body})); ec != types.E_NONE {
					t.Fatal(ec)
				}
			}
			if hooks {
				addVerb(1, "recycle", "#0.recycles = #0.recycles + 1;")
				addVerb(1, "exitfunc", "#0.exits = #0.exits + 1;")
			}
			addVerb(0, "recycle_probe", fmt.Sprintf(
				"o = create(#1); move(o, #%d); c = create(#1); move(c, o); recycle(o); "+
					"n = create(#1); recycle(n); return {!valid(o), !valid(n), valid(c) && c.location == #-1, length(#%d.contents) == 0, #0.recycles, #0.exits};", room, room))
			rt := NewRuntime(store)
			defer rt.Stop()
			result, err := rt.RunServerVerbTask(0, "recycle_probe", nil, 2)
			calls := int64(0)
			if hooks {
				calls = 2
			}
			want := types.NewList([]types.Value{types.NewInt(1), types.NewInt(1), types.NewInt(1), types.NewInt(1), types.NewInt(calls), types.NewInt(calls)})
			if err != nil || result.Flow != types.FlowReturn || !result.Val.Equal(want) {
				t.Fatalf("recycle result=%+v error=%v, want %s", result, err, want)
			}
			if holds := rt.GateHolds(); len(holds) != 0 {
				t.Fatalf("ordinary numbered recycle acquired the exclusive commit gate: %+v", holds)
			}
		})
	}
}

func TestNumberedRecycleSelfRecursionReturnsInvarg(t *testing.T) {
	store := protectedRedirectStore(t)
	code := `o = create(#-1); add_verb(o, {player, "rxd", "recycle"}, {"this", "none", "this"}); ` +
		`set_verb_code(o, "recycle", {"recycle(this);"}); return recycle(o);`
	if _, ec := store.AddVerb(0, dbstore.NewVerb([]string{"self_recycle"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "none"}, []string{code})); ec != types.E_NONE {
		t.Fatal(ec)
	}
	rt := NewRuntime(store)
	defer rt.Stop()
	result, err := rt.RunServerVerbTask(0, "self_recycle", nil, 2)
	if err != nil || result.Flow != types.FlowException || result.Error != types.E_INVARG {
		t.Fatalf("self recycle result=%+v error=%v, want native E_INVARG", result, err)
	}
}

func TestRecycleThenQueryAndRecreateStagedObject(t *testing.T) {
	rt := NewRuntime(protectedRedirectStore(t))
	defer rt.Stop()
	got := rt.EvalCommandOutput(2, "o = create(#-1); recycle(o); next_recycled_object(o); n = recreate(o, #-1); return {n == o, valid(n)};")
	if got != "{1, {1, 1}}" {
		t.Fatalf("recycle/query/recreate = %s, want {1, {1, 1}}", got)
	}
}

func TestSequentialRecycleAfterCoarseMovesCompletes(t *testing.T) {
	store, _ := buildConcurrencyStore(t, 0, 0)
	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()
	code := "a = create(#-1); b = create(#-1); c = create(#-1); move(b, a); move(c, b); " +
		"r1 = `recycle(c) ! ANY'; r2 = `recycle(b) ! ANY'; r3 = `recycle(a) ! ANY'; return {r1, r2, r3};"
	tk := s.buildBenchTask(t, 99992, code)
	if err := s.runTask(tk); err != nil {
		t.Fatal(err)
	}
	want := types.NewList([]types.Value{types.NewInt(0), types.NewInt(0), types.NewInt(0)})
	if tk.Result.Flow != types.FlowReturn || !tk.Result.Val.Equal(want) {
		t.Fatalf("sequential recycle = flow %v, value %v; want %v", tk.Result.Flow, tk.Result.Val, want)
	}
}

func TestRecycleParentAfterAnonymousCycleCollectionCompletes(t *testing.T) {
	database, err := dbformat.LoadDatabase(filepath.Join("..", "Test_conf.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}
	store, _ := database.NewStoreFromDatabase()
	code := `o = p = #-1; try ` +
		`o = create(#-1); add_property(o, "next", 0, {#3, ""}); ` +
		`p = create(o, 1); p.next = p; add_property(o, "foo", 0, {#3, ""}); ` +
		`run_gc(); recycle(o); run_gc(); return 1; ` +
		`finally if (valid(p)) recycle(p); endif if (valid(o)) recycle(o); endif endtry`
	if _, errCode := store.AddVerb(3, dbstore.NewVerb(
		[]string{"anonymous-cycle-probe"}, 3,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "none"}, []string{code},
	)); errCode != types.E_NONE {
		t.Fatalf("add probe verb: %v", errCode)
	}
	s := NewRuntime(store)
	defer s.Stop()
	result, err := s.RunServerVerbTask(3, "anonymous-cycle-probe", nil, 3)
	if err != nil {
		t.Fatalf("run probe verb: %v", err)
	}
	if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != 1 {
		t.Fatalf("anonymous-cycle parent recycle = flow %v value %v error %v, want return 1", result.Flow, result.Val, result.Error)
	}
}
