# Task: Implement Waif Objects

## Context

Barn (Go MOO server) is failing 21 conformance tests in the `waif` category. Waifs are lightweight objects in MOO that don't have a database presence.

## Test Location

Tests are in: `~/code/cow_py/tests/conformance/` - search for waif-related yaml files

## Reference Implementations

1. **ToastStunt** (C++): `~/src/toaststunt/`
   - Search for "waif" in the codebase
   - Look at how waifs are created, stored, and accessed

2. **cow_py** (Python): `~/code/cow_py/`
   - Search for waif implementation
   - This is the Python MOO server that runs these tests

## What Needs to Be Done

1. Understand what waifs are by reading ToastStunt/cow_py code
2. Check what barn already has for waifs in `barn/types/`
3. Implement missing waif functionality:
   - Waif creation
   - Waif property access
   - Waif type checking
   - Waif builtins

## Key Files to Check in Barn

- `types/waif.go` - if it exists
- `types/value.go` - type definitions
- `builtins/` - waif-related builtins

## Test Command

```bash
cd ~/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9302 -k "waif" -v
```

Server should already be running on port 9302.

## Output

Write findings and implementation to `./reports/fix-waif.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
