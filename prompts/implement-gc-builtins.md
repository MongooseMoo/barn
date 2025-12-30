# Task: Implement GC Builtins

## Context
23 tests failing because `run_gc()` and `gc_stats()` builtins are not implemented.

## Objective
Implement `run_gc()` and `gc_stats()` builtins.

## ToastStunt Reference

Look at ToastStunt for the expected behavior:
```bash
grep -n "bf_run_gc\|bf_gc_stats" /c/Users/Q/src/toaststunt/src/functions.cc
```

### run_gc()
- Requires wizard permissions
- Triggers garbage collection of cyclic references in anonymous objects
- Returns nothing (or possibly stats)

### gc_stats()
- Requires wizard permissions
- Returns a map with GC statistics including:
  - "purple" - count of purple (possible root) objects
  - "black" - count of black (confirmed garbage) objects

## Implementation

1. Create `builtins/gc.go` or add to appropriate existing file

2. run_gc signature:
```go
func builtinRunGC(e *Evaluator, args []Value) (Value, error)
// Check wizard perms
// Trigger GC (or no-op if Go's GC handles it)
// Return 0 or stats
```

3. gc_stats signature:
```go
func builtinGCStats(e *Evaluator, args []Value) (Value, error)
// Check wizard perms
// Return map with "purple" and "black" keys (can be 0)
```

4. Register in registry.go

## Tests Expecting

From test names:
- run_gc_requires_wizard_perms - non-wizard gets E_PERM
- run_gc_allows_wizard - wizard can call it
- gc_stats_requires_wizard_perms - non-wizard gets E_PERM
- gc_stats_allows_wizard - wizard can call it
- gc_stats_returns_map - returns a map
- gc_stats_has_purple_key - map has "purple" key
- gc_stats_has_black_key - map has "black" key
- gc_stats_purple_is_int - purple value is integer
- gc_stats_black_is_int - black value is integer

## Verification

```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
# restart server

./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return run_gc();"
./moo_client.exe -port 9300 -cmd "; return gc_stats();"

cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/test_gc.py --transport socket --moo-port 9300 -v
```

## After Fix Verified

Commit:
```bash
git add builtins/gc.go builtins/registry.go
git commit -m "Implement run_gc() and gc_stats() builtins

- run_gc() triggers garbage collection (wizard only)
- gc_stats() returns GC statistics map (wizard only)
- Both require wizard permissions, return E_PERM otherwise"
```

## Output
Write status to `./reports/implement-gc-builtins.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
5. If all formats fail, STOP and report - do not use bash workarounds
