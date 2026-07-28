package builtins

import (
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"testing"
)

// coldCompileMOOPattern reproduces the pre-cache compile path verbatim so the
// cached path can be diffed against it.
func coldCompileMOOPattern(pattern string, caseSensitive, anchored bool) (*regexp.Regexp, error) {
	goPattern, err := mooPatternToGoRegex(pattern)
	if err != nil {
		return nil, err
	}
	pat := goPattern
	if !caseSensitive {
		pat = "(?i)" + goPattern
	}
	if anchored {
		pat = "^(?:" + pat + ")"
	}
	return regexp.Compile(pat)
}

func TestCachedMOOPatternMatchesColdPath(t *testing.T) {
	patterns := []string{
		"[][$^.*+?%].*",
		"%(foo%)%(bar%)",
		"^%w+$",
		"a%db",
		"[^abc]+",
		"%<word%>",
		"",
		"a|b",
	}
	subjects := []string{"", "foo", "FOO", "a$b", "adb", "[bracket]", "word here", "AbC"}

	for _, pattern := range patterns {
		for _, caseSensitive := range []bool{false, true} {
			for _, anchored := range []bool{false, true} {
				want, wantErr := coldCompileMOOPattern(pattern, caseSensitive, anchored)
				got, gotErr := cachedMOOPattern(pattern, caseSensitive, anchored)
				if (wantErr == nil) != (gotErr == nil) {
					t.Fatalf("pattern %q cs=%v anchored=%v: err mismatch cold=%v cached=%v",
						pattern, caseSensitive, anchored, wantErr, gotErr)
				}
				if wantErr != nil {
					continue
				}
				if got.String() != want.String() {
					t.Fatalf("pattern %q cs=%v anchored=%v: compiled %q, want %q",
						pattern, caseSensitive, anchored, got.String(), want.String())
				}
				for _, subject := range subjects {
					wantLoc := want.FindStringSubmatchIndex(subject)
					gotLoc := got.FindStringSubmatchIndex(subject)
					if fmt.Sprint(wantLoc) != fmt.Sprint(gotLoc) {
						t.Fatalf("pattern %q cs=%v anchored=%v subject %q: got %v, want %v",
							pattern, caseSensitive, anchored, subject, gotLoc, wantLoc)
					}
				}
			}
		}
	}
}

func TestCachedMOOPatternReusesCompiledRegexp(t *testing.T) {
	first, err := cachedMOOPattern("%(a%)[0-9]+", true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := cachedMOOPattern("%(a%)[0-9]+", true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first != second {
		t.Fatalf("cache miss on repeat lookup: %p != %p", first, second)
	}
}

func TestCachedMOOPatternVariantsAreDistinct(t *testing.T) {
	insensitive, err := cachedMOOPattern("abc", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sensitive, err := cachedMOOPattern("abc", true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	anchored, err := cachedMOOPattern("abc", true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if insensitive.String() == sensitive.String() {
		t.Fatalf("case flag collapsed: both compiled to %q", sensitive.String())
	}
	if anchored.String() == sensitive.String() {
		t.Fatalf("anchor flag collapsed: both compiled to %q", sensitive.String())
	}
	if insensitive.MatchString("ABC") != true || sensitive.MatchString("ABC") != false {
		t.Fatalf("case semantics wrong: insensitive=%v sensitive=%v",
			insensitive.MatchString("ABC"), sensitive.MatchString("ABC"))
	}
}

func TestCachedMOOPatternCachesTranslationFailure(t *testing.T) {
	// An unterminated character class survives translation and fails Go's compiler.
	bad := "[abc"
	if _, err := coldCompileMOOPattern(bad, true, false); err == nil {
		t.Fatalf("test premise broken: %q compiles", bad)
	}
	for i := 0; i < 2; i++ {
		if _, err := cachedMOOPattern(bad, true, false); err == nil {
			t.Fatalf("attempt %d: expected error for %q", i, bad)
		}
	}
}

func TestCachedMOOPatternEvictsAtCap(t *testing.T) {
	resetRegexpCacheForTest()
	for i := 0; i < regexpCacheCap*3; i++ {
		if _, err := cachedMOOPattern("prefix"+strconv.Itoa(i), true, false); err != nil {
			t.Fatalf("unexpected error at %d: %v", i, err)
		}
		if n := regexpCacheLenForTest(); n > regexpCacheCap {
			t.Fatalf("cache grew past cap after %d inserts: %d > %d", i+1, n, regexpCacheCap)
		}
	}
	if regexpCacheLenForTest() == 0 {
		t.Fatalf("cache empty after %d inserts", regexpCacheCap*3)
	}
}

// BenchmarkMOOPatternCold vs BenchmarkMOOPatternCached quantifies the
// allocation the cache removes from the $string_utils:regexp_quote hot pattern.
func BenchmarkMOOPatternCold(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := coldCompileMOOPattern("[][$^.*+?%].*", false, true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMOOPatternCached(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := cachedMOOPattern("[][$^.*+?%].*", false, true); err != nil {
			b.Fatal(err)
		}
	}
}

func TestCachedMOOPatternConcurrent(t *testing.T) {
	resetRegexpCacheForTest()
	const goroutines = 16
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				// Mix of hot shared patterns and unique ones that force eviction.
				shared, err := cachedMOOPattern("[][$^.*+?%].*", false, i%2 == 0)
				if err != nil {
					t.Errorf("shared pattern error: %v", err)
					return
				}
				if !shared.MatchString("$foo") {
					t.Errorf("shared pattern failed to match")
					return
				}
				if _, err := cachedMOOPattern(fmt.Sprintf("u%d_%d", g, i), true, false); err != nil {
					t.Errorf("unique pattern error: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	if n := regexpCacheLenForTest(); n > regexpCacheCap {
		t.Fatalf("cache grew past cap under concurrency: %d > %d", n, regexpCacheCap)
	}
}
