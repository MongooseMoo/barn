package engine

import (
	"path/filepath"
	"testing"

	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

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
		"anonymous-cycle-probe", []string{"anonymous-cycle-probe"}, 3,
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
