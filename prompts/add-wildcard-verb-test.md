# Task: Add Verb Wildcard Matching Test to cow_py Conformance Tests

## Objective

Add a YAML test to cow_py's conformance suite that verifies verb wildcard prefix matching works.

## The Bug We Fixed

MOO verbs can have wildcards like `test*verb` meaning:
- `test`, `testv`, `testve`, `testver`, `testverb` all match

Barn was doing exact matching. We fixed it. Now we need a regression test.

## Test Location

Add to: `~/code/cow_py/tests/conformance/objects/verbs.yaml`

Or create new file: `~/code/cow_py/tests/conformance/objects/verb_matching.yaml`

## Test Content

```yaml
name: verb_matching
description: Tests for verb name matching including wildcards

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

## Output

1. Add the test file
2. Run tests against Barn to verify they pass:
   ```bash
   cd ~/code/cow_py
   uv run pytest tests/conformance/objects/verb_matching.yaml -v --transport socket --moo-port 9300
   ```
3. Write results to `./reports/add-wildcard-verb-test.md`
