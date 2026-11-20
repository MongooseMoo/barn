package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/leakcheck"
	"github.com/MongooseMoo/barn/types"
)

func TestMain(m *testing.M) {
	os.Exit(leakcheck.Main(m.Run))
}

// Keep the same store/runtime alive across real eval, suspension and automatic
// anonymous-object cleanup. Whole-process shutdown alone would hide retention.
func TestMemoryRuntimeLifecycle(t *testing.T) {
	out := os.Getenv("BARN_LEAK_OUTPUT")
	if out == "" {
		t.Skip("set BARN_LEAK_OUTPUT for same-runtime lifecycle replay")
	}
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatal(err)
	}
	rt := newRuntimeWithWorkerCount(store, config.DefaultOptions(), 4)
	defer rt.Stop()
	result, err := leakcheck.Replay(filepath.Join(out, "runtime-lifecycle"), func() {
		got := rt.EvalCommandOutput(0, "x=create(#0,1); x=0; suspend(0); return 1;")
		if got != "{1, 1}" {
			t.Fatalf("lifecycle replay: %s", got)
		}
	}, func() any { return holders(rt, store) })
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("same-runtime memory replay: %s %v", result.Verdict, result.Reasons)
	if result.Verdict != "healthy" {
		t.Fatalf("same-runtime memory replay %s: %v", result.Verdict, result.Reasons)
	}
}
