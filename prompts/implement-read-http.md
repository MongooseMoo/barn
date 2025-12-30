# Task: Implement read_http Builtin

## Context
6 tests failing because `read_http()` builtin is not implemented.

## Objective
Implement `read_http()` builtin for HTTP connection handling.

## ToastStunt Reference

```bash
grep -n "bf_read_http" /c/Users/Q/src/toaststunt/src/functions.cc
```

### read_http([type [, connection]])
- Reads HTTP request data from a connection
- Type argument specifies what to read
- Without args, requires wizard permissions
- Returns E_INVARG for invalid types

## Tests Expecting

From test names:
- non_wizard_cannot_call_no_arg_version - non-wizard no-args gets E_PERM
- read_http_no_args_fails - some argument validation
- read_http_invalid_type_foobar - "foobar" as type is invalid
- read_http_invalid_type_empty_string - "" as type is invalid
- read_http_type_arg_not_string - type must be string
- read_http_connection_arg_not_obj - connection must be object

## Implementation

```go
func builtinReadHTTP(e *Evaluator, args []Value) (Value, error) {
    // If no args: require wizard permissions
    // If type arg provided: validate it's a string
    // If connection arg provided: validate it's an object
    // Valid types: check ToastStunt for valid type strings
    // Return E_INVARG for invalid types
}
```

## Verification

```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
# restart server

cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/test_http.py --transport socket --moo-port 9300 -v
```

## After Fix Verified

Commit:
```bash
git add builtins/*.go
git commit -m "Implement read_http() builtin

Reads HTTP request data from a connection.
Validates type and connection arguments, requires wizard for no-arg form."
```

## Output
Write status to `./reports/implement-read-http.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
5. If all formats fail, STOP and report - do not use bash workarounds
