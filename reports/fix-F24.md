# Fix F24 — add_verb (and disassemble) checked ctx.Player instead of ctx.Programmer

## Finding
MOO permission checks key on the TASK's effective programmer (`progr`), not the
connection player. Under lowered task perms (`set_task_perms`), Barn's `add_verb`
compared the object/verbinfo owner against `ctx.Player`, wrongly denying a
programmer who owns the object with `E_PERM`. The sibling `disassemble` shared the
bug. `verb_info`/`verb_args` were already correct.

## Authority — ToastStunt source (identity each check uses)
All gate on `progr` (the task programmer), never the player:

| Builtin | file:line | Check |
|---|---|---|
| `bf_add_verb` | `src/verbs.cc:198-199` | `!db_object_allows(obj, progr, FLAG_WRITE) \|\| (progr != owner && !is_wizard(progr))` → E_PERM |
| `bf_verb_info` | `src/verbs.cc:295` | `!db_verb_allows(h, progr, VF_READ)` → E_PERM |
| `bf_verb_args` | `src/verbs.cc:405` | `!db_verb_allows(h, progr, VF_READ)` → E_PERM |
| `bf_set_verb_info` | `src/verbs.cc:353-354` | `!db_verb_allows(h, progr, VF_WRITE) \|\| (!is_wizard(progr) && db_verb_owner(h) != new_owner)` |
| `bf_disassemble` | `src/disassemble.cc:483` | `!db_verb_allows(h, progr, VF_READ)` → E_PERM |

Helpers (both keyed on `progr`):
- `db_object_allows` `src/db_objects.cc:1294-1298`: `is_wizard(progr) || progr == owner || has_flag`.
- `db_verb_allows` `src/db_verbs.cc:880-885`: `flag set || progr == verb_owner || is_wizard(progr)`.

Barn convention cross-check: the already-correct builtins in `builtins/verbs.go`
(`verb_info` L209, `verb_args` L271, `set_verb_info` L548, `set_verb_args` L613,
`set_verb_code` L680) all read `ctx.Programmer`/`ctx.IsWizard`. The fix mirrors them.

## Changes (`builtins/verbs.go`)
- `builtinAddVerb`: `objectOwner != ctx.Player` → `!= ctx.Programmer`; `ownerID != ctx.Player`
  → `!= ctx.Programmer` (the two FLAG_WRITE / verbinfo-owner checks). Matches
  `db_object_allows(obj, progr, FLAG_WRITE) || (progr != owner && !wizard)`.
- `builtinDisassemble`: `isOwner := verb.Owner == ctx.Player` → `== ctx.Programmer`.
  Matches `db_verb_allows(h, progr, VF_READ)`.

Success path and error codes (E_PERM/E_INVARG/E_TYPE) unchanged.

## Siblings fixed vs left
- Fixed: `add_verb`, `disassemble` (source proves both must use the programmer).
- Left correct (already `ctx.Programmer`): `verb_info`, `verb_args`, `set_verb_info`,
  `set_verb_args`, `set_verb_code`.
- Out of F24 scope (left as-is): `respond_to` (L115 `ctx.Player`) — Toast's
  `bf_respond_to` gates on object `FLAG_READ` via `progr`, a separate discrepancy
  not part of this finding; `verb_code` owner-without-read-bit is its own red test
  finding, untouched.

## Test contract (`builtins/review_test.go`)
- `TestReview_AddVerbUsesProgNotPlayerForPerm`: owning programmer (#0) can `add_verb`
  when `ctx.Player=#5` (lowered perms); non-owner non-wizard programmer (#5, valid
  owner #0 in verbinfo) still gets `E_PERM`.
- `TestReview_DisassembleUsesProgNotPlayerForPerm` (new): owning programmer can
  disassemble a non-readable verb when `ctx.Player != ctx.Programmer`; non-owner
  non-wizard gets `E_PERM`.

## Before / after
- Against old `ctx.Player` code (sed-reverted), both tests FAIL:
  `--- FAIL: TestReview_AddVerbUsesProgNotPlayerForPerm`,
  `--- FAIL: TestReview_DisassembleUsesProgNotPlayerForPerm`.
- After fix: `go test ./builtins/ -run 'AddVerb|VerbInfo|VerbArgs|Disassemble|Perm' -v` → PASS.
- `go vet ./builtins/` → clean (exit 0).
- `go test ./builtins/...` → 5 pre-existing intentionally-red findings remain
  (`Data_IsMemberStrCaseSensitiveBug`, `Data_PcreMatchEmptySubject`,
  `IO_FileReadlinesBinaryMode`, `IO_QueuedTasksSortOrder`,
  `VerbCodeAllowsOwnerWithoutReadBit`); no NEW failures.

## Commit
`COMMIT_HASH_PLACEHOLDER` on `review/branch-stocktake-2026-06-25`.
