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

	switch args[0].Kind() {
	case types.KindStr:
		// Return raw string length (like C strlen) - do NOT decode ~XX escapes
		return types.Ok(types.NewInt(int64(len(args[0].Str()))))
	case types.KindList:
		return types.Ok(types.NewInt(int64(args[0].List().Len())))
	case types.KindMap:
		return types.Ok(types.NewInt(int64(args[0].Map().Len())))
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

	subject, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	old, ok := args[1].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	new, ok := args[2].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Empty old string is invalid
	if old == "" {
		return types.Err(types.E_INVARG)
	}

	caseSensitive := false
	if len(args) == 4 {
		caseSensitive = args[3].Truthy()
	}

	subj := subject
	oldStr := old
	newStr := new

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

	haystack, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	needle, ok := args[1].AsStr()
	if !ok {
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
		offsetVal, ok := args[3].AsInt()
		if !ok {
			return types.Err(types.E_TYPE)
		}
		offset = int(offsetVal)
		// Negative offset is invalid
		if offset < 0 {
			return types.Err(types.E_INVARG)
		}
	}

	h := haystack
	n := needle

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

	haystack, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	needle, ok := args[1].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	caseSensitive := false
	if len(args) >= 3 {
		caseSensitive = args[2].Truthy()
	}

	h := haystack
	n := needle

	// Convert to runes
	hRunes := []rune(h)
	nRunes := []rune(n)

	// Handle offset (4th argument)
	// offset <= 0: specifies search end position (length + offset)
	// offset > 0: invalid
	endPos := len(hRunes) // Default: search whole string
	if len(args) == 4 {
		offsetVal, ok := args[3].AsInt()
		if !ok {
			return types.Err(types.E_TYPE)
		}
		offset := int(offsetVal)
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

	str1, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	str2, ok := args[1].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	cmp := strings.Compare(str1, str2)
	return types.Ok(types.NewInt(int64(cmp)))
}

// builtinUpcase converts string to uppercase
// upcase(str) -> str
func builtinUpcase(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewStr(strings.ToUpper(str)))
}

// builtinDowncase converts string to lowercase
// downcase(str) -> str
func builtinDowncase(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewStr(strings.ToLower(str)))
}

// builtinCapitalize capitalizes first letter of each word
// capitalize(str) -> str
func builtinCapitalize(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewStr(strings.Title(str)))
}

// builtinExplode splits a string into a list of substrings
// explode(str [, delimiter [, adjacent]]) -> list
func builtinExplode(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	s := str

	delim := " "
	if len(args) >= 2 {
		delimVal, ok := args[1].AsStr()
		if !ok {
			return types.Err(types.E_TYPE)
		}
		if delimVal != "" {
			delim = string([]byte{delimVal[0]})
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

	list, ok := args[0].AsList()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	delimiter := ""
	if len(args) == 2 {
		delim, ok := args[1].AsStr()
		if !ok {
			return types.Err(types.E_TYPE)
		}
		delimiter = delim
	}

	// Convert list elements to strings
	parts := make([]string, list.Len())
	for i := 1; i <= list.Len(); i++ {
		elem := list.Get(i)
		str, ok := elem.AsStr()
		if !ok {
			return types.Err(types.E_TYPE)
		}
		parts[i-1] = str
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

	str, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	s := str
	if len(args) == 1 {
		// Trim whitespace
		return types.Ok(types.NewStr(strings.TrimSpace(s)))
	}

	// Trim specific characters
	chars, ok := args[1].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewStr(strings.Trim(s, chars)))
}

// builtinLtrim removes leading characters
// ltrim(str [, chars]) -> str
func builtinLtrim(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	s := str
	if len(args) == 1 {
		// Trim whitespace
		return types.Ok(types.NewStr(strings.TrimLeftFunc(s, unicode.IsSpace)))
	}

	// Trim specific characters
	chars, ok := args[1].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewStr(strings.TrimLeft(s, chars)))
}

// builtinRtrim removes trailing characters
// rtrim(str [, chars]) -> str
func builtinRtrim(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	s := str
	if len(args) == 1 {
		// Trim whitespace
		return types.Ok(types.NewStr(strings.TrimRightFunc(s, unicode.IsSpace)))
	}

	// Trim specific characters
	chars, ok := args[1].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewStr(strings.TrimRight(s, chars)))
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

	str, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	from, ok := args[1].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	to, ok := args[2].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	caseSensitive := false
	if len(args) == 4 {
		caseSensitive = args[3].Truthy()
	}

	s := str
	fromRunes := []rune(from)
	toRunes := []rune(to)

	// Empty from string - return unchanged
	if len(fromRunes) == 0 {
		return types.Ok(types.NewStr(str))
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

	subjectVal, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	subject := subjectVal

	patternVal, ok := args[1].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	pattern := patternVal

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

	subjectVal, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	subject := subjectVal

	patternVal, ok := args[1].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	pattern := patternVal

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

	templateVal, ok := args[0].AsStr()
	if !ok {
		return types.Err(types.E_TYPE)
	}
	template := templateVal

	matchResult, ok := args[1].AsList()
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Match result format: {start, end, subs, subject}
	// If empty list, no match - return template unchanged
	if matchResult.Len() == 0 {
		return types.Ok(types.NewStr(templateVal))
	}

	// Match result must be {start, end, subs, subject}.
	if matchResult.Len() < 4 {
		return types.Err(types.E_INVARG)
	}

	startVal, ok := matchResult.Get(1).AsInt()
	if !ok {
		return types.Err(types.E_INVARG)
	}
	endVal, ok := matchResult.Get(2).AsInt()
	if !ok {
		return types.Err(types.E_INVARG)
	}

	subs, ok := matchResult.Get(3).AsList()
	if !ok {
		return types.Err(types.E_INVARG)
	}

	subject, ok := matchResult.Get(4).AsStr()
	if !ok {
		return types.Err(types.E_INVARG)
	}

	subjectText := subject
	extract := func(start, end int) string {
		if start <= 0 || end < 0 || start-1 > len(subjectText) || end > len(subjectText) || start-1 > end {
			return ""
		}
		return subjectText[start-1 : end]
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
					result.WriteString(extract(int(startVal), int(endVal)))
				} else {
					if groupNum <= subs.Len() {
						groupRange, ok := subs.Get(groupNum).AsList()
						if !ok || groupRange.Len() < 2 {
							return types.Err(types.E_INVARG)
						}
						gStart, ok := groupRange.Get(1).AsInt()
						if !ok {
							return types.Err(types.E_INVARG)
						}
						gEnd, ok := groupRange.Get(2).AsInt()
						if !ok {
							return types.Err(types.E_INVARG)
						}
						result.WriteString(extract(int(gStart), int(gEnd)))
					}
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
			switch pattern[i+1] {
			case 'd': // digit
				result.WriteString("[0-9]")
			case 'D': // non-digit
				result.WriteString("[^0-9]")
			case 'w': // word character
				result.WriteString("[a-zA-Z0-9_]")
			case 'W': // non-word character
				result.WriteString("[^a-zA-Z0-9_]")
			case 's': // whitespace
				result.WriteString("[ \\t\\n\\r]")
			case 'S': // non-whitespace
				result.WriteString("[^ \\t\\n\\r]")
			case '%': // literal %
				result.WriteByte('%')
			case '(': // start capture group
				result.WriteByte('(')
			case ')': // end capture group
				result.WriteByte(')')
			case '+': // one or more (non-greedy in MOO)
				result.WriteString("+?")
			case '*': // zero or more (non-greedy in MOO)
				result.WriteString("*?")
			case '?': // zero or one
				result.WriteByte('?')
			case '.': // any char
				result.WriteByte('.')
			case '^': // start of string
				result.WriteByte('^')
			case '$': // end of string
				result.WriteByte('$')
			case '[': // character class start
				result.WriteByte('[')
			case ']': // character class end
				result.WriteByte(']')
			case '|': // alternation
				result.WriteByte('|')
			default:
				// Unknown escape - pass through
				result.WriteByte('%')
				result.WriteByte(pattern[i+1])
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
