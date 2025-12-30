# Task: Fix exec() Tests Using Toast's Test Binaries

## Context
Barn is a Go MOO server. The exec() builtin is implemented but tests fail because they need test binaries (test_io, test_args, etc.) that exist in Toast's test directory.

## Objective
Get exec() tests passing by:
1. Finding Toast's test binaries/scripts
2. Copying or referencing them for Barn's tests
3. Fixing any remaining exec() issues

## Current State
- exec() is implemented in `builtins/system.go`
- 7 security tests pass
- Tests like exec_io_works, exec_args_works fail with E_INVARG because binaries don't exist

## Reference Implementation
ToastStunt location: `~/src/toaststunt/`
- Check `test/` directory for test helper programs
- Look for test_io, test_args, test_exit_status, etc.

## Test Files to Find
The conformance tests reference these:
- `test_io` - reads stdin, writes to stdout/stderr
- `test_args` - echoes command line arguments
- `test_exit_status` - exits with specific code
- `test_sleep` - sleeps for a duration

## Implementation Steps

1. **Find Toast's test helpers**:
   ```bash
   find ~/src/toaststunt -name "test_*" -type f
   ls ~/src/toaststunt/test/
   ```

2. **Copy or create equivalent test programs** for Barn
   - These might be simple shell scripts or C programs
   - Place them somewhere Barn's tests can find them

3. **Check exec() path handling**:
   - Does Barn look in the right places for executables?
   - May need to copy binaries to working directory

4. **Run tests**:
   ```bash
   cd /c/Users/Q/code/cow_py && uv run pytest tests/conformance/ --transport socket --moo-port 9650 -v -k "exec" 2>&1
   ```

## Files to Examine
- `C:\Users\Q\code\barn\builtins\system.go` - exec implementation
- `C:\Users\Q\code\cow_py\tests\conformance\server\exec.yaml` - test definitions

## Output
Write findings and implementation status to `./reports/fix-exec-tests.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
