package builtins

import (
	"regexp"
	"regexp/syntax"
	"strings"
	"sync"
	"time"

	"github.com/dlclark/regexp2"
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
	rawPCRE       bool
}

// pcreMatchTimeout bounds a single pcre_match/pcre_replace evaluation. PCRE2
// stops a runaway pattern through its match limit and Toast raises E_INVARG;
// regexp2 is a backtracking engine with the same failure mode, so the timeout
// is the equivalent guard and surfaces as the same E_INVARG.
const pcreMatchTimeout = 2 * time.Second

// pcrePattern is a compiled pcre_match()/pcre_replace() pattern. Toast runs
// these on PCRE2, so lookaround, backreferences and (?P<name>) groups must
// work; Go's RE2 regexp rejects all three, hence regexp2. groups lists the
// capture groups in PCRE numbering (left to right by opening parenthesis):
// the group's name, or "" for an unnamed group. regexp2 follows .NET and
// numbers unnamed groups before named ones, so the MOO-visible keys come from
// this list rather than from regexp2's numbering.
type pcrePattern struct {
	re     *regexp2.Regexp
	groups []string
}

// MatchString reports whether the pattern matches anywhere in s.
func (p *pcrePattern) MatchString(s string) bool {
	ok, err := p.re.MatchString(s)
	return err == nil && ok
}

// cachedPCREPattern returns the compiled pattern for the raw PCRE patterns
// accepted by pcre_match() and pcre_replace(). Raw PCRE patterns use a distinct
// key space from translated MOO patterns, even when the source text and flags
// match.
func cachedPCREPattern(pattern string, caseSensitive bool) (*pcrePattern, error) {
	key := regexpCacheKey{pattern: pattern, caseSensitive: caseSensitive, rawPCRE: true}

	regexpCacheMu.RLock()
	entry, ok := regexpCache[key]
	regexpCacheMu.RUnlock()
	if ok {
		return entry.pcre, entry.err
	}

	// RE2 compatibility mode keeps regexp2's lookaround and backreferences and
	// adds PCRE's (?P<name>) syntax and ASCII \w \d \s classes (PCRE2 without
	// UCP), which is what Toast compiles.
	opts := regexp2.RegexOptions(regexp2.RE2)
	if !caseSensitive {
		opts |= regexp2.IgnoreCase
	}
	re, err := regexp2.Compile(pattern, opts)
	if err == nil {
		re.MatchTimeout = pcreMatchTimeout
		entry = regexpCacheEntry{pcre: &pcrePattern{re: re, groups: pcreCaptureGroups(pattern)}}
	} else {
		entry = regexpCacheEntry{err: err}
	}

	regexpCacheMu.Lock()
	if len(regexpCache) >= regexpCacheCap {
		regexpCache = make(map[regexpCacheKey]regexpCacheEntry, regexpCacheCap)
	}
	regexpCache[key] = entry
	regexpCacheMu.Unlock()

	return entry.pcre, entry.err
}

// pcreCaptureGroups scans a PCRE pattern and returns its capture groups in
// PCRE numbering order: the name of each named group ((?P<n>), (?<n>), (?'n'))
// or "" for a plain (...) group. Non-capturing constructs — (?:, lookaround,
// atomic groups, inline flags, comments — are skipped, as are escapes and the
// contents of character classes.
func pcreCaptureGroups(pattern string) []string {
	var groups []string
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++ // the escaped character is never syntax
		case '[':
			i = skipPCREClass(pattern, i)
		case '(':
			if i+1 >= len(pattern) || pattern[i+1] != '?' {
				if i+1 < len(pattern) && pattern[i+1] == '*' {
					continue // (*VERB) control verb, not a group
				}
				groups = append(groups, "")
				continue
			}
			rest := pattern[i+2:]
			var close byte
			switch {
			case strings.HasPrefix(rest, "P<"):
				rest, close = rest[1:], '>'
			case strings.HasPrefix(rest, "<") && !strings.HasPrefix(rest, "<=") && !strings.HasPrefix(rest, "<!"):
				close = '>'
			case strings.HasPrefix(rest, "'"):
				close = '\''
			default:
				continue // non-capturing construct
			}
			end := strings.IndexByte(rest[1:], close)
			if end < 0 {
				continue
			}
			groups = append(groups, rest[1:1+end])
		}
	}
	return groups
}

// skipPCREClass returns the index of the ']' closing the character class that
// opens at pattern[start], honoring a leading ']' or '^]' literal and escapes.
func skipPCREClass(pattern string, start int) int {
	i := start + 1
	if i < len(pattern) && pattern[i] == '^' {
		i++
	}
	if i < len(pattern) && pattern[i] == ']' {
		i++
	}
	for ; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '[':
			// POSIX class such as [:alpha:] inside the set.
			if i+1 < len(pattern) && pattern[i+1] == ':' {
				if end := strings.Index(pattern[i+2:], ":]"); end >= 0 {
					i += 2 + end + 1
				}
			}
		case ']':
			return i
		}
	}
	return len(pattern)
}

// regexpCacheEntry also memoizes failures: an invalid pattern costs the same
// translate+compile work as a valid one and is equally repeatable from MOO.
type regexpCacheEntry struct {
	re                 *regexp.Regexp
	pcre               *pcrePattern
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
