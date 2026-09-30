package builtins

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Frozen pre-optimization callback, retained only as a differential oracle
// and benchmark control. It deliberately keeps the original Get/SliceStable.
func originalSortCallback(list, keys types.Value, useKeys, natural, reverse bool) types.Value {
	sortList := list
	if useKeys {
		sortList = keys
	}
	n := sortList.Len()
	if n == 0 {
		return types.NewList([]types.Value{})
	}
	if useKeys && list.Len() != keys.Len() {
		return types.NewErr(types.E_INVARG)
	}
	keyType := sortList.Get(1).Type()
	for i := 1; i <= n; i++ {
		kind := sortList.Get(i).Type()
		if kind != keyType || kind == types.TYPE_LIST || kind == types.TYPE_MAP || kind == types.TYPE_ANON || kind == types.TYPE_WAIF {
			return types.NewErr(types.E_TYPE)
		}
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i + 1
	}
	sort.SliceStable(idx, func(i, j int) bool { return sortLess(sortList.Get(idx[i]), sortList.Get(idx[j]), natural) })
	if reverse {
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			idx[i], idx[j] = idx[j], idx[i]
		}
	}
	result := make([]types.Value, n)
	for i, index := range idx {
		result[i] = list.Get(index)
	}
	return types.NewList(result)
}

func TestSortOptimizationMatchesOriginalCallback(t *testing.T) {
	rng := rand.New(rand.NewSource(345))
	for kind := 0; kind < 5; kind++ {
		for size := 0; size < 100; size++ {
			values, keys := make([]types.Value, size), make([]types.Value, size)
			for i := range values {
				values[i] = types.NewInt(int64(i))
				n := int64(rng.Intn(15))
				switch kind {
				case 0:
					keys[i] = types.NewInt(n)
				case 1:
					keys[i] = types.NewFloat(float64(n))
				case 2:
					keys[i] = types.NewObj(types.ObjID(n))
				case 3:
					keys[i] = types.NewErr(types.ErrorCode(n))
				case 4:
					keys[i] = types.NewStr(fmt.Sprintf("item%02d", n))
				}
			}
			for _, natural := range []bool{false, true} {
				for _, reverse := range []bool{false, true} {
					list, keyList := types.NewList(values), types.NewList(keys)
					want := originalSortCallback(list, keyList, true, natural, reverse)
					got := sortCallback(list, keyList, true, natural, reverse)
					if !got.Equal(want) {
						t.Fatalf("kind=%d size=%d natural=%v reverse=%v got=%s want=%s", kind, size, natural, reverse, got, want)
					}
				}
			}
		}
	}
	for _, values := range [][]types.Value{
		{types.NewFloat(math.NaN()), types.NewFloat(1), types.NewFloat(math.Inf(-1)), types.NewFloat(math.Copysign(0, -1)), types.NewFloat(0)},
		{types.NewInt(1), types.NewStr("mixed")},
		{types.NewEmptyList()},
	} {
		list := types.NewList(values)
		for _, reverse := range []bool{false, true} {
			if got, want := sortCallback(list, types.None, false, false, reverse), originalSortCallback(list, types.None, false, false, reverse); got.String() != want.String() {
				t.Fatalf("got=%s want=%s", got, want)
			}
		}
	}
}

var callbackBenchmarkResult types.Value

func BenchmarkSortCallbacks(b *testing.B) {
	for _, n := range []int{32, 1024} {
		for _, natural := range []bool{false, true} {
			values := make([]types.Value, n)
			for i := range values {
				values[i] = types.NewStr(fmt.Sprintf("Item%d", (i*7919)%n))
			}
			list := types.NewList(values)
			for _, implementation := range []string{"control", "candidate"} {
				b.Run(fmt.Sprintf("n=%d/natural=%v/impl=%s", n, natural, implementation), func(b *testing.B) {
					callback := originalSortCallback
					if implementation == "candidate" {
						callback = sortCallback
					}
					b.ReportAllocs()
					for b.Loop() {
						callbackBenchmarkResult = callback(list, types.None, false, natural, false)
					}
				})
			}
		}
	}
}

func BenchmarkAllMembersCallbacks(b *testing.B) {
	for _, step := range []int{0, 64, 1} {
		values := make([]types.Value, 1024)
		for i := range values {
			values[i] = types.NewStr("miss")
			if step != 0 && i%step == 0 {
				values[i] = types.NewStr("MATCH")
			}
		}
		list, needle := types.NewList(values), types.NewStr("match")
		for _, implementation := range []string{"control", "candidate"} {
			b.Run(fmt.Sprintf("step=%d/impl=%s", step, implementation), func(b *testing.B) {
				ctx := newTestExecution()
				ctx.ThreadMode = false
				args := []types.Value{needle, list, types.NewInt(0)}
				b.ReportAllocs()
				for b.Loop() {
					if implementation == "candidate" {
						callbackBenchmarkResult = builtinAllMembers(ctx, args).Val
						continue
					}
					callbackBenchmarkResult = backgroundValue(ctx, func() types.Value {
						result := make([]types.Value, 0)
						for i := 1; i <= list.Len(); i++ {
							item := list.Get(i)
							if needle.Type() == types.TYPE_STR && item.Type() == types.TYPE_STR && strings.EqualFold(needle.Str(), item.Str()) {
								result = append(result, types.NewInt(int64(i)))
							}
						}
						return types.NewList(result)
					}).Val
				}
			})
		}
	}
}
