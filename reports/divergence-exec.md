# Divergence Report: exec

**Spec File**: `spec/builtins/exec.md`
**Barn Files**: `builtins/system.go` (lines 154-389)
**Status**: divergences_found
**Date**: 2026-01-03

## Summary

Tested the `exec()` builtin and related functions documented in the spec. Found 9 behaviors tested, with 0 Barn-vs-Toast divergences but multiple spec gaps. The basic `exec()` function works identically between Barn and Toast, but many features documented in the spec (exec_async, exec_timeout, kill_process, wait_process, exec_env, string form) are either unimplemented or non-functional in both servers.

## Divergences

### No Barn-vs-Toast Divergences Found

All tested behaviors matched identically between Barn (port 9500) and Toast (port 9501):
- Nonexistent program rejection
- Path validation (absolute, ./, ../)
- Basic I/O with test_io
- Argument passing with test_args
- Exit status handling
- Binary string validation

## Spec Gaps

Behaviors documented in spec but NOT matching actual implementation:

### 1. String Form exec() - Documented but Non-Functional

**Spec Section**: 1.2 "Command Forms"
**Spec Says**: `exec("program arg1 arg2")` - string form passed to shell, supports pipes, redirects
**Reality**: String form is rejected with E_INVARG on both servers

| Test | `exec("echo hello")` |
|------|---------------------|
| Barn | E_INVARG |
| Toast | E_INVARG |
| Evidence | Code in system.go:192-194 tries to use "sh" but validateAndResolvePath() rejects it because "sh" is not in executables/ directory |

**Conclusion**: Spec documents string form as working, but both implementations require shell executable to be in executables/ directory, which isn't standard. String form is effectively non-functional.

### 2. exec_async() - Documented but Unimplemented

**Spec Section**: 2.1 "exec_async (ToastStunt)"
**Spec Says**: `exec_async(command [, callback_obj, callback_verb]) → INT`
**Reality**: Function does not exist

| Test | `exec_async({"test_io"})` |
|------|---------------------------|
| Barn | E_VERBNF |
| Toast | E_VERBNF |
| Evidence | registry.go only registers "exec", not "exec_async" |

### 3. exec_timeout() - Documented but Unimplemented

**Spec Section**: 2.2 "exec_timeout (ToastStunt)"
**Spec Says**: `exec_timeout(command, timeout [, input]) → LIST`
**Reality**: Function does not exist, but exec() has hardcoded 30s timeout

| Test | `exec_timeout({"test_io"}, 5)` |
|------|--------------------------------|
| Barn | E_VERBNF |
| Toast | E_VERBNF |
| Evidence | Code shows ctx.WithTimeout(30*time.Second) hardcoded at system.go:351 |

**Note**: exec() itself has a 30-second timeout, but exec_timeout() doesn't exist to customize it.

### 4. kill_process() - Documented but Unimplemented

**Spec Section**: 3.1 "kill_process (ToastStunt)"
**Spec Says**: `kill_process(pid [, signal]) → none`
**Reality**: Function does not exist

| Test | `kill_process(123)` |
|------|---------------------|
| Barn | E_VERBNF |
| Toast | E_VERBNF |

### 5. wait_process() - Documented but Unimplemented

**Spec Section**: 3.2 "wait_process (ToastStunt)"
**Spec Says**: `wait_process(pid) → LIST`
**Reality**: Function does not exist

| Test | `wait_process(123)` |
|------|---------------------|
| Barn | E_VERBNF |
| Toast | E_VERBNF |

### 6. exec_env() - Documented but Unimplemented

**Spec Section**: 4.1 "exec_env (ToastStunt)"
**Spec Says**: `exec_env(command, env [, input]) → LIST`
**Reality**: Function does not exist

| Test | `exec_env({"test_io"}, [])` |
|------|----------------------------|
| Barn | E_VERBNF |
| Toast | E_VERBNF |

### 7. Permission Error Code - Spec vs Conformance Test Mismatch

**Spec Section**: 1.1 "Errors" - E_PERM: Not allowed to exec
**Conformance Test**: exec.yaml line 13 expects E_INVARG for non-wizards
**Reality**: Cannot test easily with moo_client (always connects as wizard)

**Conclusion**: Spec says E_PERM for permission denied, but conformance test expects E_INVARG. Need to verify which is correct.

## Test Coverage Gaps

Behaviors documented in spec but NOT covered by current conformance tests:

- **String form execution**: `exec("command with args")` - documented but doesn't work
- **Timeout behavior**: exec() has 30s hardcoded timeout, not tested
- **Output size limits**: Spec section 8 mentions output size limits, not tested
- **Concurrent exec limits**: Spec section 8 mentions max concurrent processes, not tested
- **Binary string encoding**: Only invalid encoding tested, valid ~XX sequences need more tests
- **Path traversal edge cases**: `foo/../../test_io` tested but more complex traversals could be added
- **Windows-specific behavior**: .bat/.cmd file execution, PATHEXT handling

## Behaviors Verified Correct

All core exec() behaviors match identically between Barn and Toast:

1. ✓ **Nonexistent program**: `exec({"nonexistent"})` → E_INVARG
2. ✓ **Absolute path rejection**: `exec({"/test_io"})` → E_INVARG
3. ✓ **Dot-slash rejection**: `exec({"./test_io"})` → E_INVARG
4. ✓ **Parent dir rejection**: `exec({"../test_io"})` → E_INVARG
5. ✓ **Basic I/O**: `exec({"test_io"}, "Hello, world!")` → {0, "Hello, world!", "Hello, world!"}
6. ✓ **Argument passing**: `exec({"test_args", "one", "two", "three"}, "")` → {0, "one two three", ""}
7. ✓ **Exit status**: `exec({"test_exit_status", "2"}, "")` → {2, "", ""}
8. ✓ **Invalid binary string**: `exec({"test_io"}, "1~ZZ23~0A")` → E_INVARG
9. ✓ **Valid binary string**: `exec({"test_io"}, "Hello~0AWorld")` → {0, "Hello~0AWorld", "Hello~0AWorld"}

## Implementation Notes

### Barn Implementation Details (system.go)

- **Path validation** (lines 248-326): Rejects absolute paths, ./, ../, path traversal
- **Binary string validation** (lines 223-246): Validates ~XX hex encoding
- **Timeout**: Hardcoded 30 seconds (line 351)
- **Executables directory**: All programs must be in `executables/` subdirectory
- **Windows handling**: Automatically uses cmd.exe for .bat/.cmd files
- **Line ending normalization**: Converts \r\n to \n (lines 377-380)

### Security Model

Both servers implement:
- Wizard-only execution (checked but error code uncertain)
- Path sandboxing (executables/ directory only)
- No absolute paths
- No path traversal (./ or ../)
- Binary string validation

## Recommendations

1. **Update Spec**: Remove or mark as unimplemented:
   - exec_async()
   - exec_timeout()
   - kill_process()
   - wait_process()
   - exec_env()
   - String form execution (or document shell requirement)

2. **Clarify Permission Error**: Spec says E_PERM, test expects E_INVARG - determine correct behavior

3. **Document Timeout**: Spec mentions exec_timeout() but actual timeout is hardcoded 30s in exec()

4. **Add Tests**:
   - Timeout behavior
   - Output size limits
   - Valid binary string sequences
   - Windows-specific .bat/.cmd handling

5. **Fix or Document String Form**: Either:
   - Add shell to executables/ directory and document requirement
   - Remove string form from spec
   - Change implementation to allow shell without path validation
