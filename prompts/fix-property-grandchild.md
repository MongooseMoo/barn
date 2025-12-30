# Task: Fix property::create_grandchild Test Failure

## Context
Barn is a Go MOO server. The conformance test `property::create_grandchild` is failing.

## The Test
```yaml
- name: create_grandchild
  code: 'eval("$temp0 = create($temp); return parent($temp0) == $temp;")'
  expect:
    value: [1, 1]
```

## What's Happening
Manual test shows:
```
; return eval("$temp0 = create($temp); return parent($temp0) == $temp;");
{1, {1, 1}}
```

Expected: `{1, 1}` (flat list)
Got: `{1, {1, 1}}` (nested - extra layer)

The test framework wraps code with `return <code>;` so it becomes:
```
return eval("$temp0 = create($temp); return parent($temp0) == $temp;");
```

But eval() returns `{success, result}`, so it's getting double-wrapped somehow.

## Investigation Needed
Check how the test framework in cow_py sends code and parses results. The issue might be:
1. How barn's eval verb wraps results
2. How the test framework interprets eval() results
3. Double-wrapping somewhere

## Files to Check
- `C:/Users/Q/code/cow_py/tests/conformance/schema.py` - get_code_to_execute()
- `C:/Users/Q/code/cow_py/tests/conformance/runner.py` - how results are parsed
- `C:/Users/Q/code/barn/Test.db` or verb definitions - how #2:eval works

## Test Command
```bash
cd ~/code/cow_py && uv run pytest tests/conformance/ --transport socket --moo-port 9230 -x -k "create_grandchild" -v
```

## Output
Write findings to `./reports/fix-property-grandchild.md`

## CRITICAL: Do NOT modify tests
The tests are correct (they pass against ToastStunt). Only fix barn implementation or understand what's different.

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again
2. Retry the Edit
3. Try different path formats
4. NEVER use cat, sed, echo
