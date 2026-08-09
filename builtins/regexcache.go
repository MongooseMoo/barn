package builtins

import (
	"regexp"
	"regexp/syntax"
	"sync"
)

// MOO code calls match()/rmatch() with a small recurring set of patterns
// ($string_utils:regexp_quote alone runs rmatch(s, "[][$^.*+?%].*") on every
// call), so re-translating and re-compiling per call dominated allocation on
// the Mongoose workload (~3.9GB per 28s profile in regexp/syntax).
//
// The cache is bounded: MOO code can synthesize unlimited distinct patterns, so
// the map is dropped wholesale once it exceeds regexpCacheCap rather than grown.
// Wholesale eviction keeps the hit path a single RLock + map read; entries in
// flight stay valid because *regexp.Regexp is immutable and callers hold their
// own reference.
const regexpCacheCap = 1024

// regexpCacheKey must capture every input to the compiled pattern. Case and
// anchoring are folded into the Go pattern by the callers, not into the MOO
// pattern, so both flags are part of the key.
type regexpCacheKey struct {
	pattern       string
	caseSensitive bool
	anchored      bool
	rightmost     bool
}

// regexpCacheEntry also memoizes failures: an invalid pattern costs the same
// translate+compile work as a valid one and is equally repeatable from MOO.
type regexpCacheEntry struct {
	re                 *regexp.Regexp
	requiresSuffixScan bool
	err                error
}

var (
	regexpCacheMu sync.RWMutex
	regexpCache   = make(map[regexpCacheKey]regexpCacheEntry)
)

// cachedMOOPattern returns the compiled Go regexp for a MOO pattern.
// caseSensitive=false prefixes "(?i)"; anchored=true wraps the result in
// "^(?:...)" for rmatch's left-anchored scan. Results and errors are identical
// to translating and compiling on every call.
func cachedMOOPattern(pattern string, caseSensitive, anchored bool) (*regexp.Regexp, error) {
	key := regexpCacheKey{pattern: pattern, caseSensitive: caseSensitive, anchored: anchored}

	regexpCacheMu.RLock()
	entry, ok := regexpCache[key]
	regexpCacheMu.RUnlock()
	if ok {
		return entry.re, entry.err
	}

	entry = compileMOOPattern(pattern, caseSensitive, anchored)

	regexpCacheMu.Lock()
	if len(regexpCache) >= regexpCacheCap {
		regexpCache = make(map[regexpCacheKey]regexpCacheEntry, regexpCacheCap)
	}
	regexpCache[key] = entry
	regexpCacheMu.Unlock()

	return entry.re, entry.err
}

// cachedMOORightmostPattern compiles a regexp that selects the match with the
// greatest starting byte in one pass. The leading greedy wildcard fixes the
// overall match at byte zero while forcing the captured MOO pattern as far
// right as it can go; the caller removes that wrapper capture from the result.
//
// Assertions about the beginning of the input or a word boundary observe the
// artificial start of every suffix in the historical rmatch implementation.
// Those patterns must keep the suffix scan to preserve exact behavior.
func cachedMOORightmostPattern(pattern string, caseSensitive bool) (*regexp.Regexp, bool, error) {
	key := regexpCacheKey{pattern: pattern, caseSensitive: caseSensitive, rightmost: true}

	regexpCacheMu.RLock()
	entry, ok := regexpCache[key]
	regexpCacheMu.RUnlock()
	if ok {
		return entry.re, entry.requiresSuffixScan, entry.err
	}

	entry = compileMOORightmostPattern(pattern, caseSensitive)

	regexpCacheMu.Lock()
	if len(regexpCache) >= regexpCacheCap {
		regexpCache = make(map[regexpCacheKey]regexpCacheEntry, regexpCacheCap)
	}
	regexpCache[key] = entry
	regexpCacheMu.Unlock()

	return entry.re, entry.requiresSuffixScan, entry.err
}

func compileMOOPattern(pattern string, caseSensitive, anchored bool) regexpCacheEntry {
	goPattern, err := mooPatternToGoRegex(pattern)
	if err != nil {
		return regexpCacheEntry{err: err}
	}
	pat := goPattern
	if !caseSensitive {
		pat = "(?i)" + pat
	}
	if anchored {
		pat = "^(?:" + pat + ")"
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return regexpCacheEntry{err: err}
	}
	return regexpCacheEntry{re: re}
}

func compileMOORightmostPattern(pattern string, caseSensitive bool) regexpCacheEntry {
	goPattern, err := mooPatternToGoRegex(pattern)
	if err != nil {
		return regexpCacheEntry{err: err}
	}
	pat := goPattern
	if !caseSensitive {
		pat = "(?i)" + pat
	}

	parsed, err := syntax.Parse(pat, syntax.Perl)
	if err != nil {
		return regexpCacheEntry{err: err}
	}
	if regexpNeedsSuffixContext(parsed) {
		entry := compileMOOPattern(pattern, caseSensitive, true)
		entry.requiresSuffixScan = true
		return entry
	}

	re, err := regexp.Compile("^(?s:.*)(" + pat + ")")
	if err != nil {
		return regexpCacheEntry{err: err}
	}
	return regexpCacheEntry{re: re}
}

func regexpNeedsSuffixContext(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpBeginLine, syntax.OpBeginText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return true
	}
	for _, sub := range re.Sub {
		if regexpNeedsSuffixContext(sub) {
			return true
		}
	}
	return false
}

func resetRegexpCacheForTest() {
	regexpCacheMu.Lock()
	regexpCache = make(map[regexpCacheKey]regexpCacheEntry)
	regexpCacheMu.Unlock()
}

func regexpCacheLenForTest() int {
	regexpCacheMu.RLock()
	defer regexpCacheMu.RUnlock()
	return len(regexpCache)
}
