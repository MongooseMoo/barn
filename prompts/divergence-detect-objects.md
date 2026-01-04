# Task: Detect Divergences in Object Builtins

## Context

We need to verify Barn's object builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all object builtins.

## Files to Read

- `spec/builtins/objects.md` - expected behavior specification
- `builtins/objects.go` - Barn implementation

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### Object Queries
- `valid()` - recycled objects, #-1, negative objnums
- `typeof()` for objects - OBJ type code
- `max_object()` - highest object number
- `players()` - list of player objects
- `connected_players()` - logged-in players

### Object Creation/Destruction
- `create()` - with/without parent, with owner
- `recycle()` - permissions, effects on references
- `recreate()` (ToastStunt) - recreate recycled object
- `chparent()` - change parent, permission checks

### Inheritance
- `parent()` - get parent object
- `children()` - get child objects
- `ancestors()` (ToastStunt) - get all ancestors
- `descendants()` (ToastStunt) - get all descendants
- `isa()` (ToastStunt) - inheritance check

### Object Attributes
- `is_player()` - player flag check
- `set_player_flag()` - modify player status
- `move()` - change object location
- `location()` - get location property
- `contents()` - get contents property

### Anonymous Objects (ToastStunt)
- `create()` with $anon parent
- Anonymous object comparisons
- Anonymous object serialization

## Edge Cases to Test

- Object #0 (system object)
- Object #-1 (NOTHING)
- Object #-2 (AMBIGUOUS)
- Object #-3 (FAILED_MATCH)
- Recycled object references
- Circular inheritance (should error)
- Permission violations

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe 'valid(#0)'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return valid(#0);"

# Check conformance tests
grep -r "valid\|create\|recycle\|parent\|children" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-objects.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test EVERY major object builtin
- Pay special attention to permission checks and special object numbers
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
