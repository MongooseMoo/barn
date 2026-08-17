package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func wizardExecution(s *Session) *Execution {
	return s.NewExecution(&kernel.TaskContext{IsWizard: true}, nil)
}

func TestSessionFileHandlesAreIsolatedAndCloseIsScoped(t *testing.T) {
	t.Chdir(t.TempDir())
	s1 := NewSession(NewRegistry(), NoHost())
	s2 := NewSession(NewRegistry(), NoHost())
	c1, c2 := wizardExecution(s1), wizardExecution(s2)

	open := func(ctx *Execution, name string) types.Value {
		t.Helper()
		result := builtinFileOpen(ctx, []types.Value{types.NewStr(name), types.NewStr("w+tn")})
		if result.IsError() {
			t.Fatalf("file_open(%q): %v", name, result.Error)
		}
		return result.Val
	}
	h1, h2 := open(c1, "one"), open(c2, "two")
	if h1.Int() != 1 || h2.Int() != 1 {
		t.Fatalf("first IDs = %d, %d; want independent ID 1", h1.Int(), h2.Int())
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}
	if got := builtinFileName(c1, []types.Value{h1}); got.Error != types.E_INVARG {
		t.Fatalf("closed registry handle error = %v", got.Error)
	}
	if got := builtinFileName(c2, []types.Value{h2}); got.IsError() || got.Val.Str() != "two" {
		t.Fatalf("other registry affected: %#v", got)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := s2.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSessionSQLiteHandlesAreIsolatedAndCloseIsScoped(t *testing.T) {
	s1 := NewSession(NewRegistry(), NoHost())
	s2 := NewSession(NewRegistry(), NoHost())
	c1, c2 := wizardExecution(s1), wizardExecution(s2)
	open := func(ctx *Execution) types.Value {
		t.Helper()
		result := builtinSqliteOpen(ctx, []types.Value{types.NewStr(":memory:")})
		if result.IsError() {
			t.Fatalf("sqlite_open: %v", result.Error)
		}
		return result.Val
	}
	h1, h2 := open(c1), open(c2)
	if h1.Int() != 1 || h2.Int() != 1 {
		t.Fatalf("first IDs = %d, %d; want independent ID 1", h1.Int(), h2.Int())
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}
	if got := builtinSqliteInfo(c1, []types.Value{h1}); got.Error != types.E_INVARG {
		t.Fatalf("closed registry handle error = %v", got.Error)
	}
	if got := builtinSqliteInfo(c2, []types.Value{h2}); got.IsError() {
		t.Fatalf("other registry affected: %#v", got)
	}
	if err := s2.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSessionCachesAndRecycleGuardsAreIsolated(t *testing.T) {
	registry := NewRegistry()
	s1, s2 := NewSession(registry, NoHost()), NewSession(registry, NoHost())

	s1.applyProtectedBuiltins(map[string]bool{"create": true})
	if !s1.IsProtectedBuiltin("create") {
		t.Fatal("first registry did not retain protected flag")
	}
	if s2.IsProtectedBuiltin("create") {
		t.Fatal("protected flag leaked to second registry")
	}

	snapshot := defaultServerOptionsSnapshot()
	snapshot.MaxStringConcat = 2048
	s1.applyServerOptionsSnapshot(&snapshot)
	if got := s1.GetMaxStringConcat(); got != 2048 {
		t.Fatalf("first registry max_string_concat = %d, want 2048", got)
	}
	if got := s2.GetMaxStringConcat(); got != defaultMaxStringConcat {
		t.Fatalf("server-option cache leaked to second registry: %d", got)
	}

	const objectID = types.ObjID(42)
	if !s1.startRecycle(objectID) || !s2.startRecycle(objectID) {
		t.Fatal("independent registries must both acquire the same recycle guard")
	}
	if s1.startRecycle(objectID) {
		t.Fatal("one registry acquired its own recycle guard twice")
	}
	s1.endRecycle(objectID)
	if s2.startRecycle(objectID) {
		t.Fatal("ending one registry's guard affected the other registry")
	}
	s2.endRecycle(objectID)
}

func TestSessionConnectionAndHeldInputStateIsIsolatedAndClosed(t *testing.T) {
	registry := NewRegistry()
	s1, s2 := NewSession(registry, NoHost()), NewSession(registry, NoHost())
	const player = types.ObjID(7)

	s1.setConnectionOption(player, "hold-input", types.NewInt(1))
	s2.setConnectionOption(player, "hold-input", types.NewInt(1))
	if handled, _ := s1.HandleHeldInput(player, "one", false); !handled {
		t.Fatal("first registry did not hold input")
	}
	if handled, _ := s2.HandleHeldInput(player, "two", false); !handled {
		t.Fatal("second registry did not hold input")
	}

	if got := s1.drainHeldCommands(player); len(got) != 1 || got[0] != "one" {
		t.Fatalf("first registry held commands = %q", got)
	}
	if got := s2.drainHeldCommands(player); len(got) != 1 || got[0] != "two" {
		t.Fatalf("second registry held commands = %q", got)
	}
	if got := sessionHeldHTTPInputCount(s1); got != 1 {
		t.Fatalf("first registry HTTP input states = %d, want 1", got)
	}
	if got := sessionHeldHTTPInputCount(s2); got != 1 {
		t.Fatalf("second registry HTTP input states = %d, want 1", got)
	}

	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}
	if s1.ConnectionOptionTruthy(player, "hold-input") {
		t.Fatal("closed registry retained connection options")
	}
	if got := sessionHeldHTTPInputCount(s1); got != 0 {
		t.Fatalf("closed registry retained %d HTTP input states", got)
	}
	if !s2.ConnectionOptionTruthy(player, "hold-input") {
		t.Fatal("closing first registry affected second registry options")
	}
	if got := sessionHeldHTTPInputCount(s2); got != 1 {
		t.Fatalf("closing first registry affected second registry HTTP state: %d", got)
	}
	if err := s2.Close(); err != nil {
		t.Fatal(err)
	}
}

func sessionHeldHTTPInputCount(s *Session) int {
	s.runtime.heldHTTPInput.mu.Lock()
	defer s.runtime.heldHTTPInput.mu.Unlock()
	return len(s.runtime.heldHTTPInput.byPlayer)
}
