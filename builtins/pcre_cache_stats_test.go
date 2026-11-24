package builtins

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Pinned Toast-green by moo-conformance-tests builtins/pcre_cache_stats.yaml.

func resetPCRECacheForTest(t *testing.T) {
	t.Helper()
	empty := func() {
		pcreCache.mu.Lock()
		pcreCache.entries = nil
		pcreCache.mu.Unlock()
	}
	empty()
	t.Cleanup(empty)
}

func pcreCacheStatsForTest(t *testing.T) string {
	t.Helper()
	ctx := newTestExecution()
	ctx.IsWizard = true
	res := builtinPcreCacheStats(ctx, nil)
	if res.Flow != types.FlowNormal {
		t.Fatalf("pcre_cache_stats() = %+v", res)
	}
	return res.Val.String()
}

func pcreMatchForTest(t *testing.T, subject, pattern string, caseMatters bool) types.Result {
	t.Helper()
	args := []types.Value{types.NewStr(subject), types.NewStr(pattern)}
	if caseMatters {
		args = append(args, types.NewInt(1))
	}
	return builtinPcreMatch(newTestExecution(), args)
}

func TestPcreCacheStatsCountsHitsPerPatternAndCase(t *testing.T) {
	resetPCRECacheForTest(t)
	if got := pcreCacheStatsForTest(t); got != "{}" {
		t.Fatalf("fresh stats = %s", got)
	}
	pcreMatchForTest(t, "abc", "b", false)
	if got := pcreCacheStatsForTest(t); got != `{{"b", 0}}` {
		t.Fatalf("after first match = %s", got)
	}
	pcreMatchForTest(t, "abc", "b", false)
	pcreMatchForTest(t, "xbx", "b", false)
	pcreMatchForTest(t, "abc", "b", true)
	if got := pcreCacheStatsForTest(t); got != `{{"b", 0}, {"b", 2}}` {
		t.Fatalf("after case-sensitive match = %s", got)
	}
}

func TestPcreCacheStatsOnlySuccessfulMatchCompiles(t *testing.T) {
	resetPCRECacheForTest(t)
	builtinPcreReplace(newTestExecution(), []types.Value{types.NewStr("abc"), types.NewStr("s/b/x/")})
	if res := pcreMatchForTest(t, "abc", "(", false); res.Error != types.E_INVARG {
		t.Fatalf("bad pattern = %+v", res)
	}
	if res := pcreMatchForTest(t, "", "", false); res.Error != types.E_INVARG {
		t.Fatalf("empty pattern with empty subject = %+v", res)
	}
	pcreMatchForTest(t, "", "q", false)
	if got := pcreCacheStatsForTest(t); got != `{{"q", 0}}` {
		t.Fatalf("stats = %s", got)
	}
}

func TestPcreCacheStatsEvictsFewestHitsWhenFull(t *testing.T) {
	for _, newPattern := range []string{"new", "("} {
		t.Run(newPattern, func(t *testing.T) {
			resetPCRECacheForTest(t)
			for i := 1; i <= pcrePatternCacheSize; i++ {
				pcreMatchForTest(t, "x", fmt.Sprintf("p%d", i), false)
			}
			for i := 2; i <= pcrePatternCacheSize; i++ {
				pcreMatchForTest(t, "x", fmt.Sprintf("p%d", i), false)
			}
			pcreMatchForTest(t, "x", newPattern, false)
			pcreCache.mu.Lock()
			defer pcreCache.mu.Unlock()
			want := pcrePatternCacheSize
			if newPattern == "(" {
				want-- // the miss evicts before the compile fails
			}
			if len(pcreCache.entries) != want {
				t.Fatalf("len = %d, want %d", len(pcreCache.entries), want)
			}
			for _, e := range pcreCache.entries {
				if e.pattern == "p1" {
					t.Fatalf("p1 (fewest hits) not evicted: %+v", pcreCache.entries)
				}
			}
		})
	}
}

func TestPcreCacheStatsRequiresWizard(t *testing.T) {
	ctx := newTestExecution()
	ctx.IsWizard = false
	if res := builtinPcreCacheStats(ctx, nil); res.Error != types.E_PERM {
		t.Fatalf("non-wizard = %+v", res)
	}
}
