# Fix exec() Conformance Tests

## Summary

Fixed 2 of 6 failing exec() conformance tests. The remaining 4 failures are not bugs in exec() itself but rather issues with fork variable scope and missing task_stack builtin.

## Test Results

**Before fixes:** 13 passed, 6 failed
**After fixes:** 15 passed, 4 failed

### Tests Fixed

1. **exec_rejects_invalid_binary_string** - Added validation to reject invalid MOO binary string encoding (e.g., `~ZZ`)
2. **exec_with_sleep_works** - Fixed line ending normalization (Windows CRLF → Unix LF) and MOO binary string encoding in output

### Tests Still Failing (Not exec() bugs)

These 4 tests fail due to issues in fork/task management, not exec():

1. **kill_task_works_on_suspended_exec** - E_VARNF: fork variable `go` not found
2. **kill_task_fails_on_already_killed** - E_VARNF: fork variable `go` not found
3. **resume_fails_on_suspended_exec** - E_VARNF: fork variable `go` not found
4. **suspended_exec_task_stack_matches_suspended_task** - E_VERBNF: task_stack() builtin not implemented

The fork variable scope issue prevents `fork go (0) ... endfork` from making the `go` variable available in the outer scope. This is a parser/evaluator issue, not an exec() issue.

## Changes Made

### 1. Binary String Validation (C:\Users\Q\code\barn\builtins\system.go)

Added `isValidBinaryString()` function to validate that exec() input strings contain only valid MOO binary string encoding:
- Regular characters are allowed
- `~XX` sequences where XX are hex digits (0-9, A-F, a-f) are allowed
- Invalid sequences like `~ZZ` now return E_INVARG

### 2. Output Encoding (C:\Users\Q\code\barn\builtins\system.go)

Normalize Windows line endings in exec() output:
```go
// Normalize line endings to Unix format (LF only)
// MOO expects \n, but Windows produces \r\n
stdoutStr := strings.ReplaceAll(stdout.String(), "\r\n", "\n")
stderrStr := strings.ReplaceAll(stderr.String(), "\r\n", "\n")
```

### 3. MOO String Encoding (C:\Users\Q\code\barn\types\str.go)

Updated `StrValue.String()` to properly encode non-printable characters using MOO's `~XX` notation:
- Printable ASCII (32-126) → output as-is
- Non-printable (< 32 or > 126) → encode as `~XX` (e.g., newline = `~0A`)
- Special cases: `"` → `\"`, `\` → `\\`

This ensures strings returned over the network protocol use MOO's binary string encoding format.

### 4. Test Infrastructure (C:\Users\Q\code\barn\executables\)

Copied test helper programs from ToastStunt:
- `test_io.bat` - Reads stdin, writes to stdout/stderr
- `test_args.bat` - Echoes command line arguments
- `test_exit_status.bat` - Exits with specified code
- `test_with_sleep.bat` - Sleeps and outputs incrementing numbers
- `test_echo.bat`, `echo.bat`, `true.bat` - Supporting utilities

These scripts are found in the `executables/` directory, matching exec()'s path resolution logic.

## Commits

1. **289cf98** - Fix exec() to validate binary string encoding in input
2. **963e07c** - Fix exec() output encoding and Windows line endings

## Issues Outside Scope

The remaining test failures require fixes to:

1. **Fork variable scope** - Variables assigned in `fork VAR (delay)` syntax need to be available in the parent scope after the fork statement executes. This is likely a parser or evaluator issue in how fork statements handle variable assignment.

2. **task_stack() builtin** - This builtin is referenced in tests but not implemented in Barn. It should return the call stack for a given task ID.

## Verification

Run exec tests:
```bash
cd C:\Users\Q\code\barn
go build -o barn.exe ./cmd/barn/
./barn.exe -db Test.db -port 9307 > barn_9307.log 2>&1 &

cd C:\Users\Q\code\cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9307 -k "exec" -v
```

**Result:** 15 passed, 4 failed, 4 skipped

The 4 failures are fork/task management issues, not exec() bugs. The exec() builtin itself is working correctly.
