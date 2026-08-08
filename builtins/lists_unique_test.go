package builtins

import (
	"math"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func TestBuiltinUniquePreservesValueEquality(t *testing.T) {
	waif := types.NewWaif(1, 2)
	otherWaif := types.NewWaif(1, 2)
	nan := types.NewFloat(math.NaN())
	values := []types.Value{
		types.NewInt(1), types.NewInt(1),
		types.NewFloat(0), types.NewFloat(math.Copysign(0, -1)),
		types.NewStr("hello"), types.NewStr("HELLO"),
		types.NewList([]types.Value{types.NewInt(2)}), types.NewList([]types.Value{types.NewInt(2)}),
		types.NewMap([][2]types.Value{{types.NewStr("key"), types.NewInt(3)}}),
		types.NewMap([][2]types.Value{{types.NewStr("KEY"), types.NewInt(3)}}),
		waif, waif, otherWaif,
		nan, nan,
	}

	result := builtinUnique(reviewDataCtx(), []types.Value{types.NewList(values)})
	if !result.IsNormal() {
		t.Fatalf("unique() returned error: %v", result.Error)
	}
	got := result.Val
	if got.Len() != 9 {
		t.Fatalf("unique() returned %d values, want 9", got.Len())
	}
	if got.Get(3).Str() != "hello" {
		t.Errorf("unique() kept %q for the string bucket, want the first occurrence %q", got.Get(3).Str(), "hello")
	}
	if got.Get(6).WaifIdentity() != waif.WaifIdentity() || got.Get(7).WaifIdentity() != otherWaif.WaifIdentity() {
		t.Error("unique() did not preserve waif reference-identity equality")
	}
	if !got.Get(8).IsNaN() || !got.Get(9).IsNaN() {
		t.Error("unique() collapsed NaN values, which are never equal")
	}

	unicodeFold := builtinUnique(reviewDataCtx(), []types.Value{types.NewList([]types.Value{
		types.NewStr("K"),
		types.NewStr("K"),
	})})
	if !unicodeFold.IsNormal() || unicodeFold.Val.Len() != 1 {
		t.Errorf("unique({K, K}) returned flow %v, error %v, length %d; want one Unicode-fold-equal value", unicodeFold.Flow, unicodeFold.Error, unicodeFold.Val.Len())
	}
}

func TestBuiltinUniqueDistinctScalarsDoesNotScaleQuadratically(t *testing.T) {
	measure := func(size int) time.Duration {
		values := make([]types.Value, size)
		for i := range values {
			values[i] = types.NewInt(int64(i))
		}
		list := types.NewList(values)
		ctx := reviewDataCtx()

		best := time.Duration(1<<63 - 1)
		for range 3 {
			start := time.Now()
			result := builtinUnique(ctx, []types.Value{list})
			elapsed := time.Since(start)
			if !result.IsNormal() || result.Val.Len() != size {
				t.Fatalf("unique() returned flow %v, error %v, length %d; want a normal %d-element result", result.Flow, result.Error, result.Val.Len(), size)
			}
			if elapsed < best {
				best = elapsed
			}
		}
		return best
	}

	// Four times as much distinct scalar input takes roughly sixteen times as
	// long with the old all-pairs scan. Allow twice the expected linear growth,
	// plus a small fixed allowance for scheduler noise on slower CI workers.
	small := measure(2_500)
	large := measure(10_000)
	if large > 8*small+10*time.Millisecond {
		t.Fatalf("unique() scales quadratically: 2,500 values took %v, 10,000 took %v", small, large)
	}
}

func BenchmarkBuiltinUniqueDistinct10000(b *testing.B) {
	values := make([]types.Value, 10_000)
	for i := range values {
		values[i] = types.NewInt(int64(i))
	}
	list := types.NewList(values)
	ctx := reviewDataCtx()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		result := builtinUnique(ctx, []types.Value{list})
		if !result.IsNormal() || result.Val.Len() != len(values) {
			b.Fatalf("unique() returned flow %v, error %v, length %d; want a normal %d-element result", result.Flow, result.Error, result.Val.Len(), len(values))
		}
	}
}
