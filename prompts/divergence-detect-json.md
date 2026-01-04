# Task: Detect Divergences in JSON Builtins

## Context

We need to verify Barn's JSON builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all JSON builtins.

## Files to Read

- `spec/builtins/json.md` - expected behavior specification
- `builtins/json.go` - Barn implementation (if exists)

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### JSON Encoding
- `generate_json()` - convert MOO value to JSON string
- `tojson()` - alias or variant (if exists)

### JSON Decoding
- `parse_json()` - convert JSON string to MOO value
- `fromjson()` - alias or variant (if exists)

### Type Mapping
- Lists → JSON arrays
- Maps → JSON objects
- Strings, numbers, booleans
- Object references (#123) - how are they handled?
- Errors - how are they handled?

## Edge Cases to Test

- Empty objects `{}`
- Empty arrays `[]`
- Nested structures
- Unicode strings
- Large numbers
- Null values
- Invalid JSON
- Circular references (if possible)

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe 'generate_json({1, 2, 3})'
./toast_oracle.exe 'parse_json("[1, 2, 3]")'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return generate_json({1, 2, 3});"
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return parse_json(\"[1, 2, 3]\");"

# Check conformance tests
grep -r "json\|generate_json\|parse_json" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-json.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test EVERY major JSON builtin
- JSON is a ToastStunt extension - check if it exists in Toast
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
