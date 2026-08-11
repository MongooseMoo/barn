package builtins

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestBuiltinRmatchReturnsRightmostMatch(t *testing.T) {
	tests := []struct {
		name         string
		subject      string
		pattern      string
		wantStart    int64
		wantEnd      int64
		wantSubStart int64
		wantSubEnd   int64
	}{
		{name: "multiple matches", subject: "one two one", pattern: "one", wantStart: 9, wantEnd: 11},
		{name: "overlapping matches", subject: "aaa", pattern: "aa", wantStart: 2, wantEnd: 3},
		{name: "capture offsets", subject: "ababa", pattern: "%(a.%)", wantStart: 3, wantEnd: 4, wantSubStart: 3, wantSubEnd: 4},
		{name: "empty match", subject: "abc", pattern: "", wantStart: 4, wantEnd: 3},
		{name: "suffix relative beginning", subject: "abc", pattern: "^.", wantStart: 3, wantEnd: 3},
		{name: "suffix relative word boundary", subject: "aa", pattern: "%ba", wantStart: 2, wantEnd: 2},
		{name: "end anchor", subject: "aba", pattern: "a$", wantStart: 3, wantEnd: 3},
		{name: "alternation", subject: "ab", pattern: "ab%|b", wantStart: 2, wantEnd: 2},
	}

	ctx := newTestExecution()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := builtinRmatch(ctx, []types.Value{types.NewStr(tc.subject), types.NewStr(tc.pattern)})
			if result.Flow != types.FlowNormal {
				t.Fatalf("rmatch(%q, %q) flow = %v, error = %v", tc.subject, tc.pattern, result.Flow, result.Error)
			}
			if got := result.Val.Get(1).Int(); got != tc.wantStart {
				t.Errorf("rmatch(%q, %q) start = %d, want %d", tc.subject, tc.pattern, got, tc.wantStart)
			}
			if got := result.Val.Get(2).Int(); got != tc.wantEnd {
				t.Errorf("rmatch(%q, %q) end = %d, want %d", tc.subject, tc.pattern, got, tc.wantEnd)
			}
			if tc.wantSubStart != 0 {
				firstSub := result.Val.Get(3).Get(1)
				if got := firstSub.Get(1).Int(); got != tc.wantSubStart {
					t.Errorf("rmatch(%q, %q) capture start = %d, want %d", tc.subject, tc.pattern, got, tc.wantSubStart)
				}
				if got := firstSub.Get(2).Int(); got != tc.wantSubEnd {
					t.Errorf("rmatch(%q, %q) capture end = %d, want %d", tc.subject, tc.pattern, got, tc.wantSubEnd)
				}
			}
		})
	}
}

func TestBuiltinRmatchMatchesLegacySuffixSemantics(t *testing.T) {
	tests := []struct {
		subject string
		pattern string
	}{
		{subject: "aaa", pattern: "aa"},
		{subject: "ababa", pattern: "%(a.%)"},
		{subject: "abc", pattern: ""},
		{subject: "ab", pattern: "ab%|b"},
		{subject: "abc", pattern: "^."},
		{subject: "aa", pattern: "%ba"},
		{subject: "a\nb", pattern: "."},
		{subject: "foobar", pattern: ".*"},
		{subject: "aaaa", pattern: "a+"},
		{subject: "ABab", pattern: "ab"},
		{subject: "é", pattern: "."},
	}

	ctx := newTestExecution()
	for _, tc := range tests {
		got := builtinRmatch(ctx, []types.Value{types.NewStr(tc.subject), types.NewStr(tc.pattern)})
		want := legacyRmatchForTest(tc.subject, tc.pattern)
		if got.Flow != types.FlowNormal || got.Val.String() != want.String() {
			t.Errorf("rmatch(%q, %q) = %s (flow %v), want legacy %s", tc.subject, tc.pattern, got.Val.String(), got.Flow, want.String())
		}
	}
}

func legacyRmatchForTest(subject, pattern string) types.Value {
	re, err := cachedMOOPattern(pattern, false, true)
	if err != nil {
		panic(err)
	}
	var best []int
	for i := 0; i <= len(subject); i++ {
		loc := re.FindStringSubmatchIndex(subject[i:])
		if loc == nil {
			continue
		}
		best = make([]int, len(loc))
		for j, index := range loc {
			if index < 0 {
				best[j] = -1
			} else {
				best[j] = index + i
			}
		}
	}
	if best == nil {
		return types.NewList(nil)
	}
	return buildMatchResult(subject, best)
}

func TestBuiltinRmatchLargeInputHasBoundedAllocations(t *testing.T) {
	ctx := newTestExecution()
	args := []types.Value{types.NewStr(strings.Repeat("x", 8*1024)), types.NewStr("x")}

	// Prime the regexp cache so this measures each rmatch call, not compilation.
	if result := builtinRmatch(ctx, args); result.Flow != types.FlowNormal {
		t.Fatalf("priming rmatch flow = %v, error = %v", result.Flow, result.Error)
	}

	allocs := testing.AllocsPerRun(1, func() {
		if result := builtinRmatch(ctx, args); result.Flow != types.FlowNormal {
			t.Fatalf("rmatch flow = %v, error = %v", result.Flow, result.Error)
		}
	})
	if allocs > 128 {
		t.Fatalf("rmatch allocated %.0f objects for an 8 KiB subject; want at most 128", allocs)
	}
}

func BenchmarkBuiltinRmatch200KB(b *testing.B) {
	ctx := newTestExecution()
	args := []types.Value{types.NewStr(strings.Repeat("x", 200*1024)), types.NewStr("x")}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := builtinRmatch(ctx, args)
		if result.Flow != types.FlowNormal {
			b.Fatalf("rmatch flow = %v, error = %v", result.Flow, result.Error)
		}
	}
}
