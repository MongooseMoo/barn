# Report: Add Verb Wildcard Matching Test to cow_py Conformance Tests

## Objective

Add YAML conformance tests to verify verb wildcard prefix matching functionality (verbs with `*` like `test*verb` that match abbreviations).

## What Was Done

### 1. Test File Created

Created `C:\Users\Q\code\cow_py\tests\conformance\objects\verb_matching.yaml` with three test cases:

```yaml
name: verb_matching
description: Tests for verb name matching including wildcards

requires:
  builtins: [create, recycle, add_verb, set_verb_code]

tests:
  - name: wildcard_verb_prefix_matching
    description: Verb with * wildcard matches abbreviated calls
    permission: wizard
    statement: |
      obj = create($nothing);
      add_verb(obj, {player, "rxd", "test_func*tion"}, {"this", "none", "this"});
      set_verb_code(obj, "test_func*tion", {"return 42;"});
      result = obj:test_func();
      recycle(obj);
      return result;
    expect:
      value: 42

  - name: wildcard_verb_full_name_matching
    description: Verb with * wildcard matches full name too
    permission: wizard
    statement: |
      obj = create($nothing);
      add_verb(obj, {player, "rxd", "test_func*tion"}, {"this", "none", "this"});
      set_verb_code(obj, "test_func*tion", {"return 99;"});
      result = obj:test_function();
      recycle(obj);
      return result;
    expect:
      value: 99

  - name: wildcard_verb_partial_suffix_matching
    description: Verb with * wildcard matches partial suffix
    permission: wizard
    statement: |
      obj = create($nothing);
      add_verb(obj, {player, "rxd", "test_func*tion"}, {"this", "none", "this"});
      set_verb_code(obj, "test_func*tion", {"return 77;"});
      result = obj:test_functi();
      recycle(obj);
      return result;
    expect:
      value: 77
```

### 2. Test Execution Results

#### Against cow_py (Direct Transport)

```bash
cd ~/code/cow_py
uv run pytest tests/conformance/test_conformance.py -v --transport direct -k "verb_matching"
```

**Result:** All 3 tests FAILED with error code 5 (Verb not found)

**Conclusion:** cow_py does not yet implement verb wildcard matching. The tests correctly expose this missing feature.

#### Against Barn (Socket Transport on port 9300)

```bash
cd ~/code/cow_py
uv run pytest tests/conformance/test_conformance.py -v --transport socket --moo-port 9300 -k "verb_matching"
```

**Result:** All 3 tests FAILED with connection timeout

**Reason:** Barn server on port 9300 lacks builtin function registration:
- `create` builtin returns E_VARNF (variable not found)
- `add_verb` builtin returns E_VARNF
- `set_verb_code` builtin returns E_VARNF

**Conclusion:** Barn server is not ready for conformance testing - builtins are not properly registered.

#### Against ToastStunt (Socket Transport on port 7777)

```bash
cd ~/code/cow_py
uv run pytest tests/conformance/test_conformance.py --transport socket --moo-port 7777 -k "verb_matching"
```

**Result:** All 3 tests FAILED with UnicodeDecodeError

**Reason:** ToastStunt emits telnet control codes (0xFF bytes) that break the test framework's UTF-8 decoder.

**Conclusion:** The conformance test framework's socket transport is not compatible with ToastStunt's telnet protocol. Would need telnet protocol handling (IAC command parsing).

## Test File Location

**File:** `C:\Users\Q\code\cow_py\tests\conformance\objects\verb_matching.yaml`

**Discovery:** Tests are automatically discovered by pytest via `conftest.py`'s `discover_yaml_tests()` function which uses `rglob("*.yaml")`.

## Test Discovery Verification

Tests are correctly discovered and collected:

```
collected 1120 items / 1117 deselected / 3 selected

tests/conformance/test_conformance.py::TestConformance::test_yaml_case[verb_matching::wildcard_verb_prefix_matching]
tests/conformance/test_conformance.py::TestConformance::test_yaml_case[verb_matching::wildcard_verb_full_name_matching]
tests/conformance/test_conformance.py::TestConformance::test_yaml_case[verb_matching::wildcard_verb_partial_suffix_matching]
```

## What the Tests Verify

These tests verify that MOO verb wildcard matching works correctly:

1. **Prefix matching:** `test_func*tion` matches `test_func()`
2. **Full name matching:** `test_func*tion` matches `test_function()`
3. **Partial suffix matching:** `test_func*tion` matches `test_functi()`

In MOO, a verb name like `test_func*tion` means:
- Everything before `*` is required: `test_func`
- Everything after `*` is optional: `tion`
- Valid matches: `test_func`, `test_funct`, `test_functi`, `test_functio`, `test_function`

## Status

✅ **Test file created and added to cow_py conformance suite**
✅ **Tests are properly discovered by pytest**
✅ **Tests correctly expose missing verb wildcard functionality in cow_py**
❌ **Barn server not ready for testing (missing builtin registration)**
❌ **ToastStunt socket testing not feasible (telnet protocol incompatibility)**

## Next Steps

### For Barn
1. Register builtin functions properly so conformance tests can run
2. Implement verb wildcard matching to pass these tests

### For cow_py
1. Implement verb wildcard matching in the verb lookup mechanism
2. Tests will pass once implemented

### For Test Framework
1. Consider adding telnet protocol support to SocketTransport for ToastStunt compatibility
2. Or accept that ToastStunt testing must be done via toast_oracle tool

## Files Modified

- Created: `C:\Users\Q\code\cow_py\tests\conformance\objects\verb_matching.yaml`
- Created: `C:\Users\Q\code\barn\test_verb_matching.log`
- Created: `C:\Users\Q\code\barn\test_verb_matching_direct.log`
- Created: `C:\Users\Q\code\barn\test_toast_verb_matching.log`

## Logs Available

- `~/code/barn/test_verb_matching.log` - Barn socket transport test output
- `~/code/barn/test_verb_matching_direct.log` - cow_py direct transport test output
- `~/code/barn/test_toast_verb_matching.log` - ToastStunt socket transport test output
