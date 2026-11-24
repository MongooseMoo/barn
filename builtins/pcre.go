package builtins

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/MongooseMoo/barn/types"
	"github.com/dlclark/regexp2"
)

func builtinPcreMatch(ctx *Execution, args []types.Value) types.Result {
	subject := args[0]
	pattern := args[1]
	ok1 := subject.Type() == types.TYPE_STR
	ok2 := pattern.Type() == types.TYPE_STR
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}
	if pattern.Str() == "" {
		return types.Err(types.E_INVARG)
	}

	caseMatters := false
	if len(args) >= 3 {
		caseMatters = args[2].Truthy()
	}
	findAll := true
	if len(args) == 4 {
		findAll = args[3].Truthy()
	}

	re, err := cachedPCREPattern(pattern.Str(), caseMatters)
	recordPCRECacheLookup(pattern.Str(), caseMatters, err == nil)
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	// An empty subject always yields no matches (Toast returns {}), even for
	// patterns that match the empty string like ".*" or "^$". Toast's match loop
	// is `while (offset < subject_length)` (toaststunt/src/pcre_moo.cc:208); with
	// subject_length == 0 the loop body never runs and ret stays new_list(0).
	// Toast checks the empty pattern and compiles (and caches) the pattern
	// first. Corroborated by conformance test pcre_match_empty_subject
	// (moo-conformance-tests .../builtins/pcre.yaml:201-205 -> value: []).
	if subject.Str() == "" {
		return types.Ok(types.NewList([]types.Value{}))
	}

	text := subject.Str()
	offsets := pcreByteOffsets(text)
	mapped, ok := pcreSubjectRunes(text)
	if !ok {
		return types.Err(types.E_INVARG)
	}
	out := make([]types.Value, 0)
	maxLoops := pcreMatchMaxIterations(ctx)
	m, merr := re.re.FindStringMatch(mapped)
	for loops := uint32(1); m != nil && merr == nil; loops++ {
		if loops >= maxLoops {
			return pcreTooManyIterations(loops)
		}
		entryPairs := make([][2]types.Value, 0, len(re.groups)+1)
		entryPairs = append(entryPairs, [2]types.Value{
			types.NewStr("0"),
			buildPcreCapture(text, offsets, &m.Group),
		})
		unnamed := 0
		for i, name := range re.groups {
			var g *regexp2.Group
			key := name
			if name == "" {
				unnamed++
				key = strconv.Itoa(i + 1)
				g = m.GroupByNumber(unnamed)
			} else {
				g = m.GroupByName(name)
			}
			entryPairs = append(entryPairs, [2]types.Value{
				types.NewStr(key),
				buildPcreCapture(text, offsets, g),
			})
		}
		out = append(out, types.NewMap(entryPairs))
		if !findAll {
			break
		}
		m, merr = re.re.FindNextMatch(m)
	}
	if merr != nil {
		// A match-limit/timeout failure is Toast's E_INVARG raise.
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewList(out))
}

// Toast's bf_pcre_match counts every pass of its match loop against
// $server_options.pcre_match_max_iterations (pcre_moo.cc:198-202): default
// DEFAULT_LOOPS, at least 100, at most 100000000. Toast assigns the option's
// C int to an unsigned int, so a negative option wraps and takes the maximum.
const (
	pcreMatchDefaultLoops = 1000
	pcreMatchMinLoops     = 100
	pcreMatchMaxLoops     = 100000000
)

func pcreMatchMaxIterations(ctx *Execution) uint32 {
	loops := uint32(serverIntOption(ctx, "pcre_match_max_iterations", pcreMatchDefaultLoops))
	return min(max(loops, pcreMatchMinLoops), pcreMatchMaxLoops)
}

// pcreTooManyIterations is Toast's raise when a pass of the match loop finds a
// match and the pass count has reached the limit (pcre_moo.cc:226-233). It
// comes before that match is added, so limit-1 matches is the most a call
// returns.
func pcreTooManyIterations(loops uint32) types.Result {
	message := "Too many iterations of matching loop: " + strconv.FormatUint(uint64(loops), 10)
	return types.Result{
		Flow:  types.FlowException,
		Error: types.E_MAXREC,
		Val:   types.NewList([]types.Value{types.NewErr(types.E_MAXREC), types.NewStr(message), types.NewInt(0)}),
	}
}

// pcreByteOffsets maps regexp2's rune indices, which are MOO character
// positions, to byte offsets for cutting the matched text. It returns nil for
// pure-ASCII subjects, where the two coincide.
func pcreByteOffsets(text string) []int {
	if utf8.RuneCountInString(text) == len(text) {
		return nil
	}
	offsets := make([]int, 0, len(text)+1)
	for i := range text {
		offsets = append(offsets, i)
	}
	return append(offsets, len(text))
}

// regexp2 decodes its input to runes, turning every invalid UTF-8 byte into
// U+FFFD. pcreSubjectRunes instead maps each invalid byte b to base+b, where
// base starts a 256-rune private-use block the subject does not use, so an
// invalid byte matches only itself. Each invalid byte stays one rune, one
// character, and callers cut matched text from the original subject.
func pcreSubjectRunes(subject string) (mapped string, ok bool) {
	if utf8.ValidString(subject) {
		return subject, true
	}
	const firstBlock, lastBlock = 0xf0000 >> 8, 0x10ff00 >> 8
	var used [lastBlock - firstBlock + 1]bool
	for _, r := range subject {
		if b := int(r >> 8); b >= firstBlock && b <= lastBlock {
			used[b-firstBlock] = true
		}
	}
	for i, taken := range used {
		if taken {
			continue
		}
		base := rune((firstBlock + i) << 8)
		var out strings.Builder
		out.Grow(len(subject) + len(subject)/2)
		for j := 0; j < len(subject); {
			r, size := utf8.DecodeRuneInString(subject[j:])
			if r == utf8.RuneError && size == 1 {
				out.WriteRune(base + rune(subject[j]))
			} else {
				out.WriteString(subject[j : j+size])
			}
			j += size
		}
		return out.String(), true
	}
	return "", false
}

func buildPcreCapture(subject string, offsets []int, g *regexp2.Group) types.Value {
	match := ""
	pos := []types.Value{}
	if g != nil && len(g.Captures) > 0 {
		start, end := g.Index, g.Index+g.Length
		byteStart, byteEnd := start, end
		if offsets != nil {
			byteStart, byteEnd = offsets[start], offsets[end]
		}
		match = subject[byteStart:byteEnd]
		// 1-based inclusive character positions.
		pos = []types.Value{types.NewInt(int64(start + 1)), types.NewInt(int64(end))}
	}
	return types.NewMap([][2]types.Value{
		{types.NewStr("match"), types.NewStr(match)},
		{types.NewStr("position"), types.NewList(pos)},
	})
}

func builtinPcreReplace(ctx *Execution, args []types.Value) types.Result {
	subject := args[0]
	spec := args[1]
	ok1 := subject.Type() == types.TYPE_STR
	ok2 := spec.Type() == types.TYPE_STR
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}

	pattern, replacement, flags, ok := parseSedReplaceSpec(spec.Str())
	if !ok || pattern == "" {
		return types.Err(types.E_INVARG)
	}

	global := false
	caseInsensitive := false
	for _, flag := range flags {
		switch flag {
		case 'g':
			global = true
		case 'i':
			caseInsensitive = true
		default:
			return types.Err(types.E_INVARG)
		}
	}

	re, err := cachedPCREPattern(pattern, !caseInsensitive)
	if err != nil {
		return types.Err(types.E_INVARG)
	}

	parts, ok := parsePcrsReplacement(replacement, len(re.groups))
	if !ok {
		return types.Err(types.E_INVARG)
	}
	if ctx == nil || ctx.Session == nil {
		return types.Err(types.E_INVARG)
	}

	source := subject.Str()
	text, ok := pcreSubjectRunes(source)
	if !ok {
		return types.Err(types.E_INVARG)
	}
	offsets := pcreByteOffsets(source)
	byteAt := func(i int) int {
		if offsets == nil {
			return i
		}
		return offsets[i]
	}
	// Build from the original bytes, so invalid UTF-8 needs no restoring and
	// the string limit applies as the result grows, not after an unbounded
	// result has been built.
	var out strings.Builder
	quota := func() types.ErrorCode {
		return ctx.Session.CheckStringLengthForTask(ctx.Context, out.Len())
	}
	last := 0
	m, merr := re.re.FindStringMatch(text)
	for m != nil && merr == nil {
		start := byteAt(m.Index)
		out.WriteString(source[last:start])
		for _, part := range parts {
			out.WriteString(part.text)
			if part.group < 0 {
				continue
			}
			if g := pcreGroupByNumber(re, m, part.group); g != nil && len(g.Captures) > 0 {
				out.WriteString(source[byteAt(g.Index):byteAt(g.Index+g.Length)])
			}
		}
		if errCode := quota(); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		last = byteAt(m.Index + m.Length)
		if !global {
			break
		}
		m, merr = re.re.FindNextMatch(m)
	}
	if merr != nil {
		return types.Err(types.E_INVARG)
	}
	out.WriteString(source[last:])
	if errCode := quota(); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	return types.Ok(types.NewStr(out.String()))
}

// pcrsPart is one literal run of a replacement template followed by the
// capture group it references, or by nothing when group is -1.
type pcrsPart struct {
	text  string
	group int
}

// Toast's pcre_replace runs pcrs (src/dependencies/pcrs.c). Its template
// compiler (pcrs_compile_replacement, pcrs.c:309-371) recognises $N (all the
// digits, an error when N exceeds the pattern's capture count), $& (group 0),
// $+ (the highest-numbered group) and $` / $'; any other '$' is a plain
// character, so "$_", "$$" and "${name}" are literal text. A template may
// reference at most PCRS_MAX_SUBMATCHES-1 (32) groups. pcrs_execute
// (pcrs.c:900-916) copies $` and $' only when their slot number (33, 34) is
// below the match's submatch count, which it never is, so they insert nothing.
const pcrsMaxSubmatches = 33

func parsePcrsReplacement(replacement string, captureCount int) ([]pcrsPart, bool) {
	var parts []pcrsPart
	var lit strings.Builder
	for i := 0; i < len(replacement); {
		if replacement[i] != '$' || i+1 >= len(replacement) {
			lit.WriteByte(replacement[i])
			i++
			continue
		}
		var group int
		next := replacement[i+1]
		switch {
		case next >= '0' && next <= '9':
			n, j := 0, i+1
			for ; j < len(replacement) && replacement[j] >= '0' && replacement[j] <= '9'; j++ {
				if n <= captureCount {
					n = n*10 + int(replacement[j]-'0')
				}
			}
			if n > captureCount || n >= pcrsMaxSubmatches+2 {
				return nil, false
			}
			group, i = n, j
		case next == '&':
			group, i = 0, i+2
		case next == '+':
			group, i = captureCount, i+2
		case next == '`' || next == '\'':
			group, i = pcrsMaxSubmatches, i+2
		default:
			lit.WriteByte('$')
			i++
			continue
		}
		if len(parts) >= pcrsMaxSubmatches-1 {
			return nil, false
		}
		if group >= pcrsMaxSubmatches {
			group = -1 // pcrs copies nothing for these slots
		}
		parts = append(parts, pcrsPart{text: lit.String(), group: group})
		lit.Reset()
	}
	return append(parts, pcrsPart{text: lit.String(), group: -1}), true
}

// pcreGroupByNumber returns PCRE capture group n of m. regexp2 numbers named
// groups after the unnamed ones, so a PCRE number is mapped through the
// pattern's group list, as pcre_match does.
func pcreGroupByNumber(re *pcrePattern, m *regexp2.Match, n int) *regexp2.Group {
	if n == 0 {
		return &m.Group
	}
	if n > len(re.groups) {
		return nil
	}
	if name := re.groups[n-1]; name != "" {
		return m.GroupByName(name)
	}
	unnamed := 0
	for _, name := range re.groups[:n] {
		if name == "" {
			unnamed++
		}
	}
	return m.GroupByNumber(unnamed)
}

func parseSedReplaceSpec(spec string) (pattern, replacement, flags string, ok bool) {
	if len(spec) < 4 || spec[0] != 's' {
		return "", "", "", false
	}
	delim := spec[1]
	pattern, next, ok := readDelimited(spec, 2, delim)
	if !ok {
		return "", "", "", false
	}
	replacement, next, ok = readDelimited(spec, next, delim)
	if !ok {
		return "", "", "", false
	}
	return pattern, replacement, spec[next:], true
}

func readDelimited(s string, start int, delim byte) (string, int, bool) {
	var out strings.Builder
	for i := start; i < len(s); i++ {
		ch := s[i]
		if ch == delim {
			return out.String(), i + 1, true
		}
		if ch == '\\' {
			if i+1 >= len(s) {
				return "", 0, false
			}
			next := s[i+1]
			if next == delim || next == '\\' {
				out.WriteByte(next)
			} else {
				out.WriteByte('\\')
				out.WriteByte(next)
			}
			i++
			continue
		}
		out.WriteByte(ch)
	}
	return "", 0, false
}

// Toast keeps a process-wide pattern cache (pcre_pattern_cache, pcre_moo.cc)
// filled by get_pcre (pcre_moo.cc:59-125), which pcre_match() and SQLite
// REGEXP call; pcre_replace() compiles through pcrs and never touches it.
// Barn compiles through cachedPCREPattern; pcreCache is Toast's ledger, which
// pcre_cache_stats() reports. An entry is keyed by pattern and case option.
// A hit adds one to the entry's unsigned int hit count. A miss on a full cache
// (PCRE_PATTERN_CACHE_SIZE, options.h:277) first evicts the first entry, in
// iteration order, with the fewest hits; the new entry is then added with 0
// hits only if the pattern compiled.
//
// Toast iterates a libstdc++ unordered_map, which links a node inserted into an
// empty bucket at the front of the list, so while no two patterns share a
// bucket it iterates newest first. Barn keeps newest first; Toast's order after
// bucket collisions and rehashes is not reproduced.
const pcrePatternCacheSize = 20

type pcreCacheEntry struct {
	pattern  string
	caseless bool
	hits     uint32
}

var pcreCache struct {
	mu      sync.Mutex
	entries []pcreCacheEntry // newest first
}

// recordPCRECacheLookup records one get_pcre call for pattern; compiled
// reports whether the pattern compiled.
func recordPCRECacheLookup(pattern string, caseSensitive, compiled bool) {
	caseless := !caseSensitive
	pcreCache.mu.Lock()
	defer pcreCache.mu.Unlock()
	entries := pcreCache.entries
	for i := range entries {
		if entries[i].pattern == pattern && entries[i].caseless == caseless {
			entries[i].hits++
			return
		}
	}
	if len(entries) >= pcrePatternCacheSize {
		victim := 0
		for i := range entries {
			if entries[i].hits < entries[victim].hits {
				victim = i
			}
		}
		entries = append(entries[:victim], entries[victim+1:]...)
	}
	if compiled {
		entries = append(entries, pcreCacheEntry{})
		copy(entries[1:], entries)
		entries[0] = pcreCacheEntry{pattern: pattern, caseless: caseless}
	}
	pcreCache.entries = entries
}

func builtinPcreCacheStats(ctx *Execution, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	pcreCache.mu.Lock()
	out := make([]types.Value, len(pcreCache.entries))
	for i, e := range pcreCache.entries {
		out[i] = types.NewList([]types.Value{types.NewStr(e.pattern), types.NewInt(int64(e.hits))})
	}
	pcreCache.mu.Unlock()
	return types.Ok(types.NewList(out))
}

func pcreDescriptors() []Descriptor {
	return []Descriptor{
		function("pcre_cache_stats", builtinPcreCacheStats, 0, 0),
		function("pcre_match", builtinPcreMatch, 2, 4, 2, 2, argFlag, argFlag),
		function("pcre_replace", builtinPcreReplace, 2, 2, 2, 2),
	}
}
