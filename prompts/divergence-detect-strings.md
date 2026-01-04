# Task: Detect Divergences in String Builtins

## Context

We need to verify Barn's string builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all string builtins.

## Files to Read

- `spec/builtins/strings.md` - expected behavior specification
- `builtins/strings.go` - Barn implementation

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

From typical MOO string builtins:

### Basic String Operations
- `length()` - empty string, unicode
- `tostr()` - various types, nested lists
- `toliteral()` - escaping, special characters
- `strsub()` - case sensitivity, empty patterns, overlapping matches
- `index()` / `rindex()` - not found (-1 vs 0?), case sensitivity
- `strcmp()` - case comparison, empty strings

### String Manipulation
- `substr()` - edge cases: start=0, negative indices, out of bounds
- `decode_binary()` / `encode_binary()` - various formats
- `crypt()` - salt formats, algorithm selection

### Pattern Matching
- `match()` / `rmatch()` - regex patterns, special chars
- `substitute()` - replacement patterns, backreferences

### Conversion
- `toint()` - invalid strings, overflow, base conversion
- `tofloat()` - invalid strings, special values (inf, nan)
- `tonum()` - type detection

### Case Operations
- `toupper()` / `tolower()` - unicode?, non-alpha chars

### Tokenization
- `explode()` / `implode()` - empty delimiters, consecutive delimiters
- `argstr()` - argument parsing

### ToastStunt Extensions
- `pcre_match()` - PCRE regex patterns
- `spellcheck()` - if implemented
- Various string utilities

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe 'index("hello", "ll")'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return index(\"hello\", \"ll\");"

# Check conformance tests
grep -r "index\|strsub\|explode" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-strings.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test EVERY major string builtin
- Pay special attention to edge cases (empty strings, unicode, escaping)
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
