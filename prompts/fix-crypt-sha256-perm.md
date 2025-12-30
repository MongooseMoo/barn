# Task: Fix crypt() SHA256 Rounds Permission Error

## Context
Barn is a Go MOO server. The test `crypt_sha256_rounds_wizard` fails with E_PERM.

## The Bug
Test expects success but gets E_PERM when calling crypt with SHA256 rounds as wizard.

## The Test
```yaml
- name: crypt_sha256_rounds_wizard
  permission: wizard
  code: 'index(crypt("password", "$5$rounds=10000$abc"), "$5$rounds=10000$")'
  expect:
    value: 1
```

The `$5$rounds=10000$` prefix indicates SHA256 with custom rounds. This requires wizard permission in MOO.

## Investigation Needed
1. The fix is likely similar to the create() permission fix
2. Check `builtins/crypto.go` for the crypt permission checks
3. Similar to the create() fix - probably need to check player wizard flag, not just ctx.IsWizard

## Files to Check
- `builtins/crypto.go` - crypt implementation and permission checks
- `~/code/cow_py/tests/conformance/builtins/algorithms.yaml` - find the test

## Test Command
```bash
cd ~/code/cow_py && uv run pytest tests/conformance/ --transport socket --moo-port 9280 -x -k "crypt_sha256_rounds_wizard" -v
```

## Output
Write findings to `./reports/fix-crypt-sha256-perm.md`

## CRITICAL: Do NOT modify tests
The tests are correct. Only fix barn implementation.
