# Task: Detect Divergences in File I/O Builtins

## Context

We need to verify Barn's file I/O builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all file I/O builtins.

## Files to Read

- `spec/builtins/fileio.md` - expected behavior specification
- `builtins/fileio.go` - Barn implementation (if exists)

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### File Operations
- `file_open()` - open file handle
- `file_close()` - close file handle
- `file_read()` - read from file
- `file_readline()` - read line
- `file_write()` - write to file
- `file_writeline()` - write line
- `file_tell()` - get position
- `file_seek()` - set position
- `file_eof()` - check end of file
- `file_flush()` - flush buffer

### File System
- `file_list()` - list directory
- `file_mkdir()` - create directory
- `file_rmdir()` - remove directory
- `file_remove()` - delete file
- `file_rename()` - rename file
- `file_chmod()` - change permissions
- `file_size()` - get file size
- `file_stat()` - get file info
- `file_exists()` - check existence

## Edge Cases to Test

- Invalid file handles
- Permission errors
- Path traversal attempts
- Binary vs text mode
- Large files
- Non-existent paths

## Testing Commands

```bash
# Toast oracle - check if functions exist
./toast_oracle.exe 'file_exists(".")'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return file_exists(\".\");"

# Check conformance tests
grep -r "file_" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-fileio.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test available builtins SAFELY (no destructive operations)
- Many of these may be ToastStunt-only and not exist in either server
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
