package wasmbuiltins

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MongooseMoo/barn/types"
	"github.com/samyfodil/wazy/component"
)

type callFixture struct {
	module, export string
	args           []types.Value
	want           types.Value
}

func callFixtures() []callFixture {
	args := []types.Value{types.NewInt(20), types.NewInt(22)}
	return []callFixture{
		{"example", "add", args, types.NewInt(42)},
		{"scalar", "add", args, types.NewInt(42)},
	}
}

func benchmarkManager(b *testing.B, module string) *Manager {
	b.Helper()
	dir := b.TempDir()
	data, err := os.ReadFile(filepath.Join("testdata", module+".wasm"))
	if err != nil {
		b.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, module+".wasm"), data, 0600); err != nil {
		b.Fatal(err)
	}
	m, err := Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := m.Close(); err != nil {
			b.Error(err)
		}
	})
	if _, err = m.Load(context.Background(), module); err != nil {
		b.Fatal(err)
	}
	return m
}

// BenchmarkWASMCall splits a wasm_call into its fresh-guest setup and the
// export itself. ExecuteExisting reuses one guest and is a diagnostic floor,
// not a supported calling mode.
func BenchmarkWASMCall(b *testing.B) {
	for _, fixture := range callFixtures() {
		b.Run(fixture.module, func(b *testing.B) {
			m := benchmarkManager(b, fixture.module)
			g := m.modules[fixture.module]
			target := fixture.module + "." + fixture.export
			b.Run("Full", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					r := m.Call(context.Background(), 0, target, fixture.args)
					if r.IsError() || !r.Val.Equal(fixture.want) {
						b.Fatalf("call: %+v", r)
					}
				}
			})
			b.Run("InstantiateClose", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					ctx, cancel := context.WithTimeout(context.Background(), CallTimeout)
					inst, err := component.Instantiate(ctx, g.runtime, g.data, component.WithCompileCache(g.cache))
					if err != nil {
						cancel()
						b.Fatal(err)
					}
					err = inst.Close(context.Background())
					cancel()
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("ExecuteExisting", func(b *testing.B) {
				fn := g.functions[fixture.export]
				values := make([]any, len(fixture.args))
				for i, p := range fn.params {
					v, e := p.shape.lower(fixture.args[i], &budget{})
					if e != types.E_NONE {
						b.Fatal(e)
					}
					values[i] = v
				}
				inst, err := component.Instantiate(context.Background(), g.runtime, g.data, component.WithCompileCache(g.cache))
				if err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() {
					if err := inst.Close(context.Background()); err != nil {
						b.Error(err)
					}
				})
				b.ReportAllocs()
				for b.Loop() {
					ctx, cancel := context.WithTimeout(context.Background(), CallTimeout)
					out, err := inst.Call(ctx, fixture.export, values...)
					cancel()
					if err != nil || len(out) != 1 {
						b.Fatalf("call: %v %v", out, err)
					}
					v, e := fn.result.lift(out[0], &budget{})
					if e != types.E_NONE || !v.Equal(fixture.want) {
						b.Fatalf("result: %v %v", v, e)
					}
				}
			})
		})
	}
}
