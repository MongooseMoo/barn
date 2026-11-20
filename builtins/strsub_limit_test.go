package builtins

import (
	"runtime"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// strsubTestLimit is a max_string_concat small enough that an over-limit
// result costs the test kilobytes, not gigabytes.
const strsubTestLimit = 4096

func newStrsubLimitExecution(limit int) *Execution {
	session := NewSession(NewRegistry(), NoHost())
	snapshot := defaultServerOptionsSnapshot()
	snapshot.MaxStringConcat = limit
	session.applyServerOptionsSnapshot(&snapshot)
	return newTestExecutionForSession(session)
}

func strsubArgs(subject, what, with string, caseMatters bool) []types.Value {
	flag := int64(0)
	if caseMatters {
		flag = 1
	}
	return []types.Value{types.NewStr(subject), types.NewStr(what), types.NewStr(with), types.NewInt(flag)}
}

var strsubCaseModes = []struct {
	name        string
	caseMatters bool
}{
	{"case_sensitive", true},
	{"case_insensitive", false},
}

// A result far over max_string_concat is refused without being built: the
// call may allocate a small multiple of the limit, not the result's size.
func TestStrsubOverLimitDoesNotBuildResult(t *testing.T) {
	const (
		runs     = 4
		maxBytes = 16 * strsubTestLimit
	)
	subject := strings.Repeat("a", 2000)
	// 2000 matches, each replaced by 2000 bytes: a 4,000,000-byte result.
	for _, mode := range strsubCaseModes {
		t.Run(mode.name, func(t *testing.T) {
			ctx := newStrsubLimitExecution(strsubTestLimit)
			args := strsubArgs(subject, "a", subject, mode.caseMatters)

			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			for range runs {
				result := builtinStrsub(ctx, args)
				if !result.IsError() || result.Error != types.E_QUOTA {
					t.Fatalf("strsub = flow %v, error %s; want E_QUOTA", result.Flow, result.Error)
				}
			}
			runtime.ReadMemStats(&after)

			perCall := (after.TotalAlloc - before.TotalAlloc) / runs
			if perCall > maxBytes {
				t.Fatalf("strsub allocated %d bytes per call for a refused result; want at most %d (limit %d)",
					perCall, maxBytes, strsubTestLimit)
			}
		})
	}
}

// The limit is inclusive: a result of exactly max_string_concat bytes is
// returned, one byte more is E_QUOTA.
func TestStrsubLimitBoundary(t *testing.T) {
	// Half the subject is replaced by two bytes per match, half is copied.
	half := strsubTestLimit / 3
	replacedAtLimit := strings.Repeat("a", half) + strings.Repeat("b", strsubTestLimit-2*half)

	cases := []struct {
		name    string
		subject string
		what    string
		with    string
		wantLen int // -1 means E_QUOTA
	}{
		{"growing_result_at_limit", replacedAtLimit, "a", "xy", strsubTestLimit},
		{"growing_result_one_over", replacedAtLimit + "b", "a", "xy", -1},
		{"growing_result_one_over_by_match", replacedAtLimit[:len(replacedAtLimit)-1] + "a", "a", "xy", -1},
		{"no_match_at_limit", strings.Repeat("b", strsubTestLimit), "a", "xy", strsubTestLimit},
		{"no_match_one_over", strings.Repeat("b", strsubTestLimit+1), "a", "xy", -1},
		{"shrinking_to_limit", strings.Repeat("b", strsubTestLimit) + "aa", "aa", "", strsubTestLimit},
		{"shrinking_one_over", strings.Repeat("b", strsubTestLimit+1) + "aa", "aa", "", -1},
		{"last_match_fills_limit", strings.Repeat("b", strsubTestLimit-3) + "a", "a", "xyz", strsubTestLimit},
		{"last_match_passes_limit", strings.Repeat("b", strsubTestLimit-3) + "a", "a", "wxyz", -1},
	}

	for _, mode := range strsubCaseModes {
		for _, tc := range cases {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				ctx := newStrsubLimitExecution(strsubTestLimit)
				result := builtinStrsub(ctx, strsubArgs(tc.subject, tc.what, tc.with, mode.caseMatters))
				if tc.wantLen < 0 {
					if !result.IsError() || result.Error != types.E_QUOTA {
						t.Fatalf("strsub = flow %v, error %s; want E_QUOTA", result.Flow, result.Error)
					}
					return
				}
				if result.IsError() {
					t.Fatalf("strsub returned %s; want a %d-byte string", result.Error, tc.wantLen)
				}
				want := strings.ReplaceAll(tc.subject, tc.what, tc.with)
				if got := result.Val.Str(); got != want {
					t.Fatalf("strsub returned %d bytes, want %d bytes equal to strings.ReplaceAll", len(got), len(want))
				}
				if len(want) != tc.wantLen {
					t.Fatalf("test case builds %d bytes, meant %d", len(want), tc.wantLen)
				}
			})
		}
	}
}

// Case-insensitive matching replaces every case variant and copies the rest.
func TestStrsubCaseInsensitiveUnderLimit(t *testing.T) {
	ctx := newStrsubLimitExecution(strsubTestLimit)
	result := builtinStrsub(ctx, strsubArgs("FooBarfOO", "foo", "x", false))
	if result.IsError() || result.Val.Str() != "xBarx" {
		t.Fatalf("strsub = %v, error %s; want \"xBarx\"", result.Val, result.Error)
	}
}

func BenchmarkStrsub(b *testing.B) {
	subject := strings.Repeat("the quick brown fox jumps over the lazy dog ", 24)
	for _, mode := range strsubCaseModes {
		b.Run(mode.name, func(b *testing.B) {
			ctx := newStrsubLimitExecution(defaultMaxStringConcat)
			args := strsubArgs(subject, "the", "that", mode.caseMatters)
			b.ReportAllocs()
			for b.Loop() {
				if result := builtinStrsub(ctx, args); result.IsError() {
					b.Fatalf("strsub returned %s", result.Error)
				}
			}
		})
	}
}
