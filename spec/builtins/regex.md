# MOO Regular Expression Built-ins

## Overview

PCRE (Perl-Compatible Regular Expressions) functions in ToastStunt. These functions are **optional** and only available if Toast was compiled with PCRE support.

**Check availability:** `; return function_info("pcre_match");`

---

## PCRE Functions in Toast

Toast provides exactly 3 PCRE functions:
1. `pcre_match` - Pattern matching with capture groups
2. `pcre_replace` - Text replacement using Perl s/// syntax
3. `pcre_cache_stats` - Cache statistics (wizard-only)

---

## 1. pcre_match

### Signature

```
pcre_match(subject, pattern [, case_matters [, find_all]]) → LIST
```

### Arguments

| Argument | Type | Description |
|----------|------|-------------|
| subject | STR | String to search |
| pattern | STR | PCRE regular expression |
| case_matters | INT | Optional: 0 = case insensitive (default), 1 = case sensitive |
| find_all | INT | Optional: non-zero = find all matches (default), 0 = find first only |

### Return Value

Returns a **LIST of MAPS** where each map represents one match occurrence.

Each map contains capture groups as keys:
- `"0"` - Full match
- `"1"`, `"2"`, ... - Numbered capture groups
- `"name"` - Named capture groups

Each capture group value is a MAP with:
- `"match"` - The matched substring (STR)
- `"position"` - List {start, end} with 1-indexed positions (LIST)

Returns `{}` if no matches found.

### Examples

**Basic match:**
```moo
pcre_match("hello world", "h(\\w+)")
=> {["0" -> ["match" -> "hello", "position" -> {1, 5}],
     "1" -> ["match" -> "ello", "position" -> {2, 5}]]}
```

**No match:**
```moo
pcre_match("hello", "goodbye")
=> {}
```

**Multiple matches (default):**
```moo
pcre_match("abc 123 def 456", "\\d+")
=> {["0" -> ["match" -> "123", "position" -> {5, 7}]],
    ["0" -> ["match" -> "456", "position" -> {13, 15}]]}
```

**First match only:**
```moo
pcre_match("abc 123 def 456", "\\d+", 0, 0)
=> {["0" -> ["match" -> "123", "position" -> {5, 7}]]}
```

**Case sensitive:**
```moo
pcre_match("HELLO", "hello", 1)
=> {}

pcre_match("HELLO", "hello", 0)
=> {["0" -> ["match" -> "HELLO", "position" -> {1, 5}]]}
```

**Named groups:**
```moo
pcre_match("hello world", "(?P<word>\\w+)")
=> {["0" -> ["match" -> "hello", "position" -> {1, 5}],
     "word" -> ["match" -> "hello", "position" -> {1, 5}]]}
```

### Errors

| Error | Condition |
|-------|-----------|
| E_INVARG | Invalid pattern or empty pattern/subject |
| E_QUOTA | Too many capture groups |
| E_MAXREC | Exceeded pcre_match_max_iterations |
| E_TYPE | Non-string arguments |
| E_ARGS | Wrong argument count |

### Performance

- Patterns are cached after first compilation
- Cache hits tracked by `pcre_cache_stats()`
- Server option `pcre_match_max_iterations` limits match loop (default 10000, min 100, max 100000000)

---

## 2. pcre_replace

### Signature

```
pcre_replace(subject, command) → STR
```

### Arguments

| Argument | Type | Description |
|----------|------|-------------|
| subject | STR | String to perform replacement on |
| command | STR | Perl-style s/pattern/replacement/flags command |

### Command Syntax

Uses Perl s/// syntax: `s/pattern/replacement/flags`

**Delimiters:** Any character can be used as delimiter:
- `s/pattern/replacement/` - standard
- `s#pattern#replacement#` - alternate
- `s|pattern|replacement|` - alternate

**Flags:**
| Flag | Effect |
|------|--------|
| g | Global - replace all matches (without g, only first match) |
| i | Case insensitive |
| (other Perl flags as supported by pcrs library) |

**Backreferences in replacement:**
- `$0` - Full match
- `$1`, `$2`, ... - Capture groups
- `$$` - Literal $

### Examples

**Simple replacement:**
```moo
pcre_replace("hello world", "s/world/MOO/")
=> "hello MOO"
```

**Global replacement:**
```moo
pcre_replace("foo bar foo", "s/foo/baz/g")
=> "baz bar baz"

pcre_replace("foo bar foo", "s/foo/baz/")  // no 'g' flag
=> "baz bar foo"  // only first match
```

**Capture groups:**
```moo
pcre_replace("abc 123 def", "s/(\\d+)/[$1]/")
=> "abc [123] def"
```

**Case insensitive:**
```moo
pcre_replace("HELLO world", "s/hello/hi/i")
=> "hi world"
```

**Multiple digits, global:**
```moo
pcre_replace("abc 123 def 456", "s/\\d+/X/g")
=> "abc X def X"
```

**Alternate delimiters:**
```moo
pcre_replace("path/to/file", "s#/#-#g")
=> "path-to-file"
```

### Errors

| Error | Condition |
|-------|-----------|
| E_INVARG | Invalid command syntax or pattern |
| E_TYPE | Non-string arguments |
| E_ARGS | Wrong argument count |

### Notes

- Non-printable characters in replacement are converted to spaces
- Uses pcrs library (included with Toast)
- Pattern is compiled on each call (no caching for pcre_replace)

---

## 3. pcre_cache_stats

### Signature

```
pcre_cache_stats() → LIST
```

### Description

Returns statistics about compiled PCRE pattern cache. **Wizard-only.**

### Return Value

List of lists, each containing:
- [1] Pattern string (STR)
- [2] Number of cache hits (INT)

### Example

```moo
pcre_cache_stats()
=> {{"\\d+", 42}, {"\\w+", 17}, {"[a-z]+", 8}}
```

### Errors

| Error | Condition |
|-------|-----------|
| E_PERM | Caller is not a wizard |
| E_ARGS | Wrong argument count |

### Added

ToastStunt v2.6.1 (March 2020)

---

## Common PCRE Patterns

| Pattern | Matches |
|---------|---------|
| `\d+` | One or more digits |
| `\w+` | Word characters (letters, digits, underscore) |
| `\s+` | Whitespace |
| `^...$` | Entire string (^ = start, $ = end) |
| `(?i)...` | Case insensitive (inline flag) |
| `(?:...)` | Non-capturing group |
| `(?P<name>...)` | Named capture group |
| `(?=...)` | Lookahead assertion |
| `(?!...)` | Negative lookahead |
| `.` | Any character except newline |

---

## Escaping in MOO Strings

Remember: MOO string literals use `\` for escaping, PCRE also uses `\` for patterns.

**You need double backslashes in MOO code:**

| Pattern Meaning | MOO String Literal |
|-----------------|-------------------|
| One digit | `"\\d"` |
| One backslash | `"\\\\"` |
| Word boundary | `"\\b"` |

---

## MOO Patterns vs PCRE

MOO has its own pattern matching with `match()` - see [strings.md](strings.md).

| Function | Pattern Type |
|----------|--------------|
| `match()` | MOO patterns (%w, %d, etc.) |
| `pcre_match()` | PCRE (standard regex) |

**MOO pattern syntax quick reference:**
| MOO | PCRE Equivalent |
|-----|-----------------|
| %w | \w |
| %d | \d |
| %s | \s |
| %. | . |
| %* | * |
| %+ | + |

Use MOO patterns for simple matching, PCRE for complex patterns.

---

## Version History

| Version | Changes |
|---------|---------|
| v2.6.1 (Mar 2020) | Added pattern caching, `pcre_cache_stats()` |
| v2.3.14 (Jun 2018) | **Breaking change:** Rewrote `pcre_match()` to return list of maps |
| Earlier | Initial PCRE support with `pcre_match()` and `pcre_replace()` |

---

## Compilation Notes

PCRE support is **optional** in ToastStunt:
- Requires PCRE library at compile time
- CMake option: `find_package(PCRE)`
- If not compiled with PCRE, functions will be "Unknown built-in function"

Check if your Toast has PCRE:
```moo
try
  return pcre_match("test", "t");
except e (ANY)
  return e[1] == E_VERBNF ? "No PCRE support" | "PCRE available";
endtry
```

---

## Go Implementation Notes

### Data Structures

```go
// Toast returns list of maps with this structure
type PcreMatch struct {
    Captures map[string]PcreCapture  // "0", "1", ..., or named groups
}

type PcreCapture struct {
    Match    string    // matched substring
    Position []int     // {start, end} 1-indexed
}
```

### Implementation Approach

```go
import "regexp"

func builtinPcreMatch(args []Value) (Value, error) {
    subject := args[0].(StringValue).String()
    pattern := args[1].(StringValue).String()

    // arg[2]: case_matters (default 0 = insensitive)
    caseSensitive := len(args) > 2 && args[2].ToBool()

    // arg[3]: find_all (default 1 = all matches)
    findAll := len(args) < 4 || args[3].ToBool()

    // Build pattern with case flag
    if !caseSensitive {
        pattern = "(?i)" + pattern
    }

    re, err := regexp.Compile(pattern)
    if err != nil {
        return nil, E_INVARG
    }

    var results []Value

    if findAll {
        // Find all matches
        matches := re.FindAllStringSubmatchIndex(subject, -1)
        for _, match := range matches {
            results = append(results, buildMatchMap(subject, re, match))
        }
    } else {
        // Find first match only
        match := re.FindStringSubmatchIndex(subject)
        if match != nil {
            results = append(results, buildMatchMap(subject, re, match))
        }
    }

    return NewList(results), nil
}

func buildMatchMap(subject string, re *regexp.Regexp, indices []int) Value {
    captures := make(map[string]Value)

    // Add numbered captures
    for i := 0; i < len(indices)/2; i++ {
        start, end := indices[2*i], indices[2*i+1]
        if start >= 0 {
            capture := map[string]Value{
                "match":    NewString(subject[start:end]),
                "position": NewList([]Value{NewInt(start+1), NewInt(end)}), // 1-indexed
            }
            captures[fmt.Sprintf("%d", i)] = NewMap(capture)
        }
    }

    // Add named captures
    for i, name := range re.SubexpNames() {
        if i > 0 && name != "" {
            start, end := indices[2*i], indices[2*i+1]
            if start >= 0 {
                capture := map[string]Value{
                    "match":    NewString(subject[start:end]),
                    "position": NewList([]Value{NewInt(start+1), NewInt(end)}),
                }
                captures[name] = NewMap(capture)
            }
        }
    }

    return NewMap(captures)
}

func builtinPcreReplace(args []Value) (Value, error) {
    subject := args[0].(StringValue).String()
    command := args[1].(StringValue).String()

    // Parse s/pattern/replacement/flags format
    pattern, replacement, flags, err := parsePerlCommand(command)
    if err != nil {
        return nil, E_INVARG
    }

    // Apply flags
    if strings.Contains(flags, "i") {
        pattern = "(?i)" + pattern
    }

    re, err := regexp.Compile(pattern)
    if err != nil {
        return nil, E_INVARG
    }

    result := subject
    if strings.Contains(flags, "g") {
        // Global replacement
        result = re.ReplaceAllString(subject, replacement)
    } else {
        // First match only
        result = re.ReplaceAllStringFunc(subject, func(match string) string {
            // Only replace first occurrence
            return replacement
        })
    }

    // Convert $1, $2 to Go's ${1}, ${2} format
    result = convertPerlBackrefs(result)

    // Sanitize non-printable characters
    return NewString(sanitize(result)), nil
}

// Parse s/pattern/replacement/flags
func parsePerlCommand(cmd string) (pattern, replacement, flags string, err error) {
    if len(cmd) < 4 || cmd[0] != 's' {
        return "", "", "", errors.New("invalid s/// syntax")
    }

    delimiter := cmd[1]
    parts := splitByDelimiter(cmd[2:], delimiter)

    if len(parts) < 2 {
        return "", "", "", errors.New("incomplete s/// command")
    }

    pattern = parts[0]
    replacement = parts[1]
    if len(parts) > 2 {
        flags = parts[2]
    }

    return pattern, replacement, flags, nil
}
```

### Key Implementation Details

1. **Case sensitivity:** Toast's 3rd arg is backwards from typical regex - 0 means insensitive
2. **Return structure:** Must build nested maps with "match" and "position" keys
3. **Position indices:** 1-indexed in MOO (Go's are 0-indexed)
4. **Named groups:** Extract from regexp.SubexpNames() and add to result
5. **pcre_replace:** Must parse s/// syntax, not just pattern + replacement
6. **Global flag:** Only in s///g syntax, not a separate argument
