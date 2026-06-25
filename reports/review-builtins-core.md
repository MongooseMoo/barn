# Builtins Core Review — objects/verbs/properties

**Analyst:** Sonnet 4.6  
**Date:** 2026-06-25  
**Scope:** builtins/{objects.go, objects_hierarchy.go, objects_misc.go, objects_movement.go, objects_movement_test.go, objects_players.go, properties.go, verbs.go, registry.go, host.go, protected.go, signatures.go, signatures_test.go, types.go, function_signatures_generated.go}

---

## Baseline

- `go test ./builtins/... -count=1` → PASS before review
- `go vet ./builtins/` → clean
- Red tests added: `builtins/review_test.go` (4 tests, all fail)
- Pre-existing tests: still pass after test file added

---

## Architecture Summary

The recent "Bundle builtin host capabilities into a Host struct" commit is directionally correct. `host.go` defines a `Host` struct with nil-safe capability fields; `hostOf(ctx)` retrieves it via a type assertion; each builtin gets graceful degradation when a capability is absent. This is the right pattern.

The `Registry` is a flat map of name→WrappedFunc. Signature validation is injected at registration time so all call paths (by ID, by name, via `call_function`) get consistent type checking.

The permission-check layer in builtins is incomplete. Several critical operations have explicit TODO comments where checks are missing, and several others use the wrong identity object (`ctx.Player` rather than `ctx.Programmer`).

---

## CONFIRMED BUGS (red tests in builtins/review_test.go)

### CONFIRMED-1 [HIGH] `delete_verb` on inherited verb silently succeeds

**Test:** `TestReview_DeleteVerbOnInheritedVerbReturnsEVERBNF`

```
--- FAIL: TestReview_DeleteVerbOnInheritedVerbReturnsEVERBNF (0.00s)
    review_test.go:78: delete_verb on inherited verb returned success (E_NONE); want E_VERBNF
```

`store.DeleteVerb` calls `findVerbLocked` (BFS over ancestors) to obtain the verb pointer, then scans the *child* object's `obj.verbs` map comparing pointer identity. The ancestor's verb pointer is never in the child's map, so the loop body never executes. No verb is deleted; `E_NONE` is returned. The builtin layer propagates that success. Toast returns `E_VERBNF`.

Root: `db/store/store_verbs.go:344` — `DeleteVerb` should verify the found verb is defined on `objID`, not just reachable from it.

---

### CONFIRMED-2 [CRITICAL] `set_verb_info`, `set_verb_args`, `set_verb_code` mutate ancestor verbs in-place

**Test:** `TestReview_SetVerbInfoMutatesAncestorVerb`

```
--- FAIL: TestReview_SetVerbInfoMutatesAncestorVerb (0.00s)
    review_test.go:138: set_verb_info(child, inherited, ...) mutated parent verb owner: was #0, now #99 (BUG: ancestor corrupted)
```

All three store methods (`SetVerbInfo`, `SetVerbArgs`, `SetVerbCode`) call `findVerbLocked` (BFS), receive the ancestor's `*Verb` pointer, then write directly to its fields (`verb.owner = owner`, `verb.argSpec = argSpec`, `verb.code = lines`). This corrupts the ancestor's verb for every object that inherits it. `SetVerbInfo` additionally inserts the ancestor verb pointer into the *child's* verbs map when the name changes, leaving the store in an internally inconsistent state.

The builtin-layer functions (`set_verb_info`, `set_verb_args`, `set_verb_code`) call these store methods with the child's `objID` and the verb's string name, making no attempt to verify the verb is locally defined.

Root: `db/store/store_verbs.go:385,415,434` — all three must restrict to locally-defined verbs (same fix needed as CONFIRMED-1).

---

### CONFIRMED-3 [MEDIUM] `verb_code` denies the verb owner if the `'r'` bit is absent

**Test:** `TestReview_VerbCodeAllowsOwnerWithoutReadBit`

```
--- FAIL: TestReview_VerbCodeAllowsOwnerWithoutReadBit (0.00s)
    review_test.go:169: verb_code denied owner without 'r' bit — want success, got E_PERM (BUG)
```

`builtins/verbs.go:315`:
```go
if !verb.Perms.Has(dbstore.VerbRead) && !ctx.IsWizard {
    return types.Err(types.E_PERM)
}
```

The owner check is absent. Toast allows the verb's owner to read their own verb code regardless of the `'r'` permission bit. Fix: `!ctx.IsWizard && ctx.Programmer != verb.Owner && !verb.Perms.Has(VerbRead)`.

---

### CONFIRMED-4 [MEDIUM] `add_verb` uses `ctx.Player` instead of `ctx.Programmer` for ownership check

**Test:** `TestReview_AddVerbUsesProgNotPlayerForPerm`

```
--- FAIL: TestReview_AddVerbUsesProgNotPlayerForPerm (0.00s)
    review_test.go:213: add_verb denied programmer (#0) who owns object; ctx.Player=#5 wrongly used (BUG): got E_PERM
```

`builtins/verbs.go:450,455`:
```go
if !hasWrite && objectOwner != ctx.Player {   // BUG
    return types.Err(types.E_PERM)
}
if ownerID != ctx.Player {                     // BUG
    return types.Err(types.E_PERM)
}
```

`ctx.Player` is the connected-player object. `ctx.Programmer` is the effective task-permissions identity (lowered by `set_task_perms`). Permission decisions must use `ctx.Programmer`. When they diverge the check either wrongly denies or wrongly grants. Fix: replace `ctx.Player` with `ctx.Programmer` in both comparisons.

---

## SUSPECTED BUGS (no red test; architecturally clear)

### SUSPECTED-1 [HIGH] `recycle()` skips permission check

`builtins/objects.go:335`: `// TODO: Check permissions (Layer 8.5)`. Any non-wizard programmer can recycle any object. Toast requires wizard or owner.

### SUSPECTED-2 [HIGH] `delete_property()` skips permission check

`builtins/properties.go:359`: `// TODO: Check permissions (owner or wizard)`. Any programmer can delete any property from any object.

### SUSPECTED-3 [HIGH] `delete_verb()` skips permission check at the builtin layer

`builtins/verbs.go:502`: `// TODO: Check permissions (must be owner or wizard)`. The store may return `E_NONE` or `E_VERBNF` but the builtin never checks whether the caller owns the verb before forwarding.

### SUSPECTED-4 [HIGH] `renumber()` skips wizard permission check

`builtins/objects_misc.go:21`: commented-out wizard check. Any programmer can renumber any object.

### SUSPECTED-5 [MEDIUM] `verb_info()`, `verb_args()`, `disassemble()` use `ctx.Player` not `ctx.Programmer`

`builtins/verbs.go:115,209,868`: owner comparison uses `ctx.Player`. Same identity confusion as CONFIRMED-4.

### SUSPECTED-6 [MEDIUM] `chparents()` skips permission and fertile-flag check

`builtins/objects_hierarchy.go:337`: `// TODO: Check permissions and fertile flags`. Non-wizards can reparent objects to non-fertile parents they don't own.

### SUSPECTED-7 [LOW] `occupants(list)` with one argument always filters by player flag

`builtins/objects_movement.go:132`:
```go
checkPlayerFlag := len(args) == 1 || (len(args) > 2 && args[2].Truthy())
```
With one argument, every object in the list is filtered to only player-flagged objects. Per spec, `occupants(list)` with no additional args should return all valid objects from the list. (Toast oracle unavailable to confirm; flagged as SUSPECTED.)

---

## ARCHITECTURAL FINDINGS

### ARCH-1 [MEDIUM] Protected-builtin state is a package global despite the Host refactor

`builtins/protected.go`: `protectedBuiltins` is a package-level `sync.RWMutex`-guarded map. Multiple `Registry` instances (e.g., in tests) share a single protection state. The `Host` struct was explicitly designed to avoid instance-shared state; protected-builtin flags should move to `Registry.host` or `Registry` itself for consistency.

### ARCH-2 [MEDIUM] `Registry.Register` wraps every known function in a signature-validator closure; direct function calls bypass it

`builtins/registry.go:340–354`: the wrapped closure is stored in `r.funcs` and `r.byID`. Unit tests calling e.g. `builtinCreate(ctx, args)` directly skip the validator. Production calls through the Registry validate. This two-path behavior means tests do not reproduce the exact conditions a running server executes.

### ARCH-3 [LOW] `validateFunctionArgs` is dead code

`builtins/signatures.go:86`: defined, exported-by-convention, but not called from any production path. `Register` calls `validateKnownFunctionArgs` directly.

### ARCH-4 [LOW] `normalizeVerbSourceLines` heuristic is brittle and two-pass compile silently fixes malformed code

`builtins/verbs.go:753`: the keyword list for semicolon injection is incomplete (`break`, `continue`, standalone `return` without a value are missing). Two-pass compile (`raw → normalized`) may mask programming errors.

### ARCH-5 [LOW] `disassemble()` is a non-functional stub

Only handles `ExprStmt` and `ReturnStmt`. All other AST node types emit `"STMT"`. The output is not useful for debugging any non-trivial verb.

### ARCH-6 [LOW] `builtinQueueInfo`, `builtinFinishedTasks`, `builtinThreads` bypass Host by calling `task.GetManager()` directly

`builtins/signatures.go:241,254,265`: three functions use a global task manager singleton. This is inconsistent with the Host pattern and prevents test isolation.

### ARCH-7 [LOW] `builtinDbDiskSize` hardcodes three filenames

`builtins/signatures.go:385–391`: only probes `"Test.db"`, `"mongoose.db"`, `"toast.db"`. Returns `0` in any production deployment with a different database path.

---

## Summary Table

| ID | Severity | Status | Description |
|----|----------|--------|-------------|
| CONFIRMED-1 | HIGH | Red test | `delete_verb` on inherited verb returns `E_NONE`, should be `E_VERBNF` |
| CONFIRMED-2 | CRITICAL | Red test | `set_verb_info/args/code` mutate ancestor verbs in-place, corrupting all inheritors |
| CONFIRMED-3 | MEDIUM | Red test | `verb_code` denies verb owner without `'r'` bit |
| CONFIRMED-4 | MEDIUM | Red test | `add_verb` uses `ctx.Player` not `ctx.Programmer` for ownership check |
| SUSPECTED-1 | HIGH | TODO comment | `recycle()` missing permission check |
| SUSPECTED-2 | HIGH | TODO comment | `delete_property()` missing permission check |
| SUSPECTED-3 | HIGH | TODO comment | `delete_verb()` missing builtin-layer permission check |
| SUSPECTED-4 | HIGH | TODO comment | `renumber()` missing wizard check |
| SUSPECTED-5 | MEDIUM | Code | `verb_info/args/disassemble` use `ctx.Player` not `ctx.Programmer` |
| SUSPECTED-6 | MEDIUM | TODO comment | `chparents()` missing permission/fertile check |
| SUSPECTED-7 | LOW | Suspected | `occupants(list)` 1-arg form filters by player flag incorrectly |
| ARCH-1 | MEDIUM | Design | Protected-builtin map is package-global despite Host refactor |
| ARCH-2 | MEDIUM | Design | Signature-validator bypass via direct function calls in tests |
| ARCH-3 | LOW | Dead code | `validateFunctionArgs` unused |
| ARCH-4 | LOW | Design | `normalizeVerbSourceLines` incomplete + masks errors |
| ARCH-5 | LOW | Stub | `disassemble()` emits `"STMT"` for all control flow |
| ARCH-6 | LOW | Design | Three builtins use `task.GetManager()` global, not Host |
| ARCH-7 | LOW | Design | `db_disk_size` hardcodes filenames |
