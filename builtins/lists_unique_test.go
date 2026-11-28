package builtins

import (
	"math"
	"runtime"
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

	result := admitTestBuiltin("unique", reviewDataCtx(), []types.Value{types.NewList(values)})
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

	unicodeFold := admitTestBuiltin("unique", reviewDataCtx(), []types.Value{types.NewList([]types.Value{
		types.NewStr("K"),
		types.NewStr("K"),
	})})
	if !unicodeFold.IsNormal() || unicodeFold.Val.Len() != 1 {
		t.Errorf("unique({K, K}) returned flow %v, error %v, length %d; want one Unicode-fold-equal value", unicodeFold.Flow, unicodeFold.Error, unicodeFold.Val.Len())
	}
}

func TestBuiltinUniqueBinaryEscapeStrings(t *testing.T) {
	for _, pair := range [][2]string{
		{"~0A", "\n"},
		{"~09", "\t"},
		{"~0a", "~0A"},
		{"K~0a", "K\n"},
		{"\xffA~09", "\xffa\t"},
	} {
		first, second := types.NewStr(pair[0]), types.NewStr(pair[1])
		if !first.Equal(second) {
			t.Fatalf("%q and %q must compare equal", pair[0], pair[1])
		}
		result := admitTestBuiltin("unique", reviewDataCtx(), []types.Value{types.NewList([]types.Value{first, second})})
		if !result.IsNormal() || result.Val.Len() != 1 || !result.Val.Get(1).Identical(first) {
			t.Errorf("unique(%q, %q) = %v, want a list containing only the first string", pair[0], pair[1], result)
		}
	}
}

func TestBuiltinIsMemberStringCaseFlag(t *testing.T) {
	list := types.NewList([]types.Value{
		types.NewInt(1), types.NewStr("A~09"), types.NewStr("a\t"),
	})
	for _, tc := range []struct {
		flags []types.Value
		index int64
	}{
		{nil, 3},
		{[]types.Value{types.NewInt(1)}, 3},
		{[]types.Value{types.NewInt(0)}, 2},
	} {
		args := append([]types.Value{types.NewStr("a\t"), list}, tc.flags...)
		got := admitTestBuiltin("is_member", reviewDataCtx(), args)
		if !got.IsNormal() || !got.Val.Identical(types.NewInt(tc.index)) {
			t.Errorf("is_member with flags %v = %v, want %d", tc.flags, got, tc.index)
		}
	}
}

func TestBuiltinUniqueDistinctScalarsDoesNotScaleQuadratically(t *testing.T) {
	const (
		smallSize = 1_000
		largeSize = 16_000
		// Each sample handles the same number of elements: many calls on the
		// small list, or a sixteenth as many on the large one. A linear
		// unique() takes about as long for both; the old all-pairs scan takes
		// sixteen times as long on the large list. The limit sits four times
		// from each. A sample is long enough (tens of milliseconds) to measure
		// with Windows' coarse clock.
		sampleElements = 256_000
		maxRatio       = 4
		maxRounds      = 10
	)
	distinct := func(size int) types.Value {
		values := make([]types.Value, size)
		for i := range values {
			values[i] = types.NewInt(int64(i))
		}
		return types.NewList(values)
	}
	small, large := distinct(smallSize), distinct(largeSize)
	ctx := reviewDataCtx()
	// measure times sampleElements worth of calls on list, stopping early once
	// the sample exceeds limit.
	measure := func(list types.Value, limit time.Duration) time.Duration {
		runtime.GC()
		start := time.Now()
		for range sampleElements / list.Len() {
			result := admitTestBuiltin("unique", ctx, []types.Value{list})
			if !result.IsNormal() || result.Val.Len() != list.Len() {
				t.Fatalf("unique() returned flow %v, error %v, length %d; want a normal %d-element result", result.Flow, result.Error, result.Val.Len(), list.Len())
			}
			if time.Since(start) > limit {
				break
			}
		}
		return time.Since(start)
	}

	// Load on a shared runner stretches samples, sometimes a whole burst of
	// them. Alternating the sizes puts both under the same load, and the
	// fastest sample of each size is the least disturbed. A linear unique()
	// meets the limit once one quiet sample of each size exists; a quadratic
	// one never does.
	bestSmall, bestLarge := time.Duration(math.MaxInt64), time.Duration(math.MaxInt64)
	for range maxRounds {
		bestSmall = min(bestSmall, measure(small, math.MaxInt64))
		bestLarge = min(bestLarge, measure(large, maxRatio*bestSmall))
		if bestLarge <= maxRatio*bestSmall {
			return
		}
	}
	t.Fatalf("unique() scales quadratically: fastest of %d rounds over %d elements, %d-value calls took %v, %d-value calls took %v (limit %dx)", maxRounds, sampleElements, smallSize, bestSmall, largeSize, bestLarge, maxRatio)
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
		result := admitTestBuiltin("unique", ctx, []types.Value{list})
		if !result.IsNormal() || result.Val.Len() != len(values) {
			b.Fatalf("unique() returned flow %v, error %v, length %d; want a normal %d-element result", result.Flow, result.Error, result.Val.Len(), len(values))
		}
	}
}
