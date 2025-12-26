package builtins

import (
	"barn/types"
	"strings"
	"unicode"
)

// ============================================================================
// LAYER 7.1: STRING BUILTINS
// ============================================================================

// builtinLength returns the length of a string, list, or map
// length(str) -> int
// length(list) -> int
// length(map) -> int
// For strings with binary escapes (~XX), counts decoded bytes, not encoded length
func builtinLength(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	switch v := args[0].(type) {
	case types.StrValue:
		// Count bytes, decoding ~XX binary escapes
		return types.Ok(types.IntValue{Val: int64(countDecodedBytes(v.Value()))})
	case types.ListValue:
		return types.Ok(types.IntValue{Val: int64(v.Len())})
	case types.MapValue:
		return types.Ok(types.IntValue{Val: int64(v.Len())})
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
func builtinStrsub(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 3 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	subject, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	old, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	new, ok := args[2].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Empty old string is invalid
	if old.Value() == "" {
		return types.Err(types.E_INVARG)
	}

	caseSensitive := false
	if len(args) == 4 {
		caseSensitive = args[3].Truthy()
	}

	subj := subject.Value()
	oldStr := old.Value()
	newStr := new.Value()

	var result string
	if caseSensitive {
		result = strings.ReplaceAll(subj, oldStr, newStr)
	} else {
		// Case-insensitive replacement
		result = replaceAllCaseInsensitive(subj, oldStr, newStr)
	}

	return types.Ok(types.NewStr(result))
}

// builtinIndex finds the first occurrence of needle in haystack
// index(haystack, needle [, case_matters [, start]]) -> int
func builtinIndex(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	haystack, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	needle, ok := args[1].(types.StrValue)
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
		offsetVal, ok := args[3].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		offset = int(offsetVal.Val)
		// Negative offset is invalid
		if offset < 0 {
			return types.Err(types.E_INVARG)
		}
	}

	h := haystack.Value()
	n := needle.Value()

	// Convert to runes for proper indexing
	hRunes := []rune(h)
	nRunes := []rune(n)

	// Start searching from position (offset + 1) in 1-based terms
	// which is offset in 0-based terms
	startIdx := offset

	if startIdx >= len(hRunes) {
		return types.Ok(types.IntValue{Val: 0})
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
				return types.Ok(types.IntValue{Val: 0})
			}
			return types.Ok(types.IntValue{Val: result})
		}
	}

	return types.Ok(types.IntValue{Val: 0})
}

// builtinRindex finds the last occurrence of needle in haystack
// rindex(haystack, needle [, case_matters [, offset]]) -> int
// offset is 0 or negative; specifies end position for search (from end of string)
func builtinRindex(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	haystack, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	needle, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	caseSensitive := false
	if len(args) >= 3 {
		caseSensitive = args[2].Truthy()
	}

	h := haystack.Value()
	n := needle.Value()

	// Convert to runes
	hRunes := []rune(h)
	nRunes := []rune(n)

	// Handle offset (4th argument)
	// offset <= 0: specifies search end position (length + offset)
	// offset > 0: invalid
	endPos := len(hRunes) // Default: search whole string
	if len(args) == 4 {
		offsetVal, ok := args[3].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		offset := int(offsetVal.Val)
		if offset > 0 {
			return types.Err(types.E_INVARG)
		}
		// offset is 0 or negative
		endPos = len(hRunes) + offset
		if endPos < 0 {
			return types.Ok(types.IntValue{Val: 0})
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
			return types.Ok(types.IntValue{Val: int64(i + 1)}) // 1-based
		}
	}

	return types.Ok(types.IntValue{Val: 0})
}

// builtinStrcmp compares two strings lexicographically (case-sensitive)
// strcmp(str1, str2) -> int (negative, zero, or positive)
func builtinStrcmp(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}

	str1, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	str2, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	cmp := strings.Compare(str1.Value(), str2.Value())
	return types.Ok(types.IntValue{Val: int64(cmp)})
}

// builtinUpcase converts string to uppercase
// upcase(str) -> str
func builtinUpcase(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewStr(strings.ToUpper(str.Value())))
}

// builtinDowncase converts string to lowercase
// downcase(str) -> str
func builtinDowncase(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewStr(strings.ToLower(str.Value())))
}

// builtinCapitalize capitalizes first letter of each word
// capitalize(str) -> str
func builtinCapitalize(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewStr(strings.Title(str.Value())))
}

// builtinExplode splits a string into a list of substrings
// explode(str [, delimiter]) -> list
func builtinExplode(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	s := str.Value()

	var parts []string
	if len(args) == 1 {
		// No delimiter - split on whitespace
		parts = strings.Fields(s)
	} else {
		// Delimiter provided
		delim, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		parts = strings.Split(s, delim.Value())
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
func builtinImplode(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	list, ok := args[0].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	delimiter := ""
	if len(args) == 2 {
		delim, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		delimiter = delim.Value()
	}

	// Convert list elements to strings
	parts := make([]string, list.Len())
	for i := 1; i <= list.Len(); i++ {
		elem := list.Get(i)
		str, ok := elem.(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		parts[i-1] = str.Value()
	}

	return types.Ok(types.NewStr(strings.Join(parts, delimiter)))
}

// builtinTrim removes leading and trailing characters
// trim(str [, chars]) -> str
func builtinTrim(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	s := str.Value()
	if len(args) == 1 {
		// Trim whitespace
		return types.Ok(types.NewStr(strings.TrimSpace(s)))
	}

	// Trim specific characters
	chars, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewStr(strings.Trim(s, chars.Value())))
}

// builtinLtrim removes leading characters
// ltrim(str [, chars]) -> str
func builtinLtrim(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	s := str.Value()
	if len(args) == 1 {
		// Trim whitespace
		return types.Ok(types.NewStr(strings.TrimLeftFunc(s, unicode.IsSpace)))
	}

	// Trim specific characters
	chars, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewStr(strings.TrimLeft(s, chars.Value())))
}

// builtinRtrim removes trailing characters
// rtrim(str [, chars]) -> str
func builtinRtrim(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	s := str.Value()
	if len(args) == 1 {
		// Trim whitespace
		return types.Ok(types.NewStr(strings.TrimRightFunc(s, unicode.IsSpace)))
	}

	// Trim specific characters
	chars, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewStr(strings.TrimRight(s, chars.Value())))
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// builtinStrtr translates characters in a string
// strtr(str, from, to [, case_matters]) -> str
func builtinStrtr(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 3 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}

	str, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	from, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	to, ok := args[2].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	caseSensitive := false
	if len(args) == 4 {
		caseSensitive = args[3].Truthy()
	}

	s := str.Value()
	fromRunes := []rune(from.Value())
	toRunes := []rune(to.Value())

	// Empty from string - return unchanged
	if len(fromRunes) == 0 {
		return types.Ok(str)
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
