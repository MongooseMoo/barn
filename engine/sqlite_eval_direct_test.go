package engine

import (
	"testing"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
)

// A threaded sqlite statement must launch at the eval's suspension boundary
// and deliver its result through the same scheduler as ordinary task resumes.
func TestEvalThreadedSqliteStatementCompletesAfterHandoff(t *testing.T) {
	store := dbstore.NewStore()
	wizard := dbstore.NewObjectBuilder(0)
	wizard.SetOwner(0)
	wizard.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(wizard.Build()); err != nil {
		t.Fatalf("Add wizard: %v", err)
	}
	rt := NewRuntime(store)
	defer rt.Stop()

	done := make(chan string, 1)
	go func() {
		done <- rt.EvalCommandOutput(0, `h = sqlite_open(":memory:"); r = sqlite_query(h, "SELECT 1"); sqlite_close(h); return r;`)
	}()
	select {
	case line := <-done:
		if line != "{1, {{1}}}" {
			t.Fatalf("eval result = %q, want {1, {{1}}}", line)
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("eval with a threaded sqlite statement did not complete")
	}
}
