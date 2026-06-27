package builtins

import (
	"regexp"
	"strings"
	"unicode"

	"barn/kernel"
	"barn/types"
)

// ============================================================================
// LAYER 7.1: STRING BUILTINS
// ============================================================================

// builtinLength returns the length of a string, list, or map
// length(str) -> int
// length(list) -> int
// length(map) -> int
// For strings, returns the raw string length (number of characters), not decoded byte count
func builtinLength(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	switch args[0].Type() {
	case types.TYPE_STR:
		// Return raw string length (like C strlen) - do NOT decode ~XX escapes
		return types.Ok(types.NewInt(int64(len(args[0].Str()))))
	case types.TYPE_LIST:
		return types.Ok(types.NewInt(int64(args[0].Len())))
	case types.TYPE_MAP:
		return types.Ok(types.NewInt(int64(args[0].Len())))
	default:
		return types.Err(types.E_TYPE)
	}
}

// countDecodedBytes counts the number of bytes in a MOO string,
// treating ~XX sequences as single bytes
func countDecodedBytes(s string) int {
	count := 0
	i := 0
	for i < len(s) {
		if i+2 < len(s) && s[i] == '~' {
			// Check if this is a valid ~XX hex escape
			c1, c2 := s[i+1], s[i+2]
			if isHexDigit(c1) && isHexDigit(c2) {
				// ~XX counts as 1 byte
				count++
				i += 3
				continue
			}
		}
		// Regular character counts as 1 byte
		count++
		i++
	}
	return count
}

// isHexDigit returns true if c is a valid hex digit (0-9, A-F, a-f)
func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f')
}

// builtinStrsub replaces all occurrences of old with new in subject
// strsub(subject, old, new [, case_matters]) -> str
func builtinStrsub(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 3 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if args[2].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	subj := args[0].Str()
	oldStr := args[1].Str()
	newStr := args[2].Str()

	// Empty old string is invalid
	if oldStr == "" {
		return types.Err(types.E_INVARG)
	}

	caseSensitive := false
	if len(args) == 4 {
		caseSensitive = args[3].Truthy()
	}

	var result string
	if caseSensitive {
		result = strings.ReplaceAll(subj, oldStr, newStr)
	} else {
		// Case-insensitive replacement
		result = replaceAllCaseInsensitive(subj, oldStr, newStr)
	}

	// Check string length limit (update from load_server_options cache first)
	UpdateContextLimits(ctx)
	if errCode := ctx.CheckStringLimit(len(result)); errCode != types.E_NONE {
		return types.Err(errCode)
	}

	return types.Ok(types.NewStr(result))
}

// builtinIndex finds the first occurrence of needle in haystack
// index(haystack, needle [, case_matters [, start]]) -> int
func builtinIndex(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	caseSensitive := false
	if len(args) >= 3 {
		caseSensitive = args[2].Truthy()
	}

	// The 4th argument is an offset that:
	// 1. Shifts the start position (search from offset+1)
	// 2. Adjusts the returned position (result - offset)
	offset := 0
	if len(args) == 4 {
		if args[3].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		offset = int(args[3].Int())
		// Negative offset is invalid
		if offset < 0 {
			return types.Err(types.E_INVARG)
		}
	}

	h := args[0].Str()
	n := args[1].Str()

	// Convert to runes for proper indexing
	hRunes := []rune(h)
	nRunes := []rune(n)

	// Start searching from position (offset + 1) in 1-based terms
	// which is offset in 0-based terms
	startIdx := offset

	if startIdx >= len(hRunes) {
		return types.Ok(types.NewInt(0))
	}

	// Search
	for i := startIdx; i <= len(hRunes)-len(nRunes); i++ {
		match := true
		for j := 0; j < len(nRunes); j++ {
			hChar := hRunes[i+j]
			nChar := nRunes[j]
			if caseSensitive {
				if hChar != nChar {
					match = false
					break
				}
			} else {
				if unicode.ToLower(hChar) != unicode.ToLower(nChar) {
					match = false
					break
				}
			}
		}
		if match {
			// Return position adjusted by offset
			// i is 0-based, so actual position is i+1
			// Result is (i+1) - offset
			result := int64(i + 1 - offset)
			if result <= 0 {
				return types.Ok(types.NewInt(0))
			}
			return types.Ok(types.NewInt(result))
		}
	}

	return types.Ok(types.NewInt(0))
}

// builtinRindex finds the last occurrence of needle in haystack
// rindex(haystack, needle [, case_matters [, offset]]) -> int
// offset is 0 or negative; specifies end position for search (from end of string)
func builtinRindex(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	caseSensitive := false
	if len(args) >= 3 {
		caseSensitive = args[2].Truthy()
	}

	h := args[0].Str()
	n := args[1].Str()

	// Convert to runes
	hRunes := []rune(h)
	nRunes := []rune(n)

	// Handle offset (4th argument)
	// offset <= 0: specifies search end position (length + offset)
	// offset > 0: invalid
	endPos := len(hRunes) // Default: search whole string
	if len(args) == 4 {
		if args[3].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		offset := int(args[3].Int())
		if offset > 0 {
			return types.Err(types.E_INVARG)
		}
		// offset is 0 or negative
		endPos = len(hRunes) + offset
		if endPos < 0 {
			return types.Ok(types.NewInt(0))
		}
	}

	// Search backwards from endPos
	startSearch := endPos - len(nRunes)
	if startSearch < 0 {
		startSearch = 0
	}
	if startSearch > len(hRunes)-len(nRunes) {
		startSearch = len(hRunes) - len(nRunes)
	}

	for i := startSearch; i >= 0; i-- {
		match := true
		for j := 0; j < len(nRunes); j++ {
			hChar := hRunes[i+j]
			nChar := nRunes[j]
			if caseSensitive {
				if hChar != nChar {
					match = false
					break
				}
			} else {
				if unicode.ToLower(hChar) != unicode.ToLower(nChar) {
					match = false
					break
				}
			}
		}
		if match {
			return types.Ok(types.NewInt(int64(i + 1))) // 1-based
		}
	}

	return types.Ok(types.NewInt(0))
}

// builtinStrcmp compares two strings lexicographically (case-sensitive)
// strcmp(str1, str2) -> int (negative, zero, or positive)
func builtinStrcmp(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	cmp := strings.Compare(args[0].Str(), args[1].Str())
	return types.Ok(types.NewInt(int64(cmp)))
}

// builtinUpcase converts string to uppercase
// upcase(str) -> str
func builtinUpcase(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewStr(strings.ToUpper(args[0].Str())))
}

// builtinDowncase converts string to lowercase
// downcase(str) -> str
func builtinDowncase(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewStr(strings.ToLower(args[0].Str())))
}

// builtinCapitalize uppercases only the first character of the string,
// leaving the rest unchanged. This matches the MOO library verb
// $string_utils:capitalize ("string with first letter capitalized").
// capitalize is not a ToastStunt C++ builtin; it is Barn-only.
// capitalize(str) -> str
func builtinCapitalize(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	s := args[0].Str()
	if s == "" {
		return types.Ok(types.NewStr(""))
	}
	// Uppercase only the first character (byte-indexed, matching MOO's
	// string[1] semantics), leaving the remainder of the string intact.
	return types.Ok(types.NewStr(strings.ToUpper(s[:1]) + s[1:]))
}

// builtinExplode splits a string into a list of substrings
// explode(str [, delimiter [, adjacent]]) -> list
func builtinExplode(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	s := args[0].Str()

	delim := " "
	if len(args) >= 2 {
		if args[1].Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		if args[1].Str() != "" {
			delim = string([]byte{args[1].Str()[0]})
		}
	}

	adjacent := false
	if len(args) == 3 {
		adjacent = args[2].Truthy()
	}

	rawParts := strings.Split(s, delim)
	parts := rawParts
	if !adjacent {
		parts = make([]string, 0, len(rawParts))
		for _, part := range rawParts {
			if part != "" {
				parts = append(parts, part)
			}
		}
	}

	// Convert to list of string values
	values := make([]types.Value, len(parts))
	for i, part := range parts {
		values[i] = types.NewStr(part)
	}

	return types.Ok(types.NewList(values))
}

// builtinImplode joins a list of strings into a single string
// implode(list [, delimiter]) -> str
func builtinImplode(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[0]

	delimiter := ""
	if len(args) == 2 {
		if args[1].Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		delimiter = args[1].Str()
	}

	// Convert list elements to strings
	parts := make([]string, list.Len())
	for i := 1; i <= list.Len(); i++ {
		elem := list.Get(i)
		if elem.Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		parts[i-1] = elem.Str()
	}

	result := strings.Join(parts, delimiter)

	// Check string limit
	if err := CheckStringLimit(result); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(types.NewStr(result))
}

// builtinTrim removes leading and trailing characters
// trim(str [, chars]) -> str
func builtinTrim(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	s := args[0].Str()
	if len(args) == 1 {
		// Trim whitespace
		return types.Ok(types.NewStr(strings.TrimSpace(s)))
	}

	// Trim specific characters
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewStr(strings.Trim(s, args[1].Str())))
}

// builtinLtrim removes leading characters
// ltrim(str [, chars]) -> str
func builtinLtrim(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	s := args[0].Str()
	if len(args) == 1 {
		// Trim whitespace
		return types.Ok(types.NewStr(strings.TrimLeftFunc(s, unicode.IsSpace)))
	}

	// Trim specific characters
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewStr(strings.TrimLeft(s, args[1].Str())))
}

// builtinRtrim removes trailing characters
// rtrim(str [, chars]) -> str
func builtinRtrim(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	s := args[0].Str()
	if len(args) == 1 {
		// Trim whitespace
		return types.Ok(types.NewStr(strings.TrimRightFunc(s, unicode.IsSpace)))
	}

	// Trim specific characters
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewStr(strings.TrimRight(s, args[1].Str())))
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// builtinStrtr translates characters in a string
// strtr(str, from, to [, case_matters]) -> str
func builtinStrtr(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 3 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if args[2].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	caseSensitive := false
	if len(args) == 4 {
		caseSensitive = args[3].Truthy()
	}

	s := args[0].Str()
	fromRunes := []rune(args[1].Str())
	toRunes := []rune(args[2].Str())

	// Empty from string - return unchanged
	if len(fromRunes) == 0 {
		return types.Ok(args[0])
	}

	// Build translation map
	// If to is shorter than from, extra chars in from are DELETED
	// If to is longer than from, ignore extra chars in to
	// If duplicate chars in from, LAST occurrence wins
	var result []rune
	for _, ch := range s {
		// Find the LAST matching character in from (duplicates: last wins)
		matchIdx := -1
		for i, fc := range fromRunes {
			var match bool
			if caseSensitive {
				match = ch == fc
			} else {
				match = unicode.ToLower(ch) == unicode.ToLower(fc)
			}
			if match {
				matchIdx = i // Keep updating to get the last match
			}
		}

		if matchIdx >= 0 {
			// Get replacement character
			if matchIdx < len(toRunes) {
				replacement := toRunes[matchIdx]

				// Case-insensitive: preserve original case
				if !caseSensitive {
					if unicode.IsUpper(ch) {
						replacement = unicode.ToUpper(replacement)
					} else if unicode.IsLower(ch) {
						replacement = unicode.ToLower(replacement)
					}
				}

				result = append(result, replacement)
			}
			// If matchIdx >= len(toRunes), the character is deleted
		} else {
			result = append(result, ch)
		}
	}

	return types.Ok(types.NewStr(string(result)))
}

// replaceAllCaseInsensitive performs case-insensitive string replacement
func replaceAllCaseInsensitive(s, old, new string) string {
	// Convert to runes for proper character handling
	sRunes := []rune(s)
	oldRunes := []rune(old)

	if len(oldRunes) == 0 {
		return s
	}

	var result []rune
	i := 0
	for i < len(sRunes) {
		// Check if we have a match at current position
		if i+len(oldRunes) <= len(sRunes) {
			match := true
			for j := 0; j < len(oldRunes); j++ {
				if unicode.ToLower(sRunes[i+j]) != unicode.ToLower(oldRunes[j]) {
					match = false
					break
				}
			}
			if match {
				// Found a match - add replacement
				result = append(result, []rune(new)...)
				i += len(oldRunes)
				continue
			}
		}
		// No match - add current character
		result = append(result, sRunes[i])
		i++
	}

	return string(result)
}

// ============================================================================
// LAYER 8.1: REGEX BUILTINS
// ============================================================================

// builtinMatch implements match(subject, pattern [, case_matters]) -> list
// MOO-style regex matching. Returns {start, end, subs, subject} or {} if no match.
// For now, implements a simplified version that handles basic patterns.
func builtinMatch(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	subject := args[0].Str()

	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	pattern := args[1].Str()

	// Case-insensitive by default; truthy third argument enables case-sensitive matching.
	caseSensitive := false
	if len(args) > 2 {
		caseSensitive = args[2].Truthy()
	}

	// Convert MOO pattern to Go regex
	goPattern, err := mooPatternToGoRegex(pattern)
	if err != nil {
		return types.Err(types.E_INVARG)
	}

	pat := goPattern
	if !caseSensitive {
		pat = "(?i)" + goPattern
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return types.Err(types.E_INVARG)
	}

	loc := re.FindStringSubmatchIndex(subject)
	if loc == nil {
		// No match - return empty list
		return types.Ok(types.NewList([]types.Value{}))
	}

	return types.Ok(buildMatchResult(subject, loc))
}

// builtinRmatch implements rmatch(subject, pattern [, case_matters]) -> list
// Like match but finds the last occurrence.
func builtinRmatch(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	subject := args[0].Str()

	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	pattern := args[1].Str()

	// Case-insensitive by default; truthy third argument enables case-sensitive matching.
	caseSensitive := false
	if len(args) > 2 {
		caseSensitive = args[2].Truthy()
	}

	// Convert MOO pattern to Go regex
	goPattern, err := mooPatternToGoRegex(pattern)
	if err != nil {
		return types.Err(types.E_INVARG)
	}

	pat := goPattern
	if !caseSensitive {
		pat = "(?i)" + goPattern
	}
	re, err := regexp.Compile("^(?:" + pat + ")")
	if err != nil {
		return types.Err(types.E_INVARG)
	}

	best := []int(nil)
	for i := 0; i <= len(subject); i++ {
		loc := re.FindStringSubmatchIndex(subject[i:])
		if loc == nil {
			continue
		}
		best = make([]int, len(loc))
		for j, idx := range loc {
			if idx < 0 {
				best[j] = -1
			} else {
				best[j] = idx + i
			}
		}
	}
	if best == nil {
		return types.Ok(types.NewList([]types.Value{}))
	}

	return types.Ok(buildMatchResult(subject, best))
}

// builtinSubstitute implements substitute(template, match_result) -> str
// Substitutes captured groups from match result into template.
// Template syntax: %1, %2, etc. for captured groups, %% for literal %
func builtinSubstitute(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	template := args[0].Str()

	if args[1].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	matchResult := args[1]

	// subs must be a well-formed match() result: exactly {start, end, groups, subject}
	// where groups is a list of exactly nine {start, end} marker pairs. ToastStunt
	// raises E_INVARG for malformed match data rather than best-effort substituting.
	if matchResult.Len() != 4 {
		return types.Err(types.E_INVARG)
	}

	if matchResult.Get(1).Type() != types.TYPE_INT {
		return types.Err(types.E_INVARG)
	}
	startVal := matchResult.Get(1).Int()
	if matchResult.Get(2).Type() != types.TYPE_INT {
		return types.Err(types.E_INVARG)
	}
	endVal := matchResult.Get(2).Int()

	subs := matchResult.Get(3)
	if subs.Type() != types.TYPE_LIST || subs.Len() != 9 {
		return types.Err(types.E_INVARG)
	}

	if matchResult.Get(4).Type() != types.TYPE_STR {
		return types.Err(types.E_INVARG)
	}
	subjectText := matchResult.Get(4).Str()
	// extract returns the substring for a {start, end} marker. An empty range
	// (end < start, e.g. the {0, -1} unmatched-group marker) yields "". A non-empty
	// range that falls outside the subject is invalid -> E_INVARG (ok=false).
	extract := func(start, end int) (string, bool) {
		if end < start {
			return "", true
		}
		if start < 1 || end > len(subjectText) {
			return "", false
		}
		return subjectText[start-1 : end], true
	}

	// Process template and substitute %N with captured groups.
	var result strings.Builder
	i := 0
	for i < len(template) {
		if template[i] == '%' && i+1 < len(template) {
			if template[i+1] == '%' {
				// %% -> literal %
				result.WriteByte('%')
				i += 2
			} else if template[i+1] >= '0' && template[i+1] <= '9' {
				// %N -> captured group N
				groupNum := int(template[i+1] - '0')
				if groupNum == 0 {
					s, inRange := extract(int(startVal), int(endVal))
					if !inRange {
						return types.Err(types.E_INVARG)
					}
					result.WriteString(s)
				} else {
					groupRange := subs.Get(groupNum)
					if groupRange.Type() != types.TYPE_LIST || groupRange.Len() < 2 {
						return types.Err(types.E_INVARG)
					}
					if groupRange.Get(1).Type() != types.TYPE_INT {
						return types.Err(types.E_INVARG)
					}
					gStart := groupRange.Get(1).Int()
					if groupRange.Get(2).Type() != types.TYPE_INT {
						return types.Err(types.E_INVARG)
					}
					gEnd := groupRange.Get(2).Int()
					s, inRange := extract(int(gStart), int(gEnd))
					if !inRange {
						return types.Err(types.E_INVARG)
					}
					result.WriteString(s)
				}
				i += 2
			} else {
				// Any other % escape is invalid.
				return types.Err(types.E_INVARG)
			}
		} else {
			result.WriteByte(template[i])
			i++
		}
	}

	resultStr := result.String()

	// Check string limit
	if err := CheckStringLimit(resultStr); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(types.NewStr(resultStr))
}

func buildMatchResult(subject string, loc []int) types.Value {
	start := types.NewInt(int64(loc[0] + 1))
	end := types.NewInt(int64(loc[1]))
	subs := make([]types.Value, 9)
	for i := 0; i < 9; i++ {
		subStart := int64(0)
		subEnd := int64(-1)
		subIdx := i + 1
		if subIdx*2+1 < len(loc) && loc[subIdx*2] >= 0 {
			subStart = int64(loc[subIdx*2] + 1)
			subEnd = int64(loc[subIdx*2+1])
		}
		subs[i] = types.NewList([]types.Value{types.NewInt(subStart), types.NewInt(subEnd)})
	}
	return types.NewList([]types.Value{
		start,
		end,
		types.NewList(subs),
		types.NewStr(subject),
	})
}

// mooPatternToGoRegex converts MOO regex patterns to Go regex
// MOO uses %d for digits, %w for word chars, %s for spaces, etc.
func mooPatternToGoRegex(pattern string) (string, error) {
	var result strings.Builder
	i := 0
	for i < len(pattern) {
		if pattern[i] == '%' && i+1 < len(pattern) {
			// MOO regex (ToastStunt's Spencer "rx") uses '%' as the escape
			// character. Only a small set of '% + char' sequences are special;
			// EVERY other one is the literal following character. In particular
			// there are NO Perl-style %d/%s classes: "%d" matches a literal 'd'.
			c := pattern[i+1]
			switch c {
			case 'w': // word constituent
				result.WriteString("[a-zA-Z0-9_]")
			case 'W': // non-word constituent
				result.WriteString("[^a-zA-Z0-9_]")
			case 'b': // word boundary
				result.WriteString("\\b")
			case 'B': // not a word boundary
				result.WriteString("\\B")
			case '<': // beginning of a word (best RE2 approximation)
				result.WriteString("\\b")
			case '>': // end of a word (best RE2 approximation)
				result.WriteString("\\b")
			case '(': // start capture group
				result.WriteByte('(')
			case ')': // end capture group
				result.WriteByte(')')
			case '|': // alternation
				result.WriteByte('|')
			case '1', '2', '3', '4', '5', '6', '7', '8', '9':
				// Backreference: RE2 cannot express these. Preserve the original
				// bytes rather than silently treating the digit as a literal.
				result.WriteByte('%')
				result.WriteByte(c)
			default:
				// Any other escaped char is that literal character.
				result.WriteString(regexp.QuoteMeta(string(c)))
			}
			i += 2
		} else {
			// Regular character - escape if special in Go regex
			// EXCEPT chars that have the same meaning in MOO and Go regex:
			// ^ $ [ ] are passed through (anchors and character classes)
			// . + * ? also pass through (they work the same in MOO and Go)
			c := pattern[i]
			if strings.ContainsRune("{}()|\\", rune(c)) {
				result.WriteByte('\\')
			}
			result.WriteByte(c)
			i++
		}
	}
	return result.String(), nil
}
