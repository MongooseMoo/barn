package builtins

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/MongooseMoo/barn/types"
	"github.com/dlclark/regexp2"
)

func builtinPcreMatch(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}
	subject := args[0]
	pattern := args[1]
	ok1 := subject.Type() == types.TYPE_STR
	ok2 := pattern.Type() == types.TYPE_STR
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}
	// An empty subject always yields no matches (Toast returns {}), even for
	// patterns that match the empty string like ".*" or "^$". Toast's match loop
	// is `while (offset < subject_length)` (toaststunt/src/pcre_moo.cc:208); with
	// subject_length == 0 the loop body never runs and ret stays new_list(0).
	// Corroborated by conformance test pcre_match_empty_subject
	// (moo-conformance-tests .../builtins/pcre.yaml:201-205 -> value: []).
	if subject.Str() == "" {
		return types.Ok(types.NewList([]types.Value{}))
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
	if err != nil {
		return types.Err(types.E_INVARG)
	}

	text := subject.Str()
	offsets := pcreByteOffsets(text)
	out := make([]types.Value, 0)
	m, merr := re.re.FindStringMatch(text)
	for m != nil && merr == nil {
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

// pcreByteOffsets maps regexp2's rune indices back to byte offsets so the
// MOO-visible positions stay Toast's 1-based byte positions. It returns nil for
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

func buildPcreCapture(subject string, offsets []int, g *regexp2.Group) types.Value {
	match := ""
	pos := []types.Value{}
	if g != nil && len(g.Captures) > 0 {
		start, end := g.Index, g.Index+g.Length
		if offsets != nil {
			start, end = offsets[start], offsets[end]
		}
		match = subject[start:end]
		// 1-based inclusive positions.
		pos = []types.Value{types.NewInt(int64(start + 1)), types.NewInt(int64(end))}
	}
	return types.NewMap([][2]types.Value{
		{types.NewStr("match"), types.NewStr(match)},
		{types.NewStr("position"), types.NewList(pos)},
	})
}

func builtinPcreReplace(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
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

	replacement = normalizePcreReplacement(replacement)

	count := 1
	if global {
		count = -1
	}
	out, rerr := re.re.Replace(subject.Str(), replacement, -1, count)
	if rerr != nil {
		return types.Err(types.E_INVARG)
	}
	if ctx == nil || ctx.Session == nil {
		return types.Err(types.E_INVARG)
	}
	if errCode := ctx.Session.CheckStringLimit(out); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	return types.Ok(types.NewStr(out))
}

func normalizePcreReplacement(replacement string) string {
	// MOO PCRE replacement supports $& for whole-match; Go uses $0.
	return strings.ReplaceAll(replacement, "$&", "$0")
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

func builtinPcreCacheStats(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	return types.Ok(types.NewList([]types.Value{types.NewInt(0), types.NewInt(0)}))
}
