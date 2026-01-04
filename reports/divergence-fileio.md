# Divergence Report: File I/O Builtins

**Spec File**: C:\Users\Q\code\barn\spec\builtins\fileio.md
**Barn Files**: None (not implemented)
**Toast Source**: C:\Users\Q\src\toaststunt\src\fileio.cc
**Status**: not_implemented
**Date**: 2026-01-03

## Summary

File I/O builtins are **NOT IMPLEMENTED** in either Barn or in the running Test.db servers. The conformance test suite confirms that "File I/O builtins are NOT available in toaststunt's Test.db (requires server config)" - all 48 file I/O tests are skipped.

ToastStunt has file I/O builtins compiled into the source code, but they are:
1. Disabled by default in server configuration
2. Not available in Test.db used for conformance testing
3. Require server configuration to enable

Barn has no file I/O implementation at all (no `builtins/fileio.go` file exists).

**Key Finding**: The spec in `spec/builtins/fileio.md` documents file I/O as if it's standard functionality, but it's actually an optional feature that:
- Requires compile-time and runtime configuration
- Is disabled in reference servers
- Has no validation against running servers
- Cannot be divergence-tested without enabling it first

## Functions in Toast Source (Not Enabled)

Toast's `src/fileio.cc` registers these 27 file I/O builtins:

### File Handle Management
- `file_handles()` - list open file handles
- `file_open(str, str)` - open file with path and mode
- `file_close(int)` - close file handle
- `file_name(int)` - get file path from handle
- `file_openmode(int)` - get mode string from handle

### File Reading
- `file_readline(int)` - read one line
- `file_readlines(int, int, int)` - read multiple lines
- `file_read(int, int)` - read bytes
- `file_grep(int, str [, int])` - grep pattern in file
- `file_count_lines(int)` - count lines in file

### File Writing
- `file_writeline(int, str)` - write line with newline
- `file_write(int, str)` - write data without newline

### File Position
- `file_tell(int)` - get current position
- `file_seek(int, int, str)` - set position
- `file_eof(int)` - check if at end
- `file_flush(int)` - flush buffer

### File Information
- `file_size(any)` - get file size (path str or handle int)
- `file_mode(any)` - get file permissions
- `file_type(any)` - get type: "file", "directory", "link", "unknown"
- `file_stat(any)` - get full file metadata
- `file_last_access(any)` - get access time
- `file_last_modify(any)` - get modification time
- `file_last_change(any)` - get change time

### File Management
- `file_rename(str, str)` - rename/move file
- `file_remove(str)` - delete file
- `file_chmod(str, str)` - change permissions

### Directory Operations
- `file_list(str [, any])` - list directory contents
- `file_mkdir(str)` - create directory
- `file_rmdir(str)` - remove directory

### Notable Absences

The spec documents these functions that **DO NOT EXIST** in Toast source:

- `file_exists(path)` - spec says "ToastStunt", but NOT in source
- `file_copy(source, dest)` - spec says "ToastStunt", but NOT in source
- `file_path_info(path)` - spec says "ToastStunt", but NOT in source
- `file_resolve(path)` - spec says "ToastStunt", but NOT in source

## Test Coverage

The conformance test suite has 48 tests in `moo-conformance-tests/src/moo_conformance/_tests/builtins/fileio.yaml`:

- **48 tests total** - ALL marked with `skip: "File I/O not available in Test.db"`
- Permission tests (27): verify all file operations require wizard
- Text file operations (3): basic read/write/readline
- Binary file operations (3): binary mode read/write
- Mixed mode tests (2): binary data in text mode filters non-text
- file_readlines tests (5): line ranges, blank lines, edge cases
- Empty file tests (2): EOF handling for empty files
- Error handling tests: E_INVARG, E_PERM, E_FILE

All tests exist but cannot be validated without:
1. Enabling file I/O in server configuration
2. Configuring allowed file paths (sandboxing)
3. Restarting servers with file I/O enabled

## Spec Issues

### 1. file_exists() - Documented but Doesn't Exist

| Field | Value |
|-------|-------|
| Spec Claims | "file_exists(path) → BOOL" marked as "(ToastStunt)" |
| Toast Source | NO `register_function("file_exists"` in fileio.cc |
| Classification | spec_error |
| Evidence | Grep through Toast source shows no file_exists function |

The spec documents `file_exists()` as a ToastStunt function, but it's not in the source code.

### 2. file_copy() - Documented but Doesn't Exist

| Field | Value |
|-------|-------|
| Spec Claims | "file_copy(source, dest) → none" marked as "(ToastStunt)" |
| Toast Source | NO `register_function("file_copy"` in fileio.cc |
| Classification | spec_error |
| Evidence | Grep through Toast source shows no file_copy function |

### 3. file_path_info() - Documented but Doesn't Exist

| Field | Value |
|-------|-------|
| Spec Claims | "file_path_info(path) → LIST" marked as "(ToastStunt)" |
| Toast Source | NO `register_function("file_path_info"` in fileio.cc |
| Classification | spec_error |
| Evidence | Grep through Toast source shows no file_path_info function |

### 4. file_resolve() - Documented but Doesn't Exist

| Field | Value |
|-------|-------|
| Spec Claims | "file_resolve(path) → STR" marked as "(ToastStunt)" |
| Toast Source | NO `register_function("file_resolve"` in fileio.cc |
| Classification | spec_error |
| Evidence | Grep through Toast source shows no file_resolve function |

### 5. file_grep() - Exists but Not Documented

| Field | Value |
|-------|-------|
| Toast Source | `register_function("file_grep", 2, 3, ...)` |
| Spec | No mention of file_grep() |
| Classification | spec_gap |
| Evidence | Function exists in Toast but spec doesn't document it |

### 6. file_count_lines() - Exists but Not Documented

| Field | Value |
|-------|-------|
| Toast Source | `register_function("file_count_lines", 1, 1, ...)` |
| Spec | No mention of file_count_lines() |
| Classification | spec_gap |
| Evidence | Function exists in Toast but spec doesn't document it |

### 7. file_flush() - Exists but Not Documented

| Field | Value |
|-------|-------|
| Spec | Brief mention in section 3.2 but no full documentation |
| Toast Source | `register_function("file_flush", 1, 1, ...)` |
| Classification | spec_incomplete |
| Evidence | Function exists and is mentioned but lacks full signature/examples |

### 8. file_handles() - Exists but Not Documented

| Field | Value |
|-------|-------|
| Toast Source | `register_function("file_handles", 0, 0, ...)` |
| Spec | No mention of file_handles() |
| Classification | spec_gap |
| Evidence | Function exists in Toast but spec doesn't document it |

### 9. file_name() - Exists but Not Documented

| Field | Value |
|-------|-------|
| Toast Source | `register_function("file_name", 1, 1, ...)` |
| Spec | No mention of file_name() |
| Classification | spec_gap |
| Evidence | Function exists in Toast but spec doesn't document it |

### 10. file_openmode() - Exists but Not Documented

| Field | Value |
|-------|-------|
| Toast Source | `register_function("file_openmode", 1, 1, ...)` |
| Spec | No mention of file_openmode() |
| Classification | spec_gap |
| Evidence | Function exists in Toast but spec doesn't document it |

## Mode String Differences

The spec documents mode strings like:
- "r", "rb", "w", "wb", "a", "ab"

But the conformance tests use extended mode strings:
- "r-tn" (read, text mode, non-blocking?)
- "w-tn" (write, text mode, non-blocking?)
- "r-bn" (read, binary mode, non-blocking?)
- "w-bn" (write, binary mode, non-blocking?)

The spec doesn't document what the "-tn" and "-bn" flags mean. This appears to be a Toast extension.

## Go Implementation Notes in Spec

The spec includes example Go code (lines 346-421) showing how to implement file I/O. This is implementation guidance, not behavior specification. Notable items:

- Shows basic file handle management pattern
- Demonstrates mode string parsing
- Shows security check: `isAllowedPath(path)` for sandboxing
- Example bufio usage for reading lines
- Basic error handling (E_FILE, E_INVARG, E_PERM)

## Testing Status

**Cannot test for divergences** because:

1. File I/O is disabled in both Test.db servers (Barn on 9500, Toast on 9501)
2. Testing file I/O requires:
   - Server restart with file I/O enabled
   - Configuration of allowed file paths
   - Test file creation/cleanup coordination

**Evidence from moo_client tests**:
```
# Toast (port 9501) - attempting file_handles()
Result: E_VERBNF - function doesn't exist in running server

# Barn (port 9500) - attempting file_handles()
Result: E_VERBNF - function doesn't exist in running server
```

## Recommendations

### For Spec

1. **Remove non-existent functions**: file_exists, file_copy, file_path_info, file_resolve
2. **Add missing functions**: file_handles, file_name, file_openmode, file_grep, file_count_lines
3. **Document mode string extensions**: Explain "-tn" and "-bn" flags used in tests
4. **Clarify optional status**: Make it clear that file I/O is compile-time and runtime optional
5. **Document configuration**: How to enable file I/O, path sandboxing, security model
6. **Complete file_flush documentation**: Currently just mentioned, needs full spec

### For Implementation

1. **Barn**: File I/O is not implemented (no `builtins/fileio.go`)
2. **When implementing**: Use Toast's function list as reference (27 functions)
3. **Don't implement**: file_exists, file_copy, file_path_info, file_resolve (don't exist in Toast)
4. **Configuration needed**: Path sandboxing, permission checks, enable/disable flag

### For Testing

1. **Current state**: Cannot validate - all 48 tests skipped
2. **To enable testing**: Need server configuration changes
3. **Test files**: All tests exist in conformance suite, just need server with file I/O enabled
4. **Coordination**: Tests create/delete files, need isolated test directory

## Conclusion

File I/O is a **compile-time optional feature** that:
- Exists in Toast source but is disabled in Test.db
- Is not implemented in Barn at all
- Has 48 conformance tests but all are skipped
- Has significant spec errors (4 non-existent functions documented)
- Has spec gaps (6 existing functions not documented)

**No divergence testing is possible** until file I/O is enabled in at least one server. The spec should be updated to:
1. Remove functions that don't exist in Toast
2. Add functions that do exist in Toast
3. Clarify this is an optional feature requiring configuration
4. Document the extended mode string syntax

The current spec presents file I/O as standard functionality when it's actually a disabled optional feature with incomplete documentation.
