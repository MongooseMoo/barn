# MOO Regular Expression Built-ins

## Overview

Functions for PCRE (Perl-Compatible Regular Expressions) pattern matching.

---

## 1. PCRE Matching

### 1.1 pcre_match

**Signature:** `pcre_match(subject, pattern [, options]) → LIST`

**Description:** Matches PCRE pattern against subject.

**Returns:** List of captures, or empty list if no match.
- First element: Full match
- Subsequent elements: Captured groups

**Examples:**
```moo
pcre_match("hello world", "h(\\w+)")
// => {"hello", "ello"}

pcre_match("hello", "goodbye")
// => {}

pcre_match("abc123def", "(\\d+)")
// => {"123", "123"}

// Named groups
pcre_match("hello world", "(?P<word>\\w+)")
// => {"hello", "hello"}
```

**Errors:**
- E_INVARG: Invalid pattern

---

### 1.2 Options

Pass options as string:

| Option | Effect |
|--------|--------|
| "i" | Case insensitive |
| "m" | Multiline (^ and $ match line boundaries) |
| "s" | Dotall (. matches newline) |
| "x" | Extended (ignore whitespace) |

**Examples:**
```moo
pcre_match("HELLO", "hello", "i")
// => {"HELLO"}

pcre_match("line1\nline2", "^line", "m")
// => {"line"}
```

---

## 2. Replace Operations

### 2.1 pcre_replace

**Signature:** `pcre_replace(subject, pattern, replacement [, options]) → STR`

**Description:** Replaces matches with replacement.

**Replacement syntax:**
| Pattern | Meaning |
|---------|---------|
| `$0` | Entire match |
| `$1`, `$2`... | Captured groups |
| `${name}` | Named group |
| `$$` | Literal $ |

**Examples:**
```moo
pcre_replace("hello world", "world", "MOO")
// => "hello MOO"

pcre_replace("abc 123 def", "(\\d+)", "[$1]")
// => "abc [123] def"

pcre_replace("hello HELLO", "hello", "hi", "i")
// => "hi HELLO"  (first match only)
```

---

### 2.2 pcre_replace_all (ToastStunt)

**Signature:** `pcre_replace_all(subject, pattern, replacement [, options]) → STR`

**Description:** Replaces all matches.

**Examples:**
```moo
pcre_replace_all("abc 123 def 456", "\\d+", "X")
// => "abc X def X"
```

---

## 3. Split Operations

### 3.1 pcre_split (ToastStunt)

**Signature:** `pcre_split(subject, pattern [, limit]) → LIST`

**Description:** Splits string by pattern.

**Examples:**
```moo
pcre_split("a,b,,c", ",")
// => {"a", "b", "", "c"}

pcre_split("a1b2c3d", "\\d")
// => {"a", "b", "c", "d"}

pcre_split("a,b,c,d", ",", 2)
// => {"a", "b,c,d"}
```

---

## 4. Multiple Matches

### 4.1 pcre_match_all (ToastStunt)

**Signature:** `pcre_match_all(subject, pattern [, options]) → LIST`

**Description:** Finds all matches.

**Returns:** List of match lists.

**Examples:**
```moo
pcre_match_all("abc 123 def 456", "\\d+")
// => {{"123"}, {"456"}}

pcre_match_all("a1b2c3", "(\\w)(\\d)")
// => {{"a1", "a", "1"}, {"b2", "b", "2"}, {"c3", "c", "3"}}
```

---

## 5. MOO Patterns (Legacy)

MOO has its own pattern matching (see [strings.md](strings.md)):

| Function | Pattern Type |
|----------|--------------|
| match() | MOO patterns |
| pcre_match() | PCRE |

**MOO pattern syntax:**
| Pattern | PCRE Equivalent |
|---------|-----------------|
| %w | \w |
| %d | \d |
| %s | \s |
| %. | . |
| %( %) | ( ) |
| %* | * |
| %+ | + |

---

## 6. Pattern Validation

### 6.1 pcre_valid (ToastStunt)

**Signature:** `pcre_valid(pattern) → BOOL`

**Description:** Tests if pattern is valid PCRE.

**Examples:**
```moo
pcre_valid("\\d+")      => true
pcre_valid("[invalid")  => false
```

---

## 7. Common Patterns

| Pattern | Matches |
|---------|---------|
| `\d+` | One or more digits |
| `\w+` | Word characters |
| `\s+` | Whitespace |
| `^...$` | Entire string |
| `(?i)` | Case insensitive (inline) |
| `(?:...)` | Non-capturing group |
| `(?P<name>...)` | Named group |
| `(?=...)` | Lookahead |
| `(?!...)` | Negative lookahead |

---

## 8. Error Handling

| Error | Condition |
|-------|-----------|
| E_INVARG | Invalid pattern |
| E_TYPE | Non-string arguments |
| E_ARGS | Wrong argument count |

---

## 9. Performance Considerations

- Patterns are compiled on each call
- For repeated use, consider caching results
- Avoid catastrophic backtracking:
  - `(a+)+` on "aaaaaab" is slow
  - Use possessive quantifiers or atomic groups

---

## 10. Go Implementation Notes

```go
import "regexp"

func builtinPcreMatch(args []Value) (Value, error) {
    subject := string(args[0].(StringValue))
    pattern := string(args[1].(StringValue))

    flags := ""
    if len(args) > 2 {
        flags = string(args[2].(StringValue))
    }

    // Build pattern with flags
    fullPattern := pattern
    if strings.Contains(flags, "i") {
        fullPattern = "(?i)" + fullPattern
    }
    if strings.Contains(flags, "m") {
        fullPattern = "(?m)" + fullPattern
    }
    if strings.Contains(flags, "s") {
        fullPattern = "(?s)" + fullPattern
    }

    re, err := regexp.Compile(fullPattern)
    if err != nil {
        return nil, E_INVARG
    }

    matches := re.FindStringSubmatch(subject)
    if matches == nil {
        return &MOOList{data: nil}, nil
    }

    result := make([]Value, len(matches))
    for i, m := range matches {
        result[i] = StringValue(m)
    }
    return &MOOList{data: result}, nil
}

func builtinPcreReplace(args []Value) (Value, error) {
    subject := string(args[0].(StringValue))
    pattern := string(args[1].(StringValue))
    replacement := string(args[2].(StringValue))

    flags := ""
    if len(args) > 3 {
        flags = string(args[3].(StringValue))
    }

    fullPattern := pattern
    if strings.Contains(flags, "i") {
        fullPattern = "(?i)" + fullPattern
    }

    re, err := regexp.Compile(fullPattern)
    if err != nil {
        return nil, E_INVARG
    }

    // Convert $1 to ${1} for Go regexp
    goReplacement := convertReplacement(replacement)

    result := re.ReplaceAllString(subject, goReplacement)
    return StringValue(result), nil
}

func builtinPcreMatchAll(args []Value) (Value, error) {
    subject := string(args[0].(StringValue))
    pattern := string(args[1].(StringValue))

    re, err := regexp.Compile(pattern)
    if err != nil {
        return nil, E_INVARG
    }

    allMatches := re.FindAllStringSubmatch(subject, -1)
    result := make([]Value, len(allMatches))

    for i, matches := range allMatches {
        matchList := make([]Value, len(matches))
        for j, m := range matches {
            matchList[j] = StringValue(m)
        }
        result[i] = &MOOList{data: matchList}
    }

    return &MOOList{data: result}, nil
}
```
