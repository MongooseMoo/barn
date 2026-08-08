package scheduler

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
)

func TestEvalBareSuspendTimeoutReturnsInvalidArgument(t *testing.T) {
	store := dbstore.NewStore()
	wizard := dbstore.NewObjectBuilder(0)
	wizard.SetOwner(0)
	wizard.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(wizard.Build()); err != nil {
		t.Fatalf("Add wizard failed: %v", err)
	}

	s := NewScheduler(store)
	defer s.Stop()

	line := s.EvalCommandOutput(0, "suspend();")

	const want = `{2, {E_INVARG, "Invalid argument", 0}}`
	if line != want {
		t.Fatalf("line = %q, want %q", line, want)
	}
}
