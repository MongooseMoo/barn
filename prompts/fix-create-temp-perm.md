# Task: Fix create($temp) Permission Error

## Context
Barn is a Go MOO server. The test `property::create_grandchild` fails with E_PERM.

## The Test Sequence
```yaml
- name: create_child_of_root
  code: 'eval("$temp = create(#1); return parent($temp);")'
  expect: [1, "#1"]

- name: create_grandchild
  code: 'eval("$temp0 = create($temp); return parent($temp0) == $temp;")'
  expect: [1, 1]
```

## What's Happening
- `create_child_of_root` passes - creates `$temp` from #1
- `create_grandchild` fails with E_PERM when trying to `create($temp)`

## The Problem
Each `connect wizard` creates a NEW wizard player object. So:
- Test 1 runs as wizard #X, creates $temp owned by #X
- Test 2 runs as wizard #Y (different object!), tries to create from $temp
- #Y doesn't own $temp and $temp might not have the fertile flag

## Investigation Needed
Check `builtins/objects.go` create() permission logic:
- Does it properly check if the player is a wizard?
- Does it check fertile flag correctly?
- The previous fix checked `isPlayerWizard()` - is that being called correctly for all code paths?

## Test Command
```bash
cd /c/Users/Q/code/barn
./barn_test.exe -db Test.db -port 9240 &
# Run exact failing sequence:
printf 'connect wizard\n; return eval("$temp = create(#1); return $temp;");\n' | nc -w 3 localhost 9240
printf 'connect wizard\n; return eval("$temp0 = create($temp); return $temp0;");\n' | nc -w 3 localhost 9240
```

The second command should succeed (wizard can create from any object).

## Files to Check
- `builtins/objects.go` - create() permission checks around line 163-180

## Output
Write findings to `./reports/fix-create-temp-perm.md`

## CRITICAL: Do NOT modify tests
The tests are correct. Only fix barn implementation.
