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
	_ = r2.Close()
}
