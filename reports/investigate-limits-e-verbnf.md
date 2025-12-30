# Investigation Report: Limits E_VERBNF Root Cause

## Date
2025-12-30

## Summary
39 conformance tests for limits are failing with `E_VERBNF` (Verb Not Found) instead of either passing or returning `E_QUOTA`. The root cause is that **critical builtins required for limits testing are not registered** in Barn's builtin registry.

**TL;DR:** Three builtins are implemented but not wired up:
- `load_server_options()` - registration function exists but is commented out with TODO
- `value_bytes()` - implemented but never registered
- `substitute()` - implemented but never registered

**Fix:** Uncomment one line in `vm/eval.go` and add two registrations in `builtins/registry.go`.

## Root Cause

### Issue 1: `load_server_options()` Not Registered

The `load_server_options()` builtin is implemented in `builtins/system.go` and added to the registry via `RegisterSystemBuiltins()`, but **this function is never called during server initialization**.

**Evidence:**

```bash
$ ./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return load_server_options();"
{2, {E_VERBNF, "", 0}}
```

**Location in code:**

File: `vm/eval.go`, lines 26, 46, 65, 84

All four evaluator constructors have this line commented out:
```go
// registry.RegisterSystemBuiltins(store) // TODO: Implement when needed
```

The builtin exists and is properly implemented in `builtins/system.go:434`:
```go
func builtinLoadServerOptions(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result
```

And is registered in `builtins/registry.go:228`:
```go
func (r *Registry) RegisterSystemBuiltins(store *db.Store) {
    r.Register("load_server_options", func(ctx *types.TaskContext, args []types.Value) types.Result {
        return builtinLoadServerOptions(ctx, args, store)
    })
}
```

**But the registration function is never called**, leaving the builtin unavailable.

### Issue 2: `value_bytes()` Not Registered At All

The `value_bytes()` builtin is implemented in `builtins/limits.go:142`:
```go
func builtinValueBytes(ctx *types.TaskContext, args []types.Value) types.Result {
    if len(args) != 1 {
        return types.Err(types.E_ARGS)
    }
    size := ValueBytes(args[0])
    return types.Ok(types.NewInt(int64(size)))
}
```

**Evidence:**
```bash
$ ./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return value_bytes({1, 2, 3});"
{2, {E_VERBNF, "", 0}}
```

However, **`value_bytes` is never registered in any Registry function**. It's not in:
- `NewRegistry()` main registration (builtins/registry.go:26-167)
- `RegisterSystemBuiltins()` (builtins/registry.go:228-232)
- Any other registration function

### Issue 3: `substitute()` Also Not Registered

Similarly, the `substitute()` builtin is implemented in `builtins/strings.go:748` but is not registered anywhere in `builtins/registry.go`.

**Registration Status of Required Builtins:**
```
setadd:              REGISTERED ✓
listinsert:          REGISTERED ✓
listappend:          REGISTERED ✓
listset:             REGISTERED ✓
setremove:           REGISTERED ✓
listdelete:          REGISTERED ✓
decode_binary:       REGISTERED ✓
mapdelete:           REGISTERED ✓
tostr:               REGISTERED ✓
toliteral:           REGISTERED ✓
strsub:              REGISTERED ✓
encode_binary:       REGISTERED ✓
substitute:          NOT REGISTERED ✗
encode_base64:       REGISTERED ✓
random_bytes:        REGISTERED ✓
value_bytes:         NOT REGISTERED ✗
load_server_options: REGISTERED (but not enabled) ✗
```

Three builtins are missing from registration.

## Impact

All 39 limits conformance tests require these builtins:

From `tests/conformance/server/limits.yaml`:
```yaml
requires:
  builtins: [setadd, listinsert, listappend, listset, setremove, listdelete,
             decode_binary, mapdelete, tostr, toliteral, strsub, encode_binary,
             substitute, encode_base64, random_bytes, value_bytes, load_server_options]

setup:
  permission: wizard
  code: |
    add_property($server_options, "max_concat_catchable", 1, {player, "r"});
    add_property($server_options, "max_string_concat", 1000000, {player, "r"});
    add_property($server_options, "max_list_value_bytes", 1000000, {player, "r"});
    add_property($server_options, "max_map_value_bytes", 1000000, {player, "r"});
    add_property($server_options, "fg_seconds", 123, {player, "r"});
    add_property($server_options, "fg_ticks", 2147483647, {player, "r"});
    load_server_options();  # <-- E_VERBNF here
```

Every test fails during setup because `load_server_options()` returns E_VERBNF.

Tests also use `value_bytes()` to measure object sizes, which also returns E_VERBNF.

## Examples of Affected Tests

1. **string_concat_limit** - Expected to pass with value 3210, returns E_VERBNF
2. **string_concat_exceeds_limit** - Expected E_QUOTA, returns E_VERBNF
3. **tostr_exceeds_limit** - Expected E_QUOTA, returns E_VERBNF
4. **setadd_checks_list_max_value_bytes_small** - Expected int result, returns E_VERBNF
5. **setadd_checks_list_max_value_bytes_exceeds** - Expected E_QUOTA, returns E_VERBNF

All 39 tests follow this pattern.

## Recommended Fix

### Fix 1: Enable RegisterSystemBuiltins

In `vm/eval.go`, uncomment the `RegisterSystemBuiltins()` call in all four constructor functions:

**Lines to change: 26, 46, 65, 84**

Change:
```go
// registry.RegisterSystemBuiltins(store) // TODO: Implement when needed
```

To:
```go
registry.RegisterSystemBuiltins(store)
```

### Fix 2: Register value_bytes Builtin

Add `value_bytes` to the appropriate registration location.

**Option A:** Add to `RegisterSystemBuiltins()` in `builtins/registry.go:228`:
```go
func (r *Registry) RegisterSystemBuiltins(store *db.Store) {
    r.Register("load_server_options", func(ctx *types.TaskContext, args []types.Value) types.Result {
        return builtinLoadServerOptions(ctx, args, store)
    })
    r.Register("value_bytes", builtinValueBytes)  // <-- Add this
}
```

**Option B:** Add to main `NewRegistry()` function in `builtins/registry.go` around line 141 (near other system builtins):
```go
// Register system builtins
r.Register("getenv", builtinGetenv)
r.Register("task_local", builtinTaskLocal)
r.Register("set_task_local", builtinSetTaskLocal)
r.Register("value_bytes", builtinValueBytes)  // <-- Add this
r.Register("task_id", builtinTaskID)
```

**Recommendation:** Option B is better because `value_bytes()` doesn't need store access (it only takes a single argument and computes size), so it doesn't need to be in the store-dependent registration function.

### Fix 3: Register substitute Builtin

Add `substitute` to the string builtins section in `NewRegistry()` in `builtins/registry.go` around line 58 (near other string builtins like `strsub`):
```go
r.Register("match", builtinMatch)
r.Register("rmatch", builtinRmatch)
r.Register("substitute", builtinSubstitute)  // <-- Add this
```

## Verification Steps

After applying fixes:

1. Rebuild barn:
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
```

2. Start server:
```bash
./barn_test.exe -db Test.db -port 9300 > test.log 2>&1 &
sleep 2
```

3. Test load_server_options:
```bash
./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return load_server_options();"
# Current: {2, {E_VERBNF, "", 0}}
# Expected after fix: 0 (or number of options loaded)
```

4. Test value_bytes:
```bash
./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return value_bytes({1, 2, 3});"
# Current: {2, {E_VERBNF, "", 0}}
# Expected after fix: integer (e.g., 48)
```

5. Test substitute:
```bash
./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return substitute(\"%1\", {1, 2, {}, \"hello\"});"
# Current: {2, {E_VERBNF, "", 0}}
# Expected after fix: "hello"
```

6. Run limits tests:
```bash
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/test_conformance.py -k limits -v
# Current: 39 failures with E_VERBNF
# Expected after fix: Tests should either pass or properly return E_QUOTA
```

## Additional Context

The TODO comment "Implement when needed" suggests this was intentionally deferred, but:

1. The implementation **is complete** - both functions are fully implemented
2. The conformance tests **require these builtins**
3. The registration infrastructure **already exists**
4. The fix is **trivial** - just uncomment one line and add one registration

The "when needed" time is now - 39 tests are blocked waiting for this.

## Files to Modify

1. `vm/eval.go` - Lines 26, 46, 65, 84 (uncomment RegisterSystemBuiltins)
2. `builtins/registry.go` - Line ~143 (add value_bytes registration)
3. `builtins/registry.go` - Line ~58 (add substitute registration)

## Conclusion

The E_VERBNF errors in limits tests are caused by missing builtin registrations, not by bugs in the limits implementation itself. The core limits code appears to be correctly implemented - it just can't be tested because the required builtins are not wired up to the evaluator.

**Three missing registrations:**
1. `load_server_options()` - implemented but registration function never called
2. `value_bytes()` - implemented but never registered
3. `substitute()` - implemented but never registered

This is a **registration issue**, not a **logic issue**. All three builtins have complete implementations ready to use.
