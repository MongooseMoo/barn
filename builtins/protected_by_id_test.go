package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// The dispatch hot path answers "is this builtin protected?" from a by-ID
// projection of the by-name snapshot. Both views must agree, including for a
// builtin registered after the snapshot was taken (outside byID's range).
func TestProtectedByIDMatchesByName(t *testing.T) {
	registry := NewRegistry()
	s := NewSession(registry, NoHost())

	s.applyProtectedBuiltins(map[string]bool{"create": true, "no_such_builtin": true})

	for _, e := range registry.entries {
		if got, want := s.isProtectedEntry(e), s.IsProtectedBuiltin(e.name); got != want {
			t.Fatalf("%s: isProtectedEntry=%v IsProtectedBuiltin=%v", e.name, got, want)
		}
	}
	id, ok := registry.GetID("create")
	if !ok {
		t.Fatal("create not registered")
	}
	if !s.isProtectedEntry(registry.entries[id]) {
		t.Fatal("create should be protected by ID")
	}

	// Late registration: byID does not cover it; byName must still answer.
	registry.Register("late_test_builtin", func(*Execution, []types.Value) types.Result {
		return types.Ok(types.NewInt(1))
	})
	lateID, _ := registry.GetID("late_test_builtin")
	late := registry.entries[lateID]
	if s.isProtectedEntry(late) {
		t.Fatal("late builtin should not be protected yet")
	}
	s.applyProtectedBuiltins(map[string]bool{"late_test_builtin": true})
	if !s.isProtectedEntry(late) || !s.IsProtectedBuiltin("late_test_builtin") {
		t.Fatal("late builtin should be protected after refresh")
	}

	// Clearing the set clears both views.
	s.applyProtectedBuiltins(nil)
	if s.isProtectedEntry(late) || s.isProtectedEntry(registry.entries[id]) {
		t.Fatal("protected flags survived a nil refresh")
	}
}
