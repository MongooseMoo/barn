# Task: Detect Divergences in Map Builtins

## Context

We need to verify Barn's map builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all map builtins.

## Files to Read

- `spec/builtins/maps.md` - expected behavior specification
- `builtins/maps.go` - Barn implementation (if exists)

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### Map Creation
- `mapkeys()` - get all keys from map
- `mapvalues()` - get all values from map
- `maphaskey()` - check if key exists
- `mapdelete()` - remove key from map

### Map Conversion
- `map()` - create map from list of pairs
- `tolist()` - convert map to list

### Map Operations
- `mapiter()` - iterate over map (if exists)
- `mapput()` - add/update key (if exists)

## Edge Cases to Test

- Empty maps
- Non-string keys
- Nested maps
- Large maps
- Key collision/overwrite
- Invalid key types

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe '[]'
./toast_oracle.exe 'mapkeys(["a" -> 1, "b" -> 2])'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return [];"
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return mapkeys([\"a\" -> 1, \"b\" -> 2]);"

# Check conformance tests
grep -r "mapkeys\|mapvalues\|maphaskey\|mapdelete" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-maps.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test EVERY major map builtin
- Maps are a ToastStunt extension - check if they exist in Toast
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
