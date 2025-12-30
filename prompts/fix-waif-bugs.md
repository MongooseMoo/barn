# Task: Fix Barn's Waif Implementation Bugs

## CRITICAL CONTEXT
**Test.db is the SAME database that ToastStunt uses for its tests.**
- The database is CORRECT - $waif EXISTS and WORKS
- Toast's waif tests PASS with this database
- If Barn's waif tests FAIL, **BARN'S CODE IS BROKEN**
- Do NOT blame the database. Fix Barn's code.

## Failing Tests
29 out of 30 waif tests are failing. Example failures:
- `waifs_have_no_parents` - expects E_INVARG, got success with None
- `waif_owner_is_creator` - ownership not working
- `nested_waif_map_indexes` - nested map access on waif properties
- `deeply_nested_waif_map_indexes` - 3-level deep nested map access

## Investigation Steps

1. **First, verify Toast behavior:**
```bash
cd /c/Users/Q/code/barn
./toast_oracle.exe 'parents($waif:new())'
# Should return E_INVARG

./toast_oracle.exe '$waif'
# Should return an object ID like #5
```

2. **Test same on Barn:**
```bash
./moo_client.exe -port 8800 -timeout 3 -cmd "connect wizard" -cmd "; return \$waif;"
./moo_client.exe -port 8800 -timeout 3 -cmd "connect wizard" -cmd "; return parents(\$waif:new());"
```

3. **Check Barn's waif-related code:**
- `barn/types/waif.go` - Waif type definition
- `barn/builtins/objects.go` - parents(), children() builtins
- `barn/builtins/types.go` - typeof() for waifs
- `barn/vm/` - waif property access, method calls

## Key Questions
1. Does Barn resolve `$waif` to the correct object?
2. Does `$waif:new()` create waifs properly?
3. Do `parents()` and `children()` return E_INVARG for waifs?
4. Does waif property access work correctly?

## Test Commands
```bash
# Server should already be running on 8800, if not:
cd /c/Users/Q/code/barn
./barn_test.exe -db Test.db -port 8800 > server_8800.log 2>&1 &

# Run waif tests
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 8800 -v -k "waif" -x
```

## Output
Write findings and fixes to `./reports/fix-waif-bugs.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
