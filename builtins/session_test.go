package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func TestRegistryDispatchCanBeSharedWithoutSharingSessionState(t *testing.T) {
	registry := NewRegistry()
	first := NewSession(registry, NoHost())
	second := NewSession(registry, NoHost())

	if first.Registry() != registry || second.Registry() != registry {
		t.Fatal("sessions must retain the shared dispatch registry")
	}

	first.setConnectionOption(7, "binary", types.NewInt(1))
	if !first.ConnectionOptionTruthy(7, "binary") {
		t.Fatal("first session did not retain its connection option")
	}
	if second.ConnectionOptionTruthy(7, "binary") {
		t.Fatal("connection options leaked between sessions")
	}

	first.applyServerOptionsSnapshot(&kernel.PendingServerOptions{MaxStringConcat: 17})
	if got := first.GetMaxStringConcat(); got != 17 {
		t.Fatalf("first session max string concat = %d, want 17", got)
	}
	if got := second.GetMaxStringConcat(); got == 17 {
		t.Fatal("server options leaked between sessions")
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close first session: %v", err)
	}
	if _, ok := registry.GetID("tostr"); !ok {
		t.Fatal("closing a session mutated the shared dispatch registry")
	}
}

func TestExecutionCarriesRegistryAndSession(t *testing.T) {
	registry := NewRegistry()
	session := NewSession(registry, NoHost())
	execution := session.NewExecution(kernel.NewTaskContext(), nil)

	if execution.Registry != registry {
		t.Fatal("execution does not carry its dispatch registry")
	}
	if execution.Session != session {
		t.Fatal("execution does not carry its mutable session")
	}
}
