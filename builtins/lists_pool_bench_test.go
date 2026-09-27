package builtins

import (
	"fmt"
	"github.com/MongooseMoo/barn/types"
	"testing"
)

func poolUniqueInput(n int, mixed, duplicate bool) types.Value {
	values := make([]types.Value, n)
	for i := range values {
		k := i
		if duplicate {
			k %= 16
		}
		if mixed && i%2 == 0 {
			values[i] = types.NewStr(fmt.Sprintf("value-%d", k))
		} else {
			values[i] = types.NewInt(int64(k))
		}
	}
	return types.NewList(values)
}

func BenchmarkPoolUnique(b *testing.B) {
	for _, tc := range []struct {
		name             string
		n                int
		mixed, duplicate bool
	}{
		{"Tiny8", 8, false, false}, {"Mixed128", 128, true, false}, {"Distinct10000", 10000, false, false}, {"Duplicate4096", 4096, true, true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			list := poolUniqueInput(tc.n, tc.mixed, tc.duplicate)
			want := tc.n
			if tc.duplicate {
				want = 16
			}
			ctx := reviewDataCtx()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				r := builtinUnique(ctx, []types.Value{list})
				if !r.IsNormal() || r.Val.Len() != want {
					b.Fatal(r)
				}
			}
		})
	}
}

// Reserved for the independent promotion verifier; excluded from development runs.
func BenchmarkPoolUniqueHoldout(b *testing.B) {
	list := poolUniqueInput(257, true, false)
	ctx := reviewDataCtx()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r := builtinUnique(ctx, []types.Value{list})
		if !r.IsNormal() || r.Val.Len() != 257 {
			b.Fatal(r)
		}
	}
}

func TestUniqueResultSurvivesSubsequentCalls(t *testing.T) {
	ctx := reviewDataCtx()
	list := poolUniqueInput(128, true, false)
	first := builtinUnique(ctx, []types.Value{list})
	for range 20 {
		builtinUnique(ctx, []types.Value{poolUniqueInput(4096, true, true)})
	}
	if !first.IsNormal() || !first.Val.Equal(list) {
		t.Fatal("returned list changed after scratch reuse")
	}
}
