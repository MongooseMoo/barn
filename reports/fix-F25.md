# Fix F25 — verb_code denies the verb owner without the 'r' bit

## Finding
`builtins/verbs.go` `builtinVerbCode` checked only
`!verb.Perms.Has(VerbRead) && !ctx.IsWizard`, omitting the OWNER bypass. A verb's
owner could not read their own verb's code unless the `r` bit was set.

## Toast authority (read rule)
- `src/verbs.cc:493-494` `bf_verb_code`: `else if (!db_verb_allows(h, progr, VF_READ)) return make_error_pack(E_PERM);`
- `src/db_verbs.cc:880-885` `db_verb_allows`:
  ```c
  return ((db_verb_flags(h) & flag)
          || progr == db_verb_owner(h)
          || is_wizard(progr));
  ```
So read is permitted when `(perms & VF_READ) || progr == verb.owner || is_wizard(progr)`;
denial returns `E_PERM`. The "owner" is the VERB's owner, compared against the task
programmer (`progr` / `ctx.Programmer`).

## The change
`builtins/verbs.go` (~line 315), read check now:
```go
if !verb.Perms.Has(dbstore.VerbRead) && !ctx.IsWizard && ctx.Programmer != verb.Owner {
    return types.Err(types.E_PERM)
}
```
Uses the verb's owner and the task programmer, matching Toast and the existing sibling
checks in this file (verb_args line ~209, verb_info line ~271, F24 convention). Denied
error code unchanged (`E_PERM`).

## Test contract
`builtins/review_test.go` `TestReview_VerbCodeAllowsOwnerWithoutReadBit` now asserts all
three:
1. Verb owner (`ctx.Programmer == verb.Owner`, non-wizard) reads code with NO `r` bit -> success.
2. Non-owner non-wizard (`ctx.Programmer = 5`) with no `r` bit -> `E_PERM`.
3. A verb with the `r` bit set is readable by anyone (still non-owner) -> success.

## Verification
- RED (old check, owner-bypass removed): test FAILS
  `review_test.go:169: verb_code denied owner without 'r' bit — want success, got E_PERM (BUG)`
- GREEN (fix applied): `--- PASS: TestReview_VerbCodeAllowsOwnerWithoutReadBit`
- `go test ./builtins/ -run 'VerbCode|verb_code|Perm' -v` -> all PASS
- `go vet ./builtins/` -> clean
- `go test ./builtins/...` -> only 4 pre-existing intentionally-red findings remain, all
  unrelated to F25: `IsMemberStrCaseSensitiveBug`, `PcreMatchEmptySubject`,
  `FileReadlinesBinaryMode` (M1), `QueuedTasksSortOrder` (H4). No NEW failures.

## set_verb_code note
Toast `bf_set_verb_code` (`verbs.cc:532`) uses
`!is_programmer(progr) || !db_verb_allows(h, progr, VF_WRITE)`, and `db_verb_allows`
applies the owner/wizard bypass for the write flag. Barn's `builtinSetVerbCode`
(verbs.go:687) checks `if !ctx.IsWizard && ctx.Programmer != verb.Owner` — i.e. it ALREADY
honors the owner bypass, so the F25 bug (owner wrongly denied) does NOT exist in
set_verb_code. Therefore no change is made here per the prompt's instructions.

(Separately, Barn's set_verb_code does not grant write to a non-owner programmer holding the
`w` bit, which Toast would allow. That is a different, opposite-direction divergence, not
F25, and has no red test — out of scope for this fix.)

## Commit
`c98f3ec168c3aa9b3ba641b9396370028007d929` on `review/branch-stocktake-2026-06-25`.
