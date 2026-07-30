package scheduler

import (
	"testing"

	dbstore "barn/db/store"
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

	lines := s.EvalCommandOutput(0, "suspend();", "", "")

	const want = `{2, {E_INVARG, "Invalid argument", 0}}`
	if len(lines) != 1 || lines[0] != want {
		t.Fatalf("lines = %#v, want %q", lines, want)
	}
}
