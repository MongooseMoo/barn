package builtins

import (
	"regexp"
	"strconv"
	"strings"

	"barn/kernel"
	"barn/types"
)

func builtinPcreMatch(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}
	subject, ok1 := args[0].(types.StrValue)
	pattern, ok2 := args[1].(types.StrValue)
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}
	// An empty subject simply yields no matches (Toast returns {}).
	if subject.Value() == "" {
		return types.Ok(types.NewList([]types.Value{}))
	}
	if pattern.Value() == "" {
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

	pat := pattern.Value()
	if !caseMatters {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return types.Err(types.E_INVARG)
	}

	maxMatches := -1
	if !findAll {
		maxMatches = 1
	}
	matches := re.FindAllStringSubmatchIndex(subject.Value(), maxMatches)
	if len(matches) == 0 {
		return types.Ok(types.NewList([]types.Value{}))
	}

	names := re.SubexpNames()
	out := make([]types.Value, 0, len(matches))
	for _, loc := range matches {
		entryPairs := make([][2]types.Value, 0, len(names)+1)
		entryPairs = append(entryPairs, [2]types.Value{
			types.NewStr("0"),
			buildPcreCapture(subject.Value(), loc[0], loc[1]),
		})

		for i := 1; i < len(names); i++ {
			gStart := -1
			gEnd := -1
			if i*2+1 < len(loc) {
				gStart = loc[i*2]
				gEnd = loc[i*2+1]
			}
			key := strconv.Itoa(i)
			if names[i] != "" {
				// Named groups use their name instead of numeric key.
				key = names[i]
			}
			entryPairs = append(entryPairs, [2]types.Value{
				types.NewStr(key),
				buildPcreCapture(subject.Value(), gStart, gEnd),
			})
		}
		out = append(out, types.NewMap(entryPairs))
	}

	return types.Ok(types.NewList(out))
}

func buildPcreCapture(subject string, start, end int) types.Value {
	match := ""
	pos := []types.Value{}
	if start >= 0 && end >= start && end <= len(subject) {
		match = subject[start:end]
		// 1-based inclusive positions.
		pos = []types.Value{types.NewInt(int64(start + 1)), types.NewInt(int64(end))}
	}
	return types.NewMap([][2]types.Value{
		{types.NewStr("match"), types.NewStr(match)},
		{types.NewStr("position"), types.NewList(pos)},
	})
}

func builtinPcreReplace(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	subject, ok1 := args[0].(types.StrValue)
	spec, ok2 := args[1].(types.StrValue)
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}

	pattern, replacement, flags, ok := parseSedReplaceSpec(spec.Value())
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

	if caseInsensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return types.Err(types.E_INVARG)
	}

	replacement = normalizePcreReplacement(replacement)

	var out string
	if global {
		out = re.ReplaceAllString(subject.Value(), replacement)
	} else {
		idx := re.FindStringIndex(subject.Value())
		if idx == nil {
			out = subject.Value()
		} else {
			replaced := re.ReplaceAllString(subject.Value()[idx[0]:idx[1]], replacement)
			out = subject.Value()[:idx[0]] + replaced + subject.Value()[idx[1]:]
		}
	}
	if errCode := CheckStringLimit(out); errCode != types.E_NONE {
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

func builtinPcreCacheStats(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	return types.Ok(types.NewList([]types.Value{types.NewInt(0), types.NewInt(0)}))
}
