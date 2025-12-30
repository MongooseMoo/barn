# Task: Implement object_bytes Builtin

## Context
9 tests failing because `object_bytes()` builtin is not implemented.

## Objective
Implement `object_bytes(obj)` builtin that returns the approximate memory size of an object.

## ToastStunt Reference

```bash
grep -n "bf_object_bytes" /c/Users/Q/src/toaststunt/src/functions.cc
```

### object_bytes(obj)
- Takes an object reference
- Returns integer representing approximate bytes used by the object
- Requires wizard permissions (or maybe just read access?)
- Returns E_INVARG for invalid/recycled objects

## Tests Expecting

From test names:
- object_bytes_permission_denied - non-wizard gets E_PERM
- object_bytes_wizard_allowed - wizard can call it
- object_bytes_type_int - works with various types
- object_bytes_type_float - works with various types
- object_bytes_type_string - works with various types
- object_bytes_recycled_object - recycled returns E_INVARG
- object_bytes_created_objects - works on created objects

## Implementation

```go
func builtinObjectBytes(e *Evaluator, args []Value) (Value, error) {
    // Check arg count (1)
    // Check wizard permissions
    // Get object from arg
    // Check if recycled -> E_INVARG
    // Calculate approximate byte size
    // Return integer
}
```

The byte size can be an estimate based on:
- Number of properties * average property size
- Number of verbs * average verb size
- Object header overhead

## Verification

```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
# restart server

./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return object_bytes(#0);"

cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/test_stress_objects.py -k "object_bytes" --transport socket --moo-port 9300 -v
```

## After Fix Verified

Commit:
```bash
git add builtins/objects.go builtins/registry.go  # or wherever implemented
git commit -m "Implement object_bytes() builtin

Returns approximate memory size of an object in bytes.
Requires wizard permissions, returns E_INVARG for recycled objects."
```

## Output
Write status to `./reports/implement-object-bytes.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
5. If all formats fail, STOP and report - do not use bash workarounds
