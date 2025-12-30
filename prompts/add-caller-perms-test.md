# Task: Add caller_perms() Test to cow_py Conformance Tests

## Objective

Add a YAML test that verifies `caller_perms()` returns the CALLER's programmer, not the current frame's programmer.

## The Bug We Fixed

`caller_perms()` was returning the current frame's programmer instead of the calling frame's programmer.

Call stack:
- Frame 0: verb owned by #2 (programmer = #2)
- Frame 1: verb owned by #3 (programmer = #3)
- Inside Frame 1, `caller_perms()` should return #2

## Test Location

Add to: `~/code/cow_py/tests/conformance/builtins/` - either existing file or new `caller_perms.yaml`

## Test Approach

Create two verbs on test objects:
1. Outer verb (caller) - owned by player, calls inner verb
2. Inner verb (callee) - returns caller_perms()

The test verifies that caller_perms() inside the inner verb returns the outer verb's owner.

## Test Content

```yaml
name: caller_perms
description: Tests for caller_perms() builtin

tests:
  - name: caller_perms_returns_caller_programmer
    description: caller_perms() returns the programmer of the calling verb, not current verb
    permission: wizard
    statement: |
      outer = create($nothing);
      inner = create($nothing);
      add_verb(outer, {player, "rxd", "call_inner"}, {"this", "none", "this"});
      add_verb(inner, {inner, "rxd", "get_caller_perms"}, {"this", "none", "this"});
      set_verb_code(outer, "call_inner", {"return inner:get_caller_perms();"});
      set_verb_code(inner, "get_caller_perms", {"return caller_perms();"});
      result = outer:call_inner();
      expected = player;
      recycle(outer);
      recycle(inner);
      return result == expected;
    expect:
      value: 1

  - name: caller_perms_nothing_at_top_level
    description: caller_perms() returns NOTHING when called at top level (no caller)
    permission: wizard
    code: "caller_perms()"
    expect:
      value: -1
```

## Output

1. Add the test file to cow_py conformance tests
2. Write results to `./reports/add-caller-perms-test.md`
