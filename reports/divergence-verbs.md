# Divergence Report: Verb Builtins

**Spec File**: spec/builtins/verbs.md
**Barn Files**: builtins/verbs.go
**Status**: divergences_found
**Date**: 2026-01-03

## Summary

Tested verb manipulation builtins (verbs, verb_info, verb_args, verb_code, add_verb, delete_verb, set_verb_info, set_verb_args, set_verb_code, disassemble) against Barn (port 9500) and Toast (port 9501).

**Key Findings:**
- 4 likely Barn bugs (missing verbs, wrong owner, missing permission checks)
- 1 likely spec gap (prep spec validation behavior not documented)
- Strong test coverage exists in conformance suite
- Core functionality works correctly

**Note**: toast_oracle.exe tool is broken for complex expressions - it returns the last evaluated expression instead of the return value. This made systematic Toast testing difficult. Relied on Barn behavior and conformance test expectations instead.

## Divergences

### 1. verbs() - Missing verbs on #0

| Field | Value |
|-------|-------|
| Test | `verbs(#0)` |
| Barn | `{"do_login_command", "handle_uncaught_error", "handle_task_timeout"}` (3 verbs) |
| Toast | 27 verbs including all the above plus many more |
| Classification | likely_barn_bug |
| Evidence | Toast returns: `{"do_login_command", "server_started", "core_object_info core_objects", "init_for_core", "user_created user_connected", ...}` (27 total). Barn is missing 24 verbs that exist in Test.db. This is a database loading issue, not a builtin issue. |

### 2. verb_info() - Wrong owner object

| Field | Value |
|-------|-------|
| Test | `verb_info(#0, "do_login_command")` |
| Barn | `{#3, "rxd", "do_login_command"}` (owner=#3) |
| Toast | `{#2, "rxd", "do_login_command"}` (owner=#2) |
| Classification | likely_barn_bug |
| Evidence | In Test.db, wizard is #2. Barn appears to be creating a new wizard as #3 on each connection instead of using the existing #2. This affects verb owners throughout the database. |

### 3. verb_code() - Different line count

| Field | Value |
|-------|-------|
| Test | `length(verb_code(#0, "do_login_command"))` |
| Barn | 37 lines |
| Toast | 42 lines |
| Classification | likely_barn_bug |
| Evidence | Same verb, different code length. Either Barn is not loading all lines, or there's a difference in how blank lines or comments are stored. Related to missing verbs issue above. |

### 4. delete_verb() - Missing permission check

| Field | Value |
|-------|-------|
| Test | Code review |
| Barn | Line 488: `// TODO: Check permissions (must be owner or wizard)` |
| Toast | Implements proper permission checks |
| Classification | likely_barn_bug |
| Evidence | Barn code has explicit TODO comment. Conformance tests exist but are skipped: "Owners can always delete verbs regardless of .w flag - need non-owner test". Barn currently allows anyone to delete any verb. |

### 5. set_verb_info() - Missing permission check

| Field | Value |
|-------|-------|
| Test | Code review |
| Barn | Line 536: `// TODO: Check permissions (must be owner or wizard)` |
| Toast | Implements proper permission checks (wizard only) |
| Classification | likely_barn_bug |
| Evidence | Barn code has explicit TODO comment. Conformance tests indicate this requires wizard permission, not just ownership. Test at line 457-471 shows wizard-only requirement. |

### 6. set_verb_args() - Missing permission check

| Field | Value |
|-------|-------|
| Test | Code review |
| Barn | Line 602: `// TODO: Check permissions (must be owner or wizard)` |
| Toast | Implements proper permission checks |
| Classification | likely_barn_bug |
| Evidence | Barn code has explicit TODO comment. Conformance tests exist but are skipped for non-owner scenarios. |

### 7. set_verb_code() - Missing permission check

| Field | Value |
|-------|-------|
| Test | Code review |
| Barn | Line 652: `// TODO: Check permissions (must be owner or wizard)` |
| Toast | Implements proper permission checks |
| Classification | likely_barn_bug |
| Evidence | Barn code has explicit TODO comment. Conformance tests exist but are skipped for non-owner scenarios. |

### 8. add_verb() - Prep spec validation not documented

| Field | Value |
|-------|-------|
| Test | `add_verb(o, {player, "x", "test"}, {"any", "with/using", "this"})` |
| Barn | Returns E_INVARG |
| Toast | (Unable to test with broken oracle) |
| Classification | likely_spec_gap |
| Evidence | Spec lists prep values like "with/using" but doesn't clarify whether the full string or individual components are valid when calling add_verb(). Barn's code (lines 398-406) validates prep specs but accepts only individual prep names ("with" or "using"), not the full slash-separated string. This is correct behavior but not documented. |

## Test Coverage Gaps

The conformance test suite (builtins/verbs.yaml) provides excellent coverage with 76 tests. However, several tests are skipped due to test design limitations:

### Skipped Tests Due to "Need Non-Owner Test"

These tests verify permission behavior for non-owners but are skipped because the test framework doesn't easily support multi-user scenarios:

- `add_verb_no_write_permission` (line 72-75)
- `delete_verb_no_write_permission` (line 177-180)
- `verb_info_no_read_permission` (line 248-251)
- `verb_args_no_read_permission` (line 317-320)
- `verb_code_no_read_permission` (line 386-389)
- `verbs_no_read_permission` (line 646-649)
- `set_verb_args_no_write_permission` (line 514-517)
- `set_verb_code_no_write_permission` (line 586-589)

### Other Skipped Tests

- `add_verb_not_owner` (line 103-113) - "Toast allows programmer to specify different owner" - behavior unclear
- `set_verb_info_basic` (line 419-422) - "Needs wizard permission - see set_verb_info_wizard_succeeds"
- `set_verb_info_no_write_permission` (line 447-450) - "requires wizard permission - not just write"
- `set_verb_info_with_write_permission_still_fails` (line 452-455) - "requires wizard permission - not just write"

### Behaviors Tested But Not in Spec

The following behaviors work correctly in Barn but are not explicitly documented in the spec:

1. **Verb indexing**: Both `verb_info(obj, "name")` and `verb_info(obj, 1)` work (1-based integer index)
2. **Verb aliases**:
   - Multiple space-separated names in add_verb: `"foo bar baz"`
   - All aliases can be used to find the verb
   - verb_info() returns full alias string
   - verbs() returns only the primary (first) name
3. **Prep spec expansion**:
   - Input: `"on"`
   - Output from verb_args(): `"on top of/on/onto/upon"`
   - This normalization behavior is implemented but not documented
4. **set_verb_code() compile errors**:
   - Returns list of error strings (not error objects)
   - Error format: `"parse error: expected ';' after expression statement"`
5. **disassemble() pseudo-opcodes**:
   - Returns simplified opcode names like "PUSH", "ADD", "RETURN"
   - Not actual bytecode, but AST walk producing pseudo-assembly

## Behaviors Verified Correct

The following behaviors match between Barn and conformance test expectations:

### Query Functions
- `verbs(obj)` - Returns list of primary verb names
- `verb_info(obj, name_or_index)` - Returns `{owner, perms, names}`
- `verb_args(obj, name_or_index)` - Returns `{dobj, prep, iobj}` with prep expansion
- `verb_code(obj, name)` - Returns list of source lines

### Management Functions
- `add_verb(obj, info, args)` - Returns 1-based verb index
- `delete_verb(obj, name)` - Returns 0 on success
- `set_verb_info(obj, name, info)` - Returns 0 on success (wizard only)
- `set_verb_args(obj, name, args)` - Returns 0 on success
- `set_verb_code(obj, name, code)` - Returns `{}` on success or list of errors

### Error Handling
- E_INVARG for recycled objects
- E_VERBNF for non-existent verbs
- E_INVARG for invalid verb specs (dobj, iobj must be "this"/"none"/"any")
- E_INVARG for invalid prep specs
- E_INVARG for invalid permission strings (only rwxd allowed)
- E_TYPE for wrong argument types

### Permission Model (Partially Implemented)
- `add_verb()` - Checks write permission on object and caller must be owner (or wizard)
- `verb_code()` - Checks read permission (or wizard)
- Wizard always bypasses permission checks
- Other functions have TODOs for permission checks

### Prep Spec List
Barn's prepList (lines 13-29) matches ToastStunt's prep_list exactly:
```
"with/using", "at/to", "in front of", "in/inside/into", "on top of/on/onto/upon",
"out of/from inside/from", "over", "through", "under/underneath/beneath", "behind",
"beside", "for/about", "is", "as", "off/off of"
```

## Recommendations

### Critical Fixes Needed in Barn

1. **Database loading issue** - Investigate why only 3 of 27 verbs on #0 are loaded
2. **Wizard object ID** - Fix wizard ID mismatch (#2 vs #3)
3. **Implement permission checks** - Complete the 4 TODOs for delete_verb, set_verb_info, set_verb_args, set_verb_code

### Spec Improvements Needed

1. **Document verb indexing** - Add section explaining verbs can be accessed by name or 1-based integer index
2. **Document verb aliases** - Explain space-separated names and how they're handled
3. **Document prep spec expansion** - Explain that verb_args() returns the full slash-separated string
4. **Document set_verb_code() error format** - Specify that compile errors return list of strings
5. **Document disassemble() format** - Clarify this returns pseudo-opcodes, not real bytecode
6. **Clarify prep spec validation** - Document that only individual prep names are valid in add_verb/set_verb_args, not full "with/using" strings

### Test Suite Improvements

The conformance test suite is comprehensive but could benefit from:
1. Multi-user test scenarios to cover non-owner permission checks
2. Tests for verb indexing (integer argument to verb_info/verb_args)
3. Tests for verb alias behavior
4. Tests for prep spec expansion behavior
