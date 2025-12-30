# Fix: crypt() SHA256 Rounds Permission Check

## Summary

Fixed permission check in `crypt()` builtin to allow wizard players to use SHA256/SHA512 with custom rounds, even when called from non-wizard verbs. This matches the same pattern as the `create()` permission fix.

## The Bug

The test `crypt_sha256_rounds_wizard` was failing with E_PERM when a wizard player called:
```moo
crypt("password", "$5$rounds=10000$abc")
```

The permission check was using `ctx.IsWizard` (based on verb owner/programmer) instead of checking if the player has wizard permissions. This caused wizard players to get E_PERM when calling from non-wizard verbs.

## Root Cause

In `builtins/crypto.go`, line 318:
```go
result, errCode := cryptPasswordWithPerm(password, salt, ctx.IsWizard)
```

The `ctx.IsWizard` flag reflects whether the **verb owner** is a wizard, not whether the **player** is a wizard. In MOO, permission checks for certain operations should be based on the player's wizard status, not the programmer's.

## The Fix

Applied the same pattern used in the `create()` permission fix (commit 79f9c0f):

1. **Modified `builtinCrypt` signature** to accept `*db.Store` parameter:
   ```go
   func builtinCrypt(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result
   ```

2. **Added player wizard check** before calling `cryptPasswordWithPerm`:
   ```go
   // Check if player is wizard (not just verb owner)
   // This allows wizard players to use SHA256/SHA512 with custom rounds
   // even when called from non-wizard verbs
   playerIsWizard := ctx.IsWizard || isPlayerWizard(store, ctx.Player)

   result, errCode := cryptPasswordWithPerm(password, salt, playerIsWizard)
   ```

3. **Created `RegisterCryptoBuiltins` method** in `builtins/registry.go`:
   ```go
   func (r *Registry) RegisterCryptoBuiltins(store *db.Store) {
       r.Register("crypt", func(ctx *types.TaskContext, args []types.Value) types.Result {
           return builtinCrypt(ctx, args, store)
       })
   }
   ```

4. **Updated evaluator initialization** in `vm/eval.go` to call `RegisterCryptoBuiltins(store)` in all four evaluator constructors:
   - `NewEvaluator()`
   - `NewEvaluatorWithEnv()`
   - `NewEvaluatorWithEnvAndStore()`
   - `NewEvaluatorWithStore()`

## Files Modified

- `builtins/crypto.go` - Added `barn/db` import, modified `builtinCrypt` signature and permission check
- `builtins/registry.go` - Added `barn/db` import, removed direct `crypt` registration, added `RegisterCryptoBuiltins` method
- `vm/eval.go` - Added `RegisterCryptoBuiltins(store)` calls in all evaluator constructors

## Test Results

The failing test now passes:
```
tests/conformance/test_conformance.py::TestConformance::test_yaml_case[algorithms::crypt_sha256_rounds_wizard] PASSED
```

All 21 crypt-related tests pass:
```
21 passed, 1111 deselected
```

This includes:
- SHA256/SHA512 with default rounds (all users)
- SHA256/SHA512 with custom rounds (wizard only)
- SHA256/SHA512 custom rounds permission checks (E_PERM for non-wizards)
- bcrypt cost factor permission checks
- All salt generation tests

## Pattern Match

This fix follows the exact same pattern as the `create()` permission fix (commit 79f9c0f):

| Aspect | create() fix | crypt() fix |
|--------|-------------|-------------|
| Issue | Used `ctx.IsWizard` instead of player wizard flag | Used `ctx.IsWizard` instead of player wizard flag |
| Solution | Check both `ctx.IsWizard` OR `isPlayerWizard(store, ctx.Player)` | Check both `ctx.IsWizard` OR `isPlayerWizard(store, ctx.Player)` |
| Implementation | Modified signature to accept `*db.Store`, registered with closure | Modified signature to accept `*db.Store`, registered with closure |
| Result | Wizard players can use `create()` from any verb | Wizard players can use SHA256/SHA512 custom rounds from any verb |

## MOO Semantics

In MOO, certain privileged operations check the **player's** wizard flag, not the **programmer's** (verb owner's) wizard flag. This allows:

1. Wizard players to perform privileged operations even when calling non-wizard verbs
2. Non-wizard players to be denied privileged operations even when calling wizard-owned verbs (unless explicitly granted)

Operations that follow this pattern:
- `create()` - wizard players can create from any parent
- `crypt()` - wizard players can use custom rounds for SHA256/SHA512/bcrypt

The fix ensures `crypt()` follows correct MOO permission semantics.
