# Task: Detect Divergences in Type Builtins

## Context

We need to verify Barn's type builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all type builtins.

## Files to Read

- `spec/builtins/types.md` - expected behavior specification
- `builtins/types.go` - Barn implementation

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### Type Checking
- `typeof()` - all type codes (INT=0, OBJ=1, STR=2, ERR=3, LIST=4, etc.)
- `is_int()` / `is_float()` / `is_num()` - edge cases
- `is_str()` / `is_list()` / `is_map()` / `is_obj()` / `is_err()`
- `is_anon()` - anonymous object detection (ToastStunt)

### Type Conversion
- `tostr()` - all types, nested structures, special values
- `toint()` - strings, floats, rounding behavior, overflow
- `tofloat()` - strings, integers, special values (inf, nan)
- `tonum()` - automatic type detection
- `toobj()` - string parsing (#N format), integer conversion
- `toliteral()` - escaping, special characters, nested structures
- `value_hash()` - hash collision testing, type handling

### Object Type Info
- `valid()` - recycled objects, negative objnums, special objects
- `parent()` / `children()` - inheritance queries
- `chparent()` - parent change behavior

## Edge Cases to Test

- Empty values (empty string, empty list, empty map)
- Special objects (#-1, #-2, #-3, #-4)
- Maximum/minimum integers
- Float special values (Inf, -Inf, NaN)
- Error values (E_TYPE, E_INVARG, etc.)
- Recursive/nested structures in tostr/toliteral

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe 'typeof(42)'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return typeof(42);"

# Check conformance tests
grep -r "typeof\|tostr\|toint\|tofloat" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-types.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test EVERY major type builtin
- Pay special attention to edge cases (empty, max/min, special values)
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
