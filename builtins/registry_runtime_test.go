package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func wizardExecution(r *Registry) *Execution {
	return r.NewExecution(&kernel.TaskContext{IsWizard: true}, nil)
}

func TestRegistryFileHandlesAreIsolatedAndCloseIsScoped(t *testing.T) {
	t.Chdir(t.TempDir())
	r1, r2 := NewRegistry(), NewRegistry()
	c1, c2 := wizardExecution(r1), wizardExecution(r2)

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
	if err := r1.Close(); err != nil {
		t.Fatal(err)
	}
	if got := builtinFileName(c1, []types.Value{h1}); got.Error != types.E_INVARG {
		t.Fatalf("closed registry handle error = %v", got.Error)
	}
	if got := builtinFileName(c2, []types.Value{h2}); got.IsError() || got.Val.Str() != "two" {
		t.Fatalf("other registry affected: %#v", got)
	}
	if err := r1.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := r2.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistrySQLiteHandlesAreIsolatedAndCloseIsScoped(t *testing.T) {
	r1, r2 := NewRegistry(), NewRegistry()
	c1, c2 := wizardExecution(r1), wizardExecution(r2)
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
	if err := r1.Close(); err != nil {
		t.Fatal(err)
	}
	if got := builtinSqliteInfo(c1, []types.Value{h1}); got.Error != types.E_INVARG {
		t.Fatalf("closed registry handle error = %v", got.Error)
	}
	if got := builtinSqliteInfo(c2, []types.Value{h2}); got.IsError() {
		t.Fatalf("other registry affected: %#v", got)
	}
	if err := r2.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryCachesAndRecycleGuardsAreIsolated(t *testing.T) {
	r1, r2 := NewRegistry(), NewRegistry()

	r1.applyProtectedBuiltins(map[string]bool{"create": true})
	if !r1.IsProtectedBuiltin("create") {
		t.Fatal("first registry did not retain protected flag")
	}
	if r2.IsProtectedBuiltin("create") {
		t.Fatal("protected flag leaked to second registry")
	}

	snapshot := defaultServerOptionsSnapshot()
	snapshot.MaxStringConcat = 2048
	r1.applyServerOptionsSnapshot(&snapshot)
	if got := r1.GetMaxStringConcat(); got != 2048 {
		t.Fatalf("first registry max_string_concat = %d, want 2048", got)
	}
	if got := r2.GetMaxStringConcat(); got != defaultMaxStringConcat {
		t.Fatalf("server-option cache leaked to second registry: %d", got)
	}

	const objectID = types.ObjID(42)
	if !r1.startRecycle(objectID) || !r2.startRecycle(objectID) {
		t.Fatal("independent registries must both acquire the same recycle guard")
	}
	if r1.startRecycle(objectID) {
		t.Fatal("one registry acquired its own recycle guard twice")
	}
	r1.endRecycle(objectID)
	if r2.startRecycle(objectID) {
		t.Fatal("ending one registry's guard affected the other registry")
	}
	r2.endRecycle(objectID)
}

func TestRegistryConnectionAndHeldInputStateIsIsolatedAndClosed(t *testing.T) {
	r1, r2 := NewRegistry(), NewRegistry()
	const player = types.ObjID(7)

	r1.setConnectionOption(player, "hold-input", types.NewInt(1))
	r2.setConnectionOption(player, "hold-input", types.NewInt(1))
	if handled, _ := r1.HandleHeldInput(player, "one", false); !handled {
		t.Fatal("first registry did not hold input")
	}
	if handled, _ := r2.HandleHeldInput(player, "two", false); !handled {
		t.Fatal("second registry did not hold input")
	}

	if got := r1.drainHeldCommands(player); len(got) != 1 || got[0] != "one" {
		t.Fatalf("first registry held commands = %q", got)
	}
	if got := r2.drainHeldCommands(player); len(got) != 1 || got[0] != "two" {
		t.Fatalf("second registry held commands = %q", got)
	}
	if got := registryHeldHTTPInputCount(r1); got != 1 {
		t.Fatalf("first registry HTTP input states = %d, want 1", got)
	}
	if got := registryHeldHTTPInputCount(r2); got != 1 {
		t.Fatalf("second registry HTTP input states = %d, want 1", got)
	}

	if err := r1.Close(); err != nil {
		t.Fatal(err)
	}
	if r1.ConnectionOptionTruthy(player, "hold-input") {
		t.Fatal("closed registry retained connection options")
	}
	if got := registryHeldHTTPInputCount(r1); got != 0 {
		t.Fatalf("closed registry retained %d HTTP input states", got)
	}
	if !r2.ConnectionOptionTruthy(player, "hold-input") {
		t.Fatal("closing first registry affected second registry options")
	}
	if got := registryHeldHTTPInputCount(r2); got != 1 {
		t.Fatalf("closing first registry affected second registry HTTP state: %d", got)
	}
	if err := r2.Close(); err != nil {
		t.Fatal(err)
	}
}

func registryHeldHTTPInputCount(r *Registry) int {
	r.runtime.heldHTTPInput.mu.Lock()
	defer r.runtime.heldHTTPInput.mu.Unlock()
	return len(r.runtime.heldHTTPInput.byPlayer)
}
