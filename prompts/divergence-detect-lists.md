# Task: Detect Divergences in List Builtins

## Context

We need to verify Barn's list builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all list builtins.

## Files to Read

- `spec/builtins/lists.md` - expected behavior specification
- `builtins/lists.go` - Barn implementation

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### Basic List Operations
- `length()` - empty list, nested lists
- `listappend()` / `listinsert()` - at boundaries, negative indices
- `listdelete()` - at boundaries, out of bounds
- `listset()` - at boundaries, out of bounds
- `setadd()` / `setremove()` - duplicates, not found

### List Information
- `is_member()` - type matching, nested lists
- `indexof()` - not found, first/last occurrence
- `equal()` - deep comparison, type differences

### List Manipulation
- `reverse()` - empty list, single element
- `sort()` - comparison modes, mixed types
- `slice()` - edge cases, negative indices, out of bounds

### Conversion
- `tolist()` - from various types
- `toid()` - object ID parsing

### ToastStunt Extensions
- `occupants()` / `contents()` - if implemented as builtins
- List comprehension support

### List Indexing (via operators, but test here)
- `list[1]` - 1-based indexing
- `list[1..3]` - slicing
- `list[@list]` - splice

## Edge Cases to Test

- Empty list `{}`
- Single element `{1}`
- Nested lists `{{1, 2}, {3, 4}}`
- Mixed types `{1, "two", #3}`
- Very long lists (if applicable)
- Index 0, negative indices
- Out of bounds access

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe '{1, 2, 3}[2]'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return {1, 2, 3}[2];"

# Check conformance tests
grep -r "listappend\|listinsert\|setadd" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-lists.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test EVERY major list builtin
- Pay special attention to indexing edge cases (MOO uses 1-based indexing)
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
