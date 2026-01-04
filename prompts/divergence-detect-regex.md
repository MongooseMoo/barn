# Task: Detect Divergences in Regex Builtins

## Context

We need to verify Barn's regex builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all regex builtins.

## Files to Read

- `spec/builtins/regex.md` - expected behavior specification
- `builtins/regex.go` - Barn implementation (if exists)

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### PCRE Functions
- `pcre_match()` - match pattern against string
- `pcre_replace()` - find and replace with regex
- `pcre_split()` - split string by pattern

### Classic match()
- `match()` - classic MOO pattern matching (not PCRE)

## Edge Cases to Test

- Invalid regex patterns
- Empty strings
- Capture groups
- Backreferences
- Unicode characters
- Case insensitivity flags
- Multiline mode
- Global vs first match

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe 'pcre_match("hello", "h.*o")'
./toast_oracle.exe 'pcre_replace("hello world", "world", "there")'
./toast_oracle.exe 'match("hello", "h%w*o")'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return pcre_match(\"hello\", \"h.*o\");"

# Check conformance tests
grep -r "pcre_\|match(" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-regex.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test both PCRE functions and classic match()
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
