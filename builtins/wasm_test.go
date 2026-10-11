package builtins

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func wasmTestSession(t *testing.T) *Session {
	t.Helper()
	dir := t.TempDir()
	data, err := os.ReadFile("../internal/wasmbuiltins/testdata/example.wasm")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "example.wasm"), data, 0600); err != nil {
		t.Fatal(err)
	}
	s := NewSession(NewRegistry(), NoHost())
	t.Cleanup(func() { _ = s.Close() })
	if err = s.ConfigureWASM(dir); err != nil {
		t.Fatal(err)
	}
	return s
}
func wasmTestCall(t *testing.T, s *Session, ctx *Execution, name string, args ...types.Value) types.Result {
	t.Helper()
	r, ok := s.CallByNameWithExecution(name, ctx, args)
	if !ok {
		t.Fatalf("missing builtin %s", name)
	}
	return r
}
func TestWASMBuiltinAuthorityAndEffects(t *testing.T) {
	s := wasmTestSession(t)
	ctx := newTestExecutionForSession(s)
	for _, name := range []string{"wasm_load", "wasm_unload"} {
		r := wasmTestCall(t, s, ctx, name, types.NewStr("example"))
		if r.Error != types.E_PERM || ctx.IrreversibleSideEffect {
			t.Fatalf("permission/effect %s: %+v", name, r)
		}
	}
	r := wasmTestCall(t, s, ctx, "wasm_call", types.NewStr("example.add"), types.NewInt(20), types.NewInt(22))
	if r.IsError() || r.Val.Int() != 42 || ctx.IrreversibleSideEffect {
		t.Fatalf("pure call: %+v", r)
	}
	if ctx.BuiltinTicksConsumed != 1000 {
		t.Fatalf("uncharged guest work: %d", ctx.BuiltinTicksConsumed)
	}
	ctx.IsWizard = true
	ctx.BeforeIrreversibleEffect = func() bool { return true }
	r = wasmTestCall(t, s, ctx, "wasm_unload", types.NewStr("example"))
	if r.Flow != types.FlowAbortAttempt || ctx.IrreversibleSideEffect {
		t.Fatalf("boundary: %+v", r)
	}
	ctx.BeforeIrreversibleEffect = nil
	r = wasmTestCall(t, s, ctx, "wasm_unload", types.NewStr("example"))
	if r.IsError() || r.Val.Int() != 1 || !ctx.IrreversibleSideEffect {
		t.Fatalf("unload: %+v", r)
	}
	r = wasmTestCall(t, s, ctx, "wasm_call", types.NewStr("example.add"), types.NewInt(1), types.NewInt(2))
	if r.Error != types.E_INVARG {
		t.Fatalf("unloaded call: %+v", r)
	}
	r = wasmTestCall(t, s, ctx, "wasm_load", types.NewStr("example"))
	if r.IsError() || r.Val.Type() != types.TYPE_MAP {
		t.Fatalf("load: %+v", r)
	}
	for _, tc := range []struct {
		name string
		args []types.Value
		err  types.ErrorCode
	}{
		{"wasm_call", nil, types.E_ARGS},
		{"wasm_call", []types.Value{types.NewInt(9)}, types.E_TYPE},
		{"wasm_load", []types.Value{types.NewInt(9)}, types.E_TYPE},
	} {
		r = wasmTestCall(t, s, ctx, tc.name, tc.args...)
		if r.Error != tc.err {
			t.Fatalf("descriptor %s: %+v", tc.name, r)
		}
	}
}
func TestWASMSessionIsolationAndLimits(t *testing.T) {
	a, b := wasmTestSession(t), wasmTestSession(t)
	aCtx, bCtx := newTestExecutionForSession(a), newTestExecutionForSession(b)
	aCtx.IsWizard = true
	wasmTestCall(t, a, aCtx, "wasm_unload", types.NewStr("example"))
	r := wasmTestCall(t, b, bCtx, "wasm_call", types.NewStr("example.counter"))
	if r.IsError() || r.Val.Int() != 1 {
		t.Fatalf("other session: %+v", r)
	}
	bCtx.TicksRemaining = 1000
	r = wasmTestCall(t, b, bCtx, "wasm_call", types.NewStr("example.counter"))
	if r.Error != types.E_QUOTA {
		t.Fatalf("ticks: %+v", r)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.ConfigureWASM(t.TempDir()); err == nil {
		t.Fatal("closed session reopened")
	}
	r = wasmTestCall(t, b, newTestExecutionForSession(b), "wasm_call", types.NewStr("example.counter"))
	if r.Error != types.E_INVARG {
		t.Fatalf("closed: %+v", r)
	}
	disabled := NewSession(NewRegistry(), NoHost())
	defer disabled.Close()
	r = wasmTestCall(t, disabled, newTestExecutionForSession(disabled), "wasm_modules")
	if r.IsError() || r.Val.Len() != 0 {
		t.Fatalf("disabled listing: %+v", r)
	}
}

func TestWASMCatalogIDsStayStableAcrossLoading(t *testing.T) {
	s := wasmTestSession(t)
	before, _ := s.Registry().GetID("wasm_call")
	ctx := newTestExecutionForSession(s)
	ctx.IsWizard = true
	wasmTestCall(t, s, ctx, "wasm_unload", types.NewStr("example"))
	wasmTestCall(t, s, ctx, "wasm_load", types.NewStr("example"))
	if after, _ := s.Registry().GetID("wasm_call"); before != after {
		t.Fatalf("mutable ABI: %d -> %d", before, after)
	}
}
