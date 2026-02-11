# MOO String Built-ins

## Overview

String manipulation functions. All string indices are 1-based.

---

## 1. Basic Operations

### 1.1 length

**Signature:** `length(string) → INT`

**Description:** Returns number of characters.

**Examples:**
```moo
length("")        => 0
length("hello")   => 5
length("日本語")  => 3  (characters, not bytes)
```

**Errors:**
- E_TYPE: Not a string

---

### 1.2 strsub

**Signature:** `strsub(subject, old, new [, case_matters]) → STR`

**Description:** Replaces all occurrences of `old` with `new`.

**Parameters:**
- `subject`: String to search in
- `old`: Pattern to find
- `new`: Replacement text
- `case_matters`: If false (default), case-insensitive

**Examples:**
```moo
strsub("hello world", "o", "0")      => "hell0 w0rld"
strsub("Hello World", "o", "0", 0)   => "Hell0 W0rld"
strsub("Hello World", "O", "0", 1)   => "Hello World"
strsub("aaa", "a", "bb")             => "bbbbbb"
```

**Errors:**
- E_TYPE: Non-string string/delimiter or non-integer `adjacent_delim`
- E_INVARG: Empty `old` string

---

### 1.3 index

**Signature:** `index(haystack, needle [, case_matters [, start]]) → INT`

**Description:** Returns 1-based position of first occurrence.

**Parameters:**
- `haystack`: String to search in
- `needle`: String to find
- `case_matters`: If false (default), case-insensitive
- `start`: Starting position (1-based)

**Returns:** Position (1-based) or 0 if not found.

**Examples:**
```moo
index("hello", "l")           => 3
index("hello", "L")           => 3  (case-insensitive)
index("hello", "L", 1)        => 0  (case-sensitive)
index("hello", "l", 0, 4)     => 4  (start at position 4)
index("hello", "x")           => 0
```

**Errors:**
- E_TYPE: Non-string arguments

---

### 1.4 rindex

**Signature:** `rindex(haystack, needle [, case_matters [, start]]) → INT`

**Description:** Returns 1-based position of last occurrence.

**Examples:**
```moo
rindex("hello", "l")   => 4
rindex("abcabc", "b")  => 5
```

---

### 1.5 strcmp

**Signature:** `strcmp(str1, str2) → INT`

**Description:** Case-sensitive string comparison.

**Returns:**
- Negative if str1 < str2
- Zero if str1 == str2
- Positive if str1 > str2

**Examples:**
```moo
strcmp("abc", "abc")   => 0
strcmp("abc", "abd")   => -1  (or any negative)
strcmp("abd", "abc")   => 1   (or any positive)
strcmp("ABC", "abc")   => -1  (uppercase < lowercase)
```

**Errors:**
- E_TYPE: Non-string arguments

---

### 1.6 strtr (ToastStunt)

**Signature:** `strtr(subject, from, to) → STR`

**Description:** Translates characters. Each character in `from` is replaced by corresponding character in `to`.

**Examples:**
```moo
strtr("hello", "aeiou", "12345")   => "h2ll4"
strtr("hello", "helo", "1234")    => "12334"
```

**Errors:**
- E_TYPE: Non-string arguments
- E_INVARG: `from` and `to` different lengths

---

## 2. Substrings

### 2.1 substr (implicit via indexing)

MOO uses indexing syntax for substrings:

```moo
str[start..end]   // 1-based, inclusive
str[start..$]     // To end of string
```

**Examples:**
```moo
"hello"[1..3]     => "hel"
"hello"[2..4]     => "ell"
"hello"[3..$]     => "llo"
```

---

### 2.2 explode (ToastStunt)

**Signature:** `explode(string [, delimiter [, adjacent_delim]]) → LIST`

**Description:** Splits a string on a single-character delimiter.

**Parameters:**
- `string`: String to split
- `delimiter`: Optional delimiter string; only the first character is used. If omitted or empty, defaults to a space.
- `adjacent_delim`: If truthy, keep empty fields between adjacent delimiters. If falsey or omitted, adjacent delimiters are treated as one.

**Examples:**
```moo
explode("hello world")            => {"hello", "world"}
explode("a,b,c", ",")             => {"a", "b", "c"}
explode("a,,b", ",")              => {"a", "b"}
explode("a,,b", ",", 1)           => {"a", "", "b"}
explode(",a,", ",", 1)            => {"", "a", ""}
explode("  hello  world  ")       => {"hello", "world"}
```

**Errors:**
- E_TYPE: Non-string arguments

---

## 3. Formatting

### 3.1 tostr

See [types.md](types.md) - converts values to strings.

---

### 3.2 crypt

**Signature:** `crypt(plaintext [, salt]) → STR`

**Description:** One-way hash using Unix crypt().

**Platform Notes:**
- Windows: Only bcrypt ($2a$/$2b$) salts are supported
- Other platforms: Traditional DES crypt may be supported

**Examples:**
```moo
crypt("password", "$2a$10$...")   => "$2a$10$..."  (bcrypt on Windows)
```

---

### 3.3 string_hash

**Signature:** `string_hash(string [, algorithm]) → STR`

**Description:** Cryptographic hash of string, returns hex-encoded string.

**Algorithms:** "MD5", "SHA1", "SHA256" (default), "SHA512"

**Examples:**
```moo
string_hash("hello")              => "2CF24DBA5FB0A30E26E83B2AC5B9E29E1B161E5C1FA7425E73043362938B9824"  (SHA256 default)
string_hash("hello", "MD5")       => "5D41402ABC4B2A76B9719D911017C592"
string_hash("hello", "SHA256")    => "2CF24DBA5FB0A30E26E83B2AC5B9E29E1B161E5C1FA7425E73043362938B9824"
```

---

### 3.4 binary_hash

**Signature:** `binary_hash(string [, algorithm]) → STR`

**Description:** Cryptographic hash of string, returns hex-encoded string. Despite the name, returns the same format as string_hash (hex string, not raw binary).

**Algorithms:** Same as string_hash

---

## 4. Character Operations

### 4.1 chr (ToastStunt)

**Signature:** `chr(args...) → STR`

**Description:** Builds a string from byte values, strings, or lists. This is not Unicode-aware; values are raw bytes.

**Semantics:**
- Each argument may be an INT (byte), STR, or LIST (recursively processed).
- Non-wizards may only emit bytes in the range 32..254.
- Wizards may emit bytes in the range 0..255.

**Examples:**
```moo
chr(65)                    => "A"
chr({65, 66}, "CD")        => "ABCD"
```

**Errors:**
- E_INVARG: Non-int/string/list element, or int outside allowed byte range
- E_QUOTA: Result exceeds allocation limits

---

## 5. Encoding

### 5.1 encode_binary

**Signature:** `encode_binary(args...) → STR`

**Description:** Creates a binary string from integers, strings, or lists (recursively).

**Semantics:**
- INT values must be in 0..255.
- STR values are appended as raw bytes.
- LIST values are traversed recursively.

**Examples:**
```moo
encode_binary(65, 66, 67)     => "ABC"
encode_binary({0, 255})       => binary string with bytes 0, 255
encode_binary({"A", {66}})    => "AB"
```

**Errors:**
- E_INVARG: Invalid type or byte outside 0..255
- E_QUOTA: Result exceeds allocation limits

---

### 5.2 decode_binary

**Signature:** `decode_binary(string [, fully]) → LIST`

**Description:** Converts a binary string to a list of integers or mixed values.

**Semantics:**
- If `fully` is truthy, returns a list of integers (one per byte).
- Otherwise, returns a list where printable runs (isgraph, space, tab) are STR and non-printable bytes are INT.

**Examples:**
```moo
decode_binary("ABC", 1)     => {65, 66, 67}
decode_binary("AB\x00C")    => {"AB", 0, "C"}
```

**Errors:**
- E_INVARG: Not a binary string
- E_QUOTA: Result exceeds allocation limits

---

### 5.3 encode_base64 (ToastStunt)

**Signature:** `encode_base64(string [, url_safe]) → STR`

**Description:** Encodes a binary string as base64.

**Semantics:**
- `string` must be a binary string (as produced by `encode_binary`).
- If `url_safe` is truthy, uses "-" and "_" and omits "=" padding.

**Examples:**
```moo
encode_base64(encode_binary("hello"))   => "aGVsbG8="
encode_base64(encode_binary("hi"), 1)   => "aGk"
```

**Errors:**
- E_INVARG: Invalid binary string
- E_QUOTA: Result exceeds allocation limits

---

### 5.4 decode_base64 (ToastStunt)

**Signature:** `decode_base64(string [, url_safe]) → STR`

**Description:** Decodes base64 into a binary string.

**Semantics:**
- If `url_safe` is truthy, accepts "-" and "_" and does not require padding.

**Examples:**
```moo
decode_base64("aGVsbG8=")     => encode_binary("hello")
decode_base64("aGk", 1)       => encode_binary("hi")
```

**Errors:**
- E_INVARG: Invalid character, invalid padding, or invalid length
- E_QUOTA: Result exceeds allocation limits

---

### 5.5 parse_ansi (ToastStunt)

**Signature:** `parse_ansi(string) → STR`

**Description:** Replaces ANSI tag tokens with ANSI escape sequences.

**Supported tags (case-insensitive):**
- `[red]`, `[green]`, `[yellow]`, `[blue]`, `[purple]`, `[cyan]`
- `[normal]`, `[inverse]`, `[underline]`, `[bold]`, `[bright]`, `[unbold]`
- `[blink]`, `[unblink]`, `[magenta]`, `[unbright]`, `[white]`, `[gray]`, `[grey]`
- `[black]`, `[b:black]`, `[b:red]`, `[b:green]`, `[b:yellow]`, `[b:blue]`, `[b:magenta]`, `[b:purple]`, `[b:cyan]`, `[b:white]`
- `[beep]` (bell), `[random]` (random color), `[null]` (removed)

**Notes:**
- `[random]` selects from a fixed set of color codes (red/green/yellow/blue/purple).
- `[null]` is removed from the output.

---

### 5.6 remove_ansi (ToastStunt)

**Signature:** `remove_ansi(string) → STR`

**Description:** Removes the ANSI tag tokens recognized by `parse_ansi()`. This does not strip raw ANSI escape codes.

---

## 6. Pattern Matching

### 6.1 match

**Signature:** `match(subject, pattern [, case_matters]) → LIST`

**Description:** Matches MOO pattern against string.

**MOO pattern syntax:**

| Pattern | Matches |
|---------|---------|
| `%w` | Word character |
| `%W` | Non-word character |
| `%s` | Space |
| `%S` | Non-space |
| `%d` | Digit |
| `%D` | Non-digit |
| `%.` | Any character |
| `%^` | Start of string |
| `%$` | End of string |
| `%(` `)%` | Capture group |
| `%*` | Zero or more |
| `%+` | One or more |
| `%?` | Zero or one |
| `%[abc]` | Character class |
| `%[^abc]` | Negated class |
| `%%` | Literal % |

**Returns:** `{start, end, capture_ranges, subject}` or `{}`

Where `capture_ranges` is a list of `{start, end}` tuples for each capture group. Groups that didn't match have `{0, -1}`.

**Examples:**
```moo
match("hello world", "%w+")
=> {1, 5, {{0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}}, "hello world"}

match("hello world", "%(hello%) %(world%)")
=> {1, 11, {{1, 5}, {7, 11}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}}, "hello world"}

match("hello", "goodbye")
=> {}
```

---

### 6.2 rmatch

**Signature:** `rmatch(subject, pattern [, case_matters]) → LIST`

**Description:** Like match but finds last occurrence. Returns same format as match.

---

### 6.3 substitute

**Signature:** `substitute(template, subs) → STR`

**Description:** Substitutes captured groups into template.

**Template syntax:**
- `%1`, `%2`, etc. - Captured groups
- `%%` - Literal %

**Examples:**
```moo
subs = match("hello world", "%(hello%) %(world%)");
substitute("%2 %1", subs)   => "world hello"
```

---

## 7. Error Handling

All string functions raise:
- E_TYPE for non-string arguments
- E_RANGE for index out of bounds
- E_INVARG for invalid arguments

---

## Go Implementation Notes

```go
func builtinIndex(args []Value) (Value, error) {
    haystack, ok := args[0].(StringValue)
    if !ok {
        return nil, E_TYPE
    }
    needle, ok := args[1].(StringValue)
    if !ok {
        return nil, E_TYPE
    }

    caseSensitive := false
    if len(args) > 2 {
        caseSensitive = isTruthy(args[2])
    }

    h, n := string(haystack), string(needle)
    if !caseSensitive {
        h = strings.ToLower(h)
        n = strings.ToLower(n)
    }

    start := 0
    if len(args) > 3 {
        s, _ := toInt(args[3])
        start = int(s) - 1  // Convert to 0-based
    }

    if start >= len(h) {
        return IntValue(0), nil
    }

    pos := strings.Index(h[start:], n)
    if pos < 0 {
        return IntValue(0), nil
    }
    return IntValue(start + pos + 1), nil  // Convert to 1-based
}
```
