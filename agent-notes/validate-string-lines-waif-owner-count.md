# validate_string_lines and waif_owner_count

## Goal

Add to Barn the two builtins the Mongoose ToastStunt fork registers and Barn
lacked, matching the fork: `validate_string_lines(LIST)` (on Mongoose's in-MOO
verb editor save path) and `waif_owner_count(OBJ)`. Nothing else.

## State

- Candidate tests `tests/mongoose-conformance/builtins/validate_string_lines.yaml`
  and `waif_owner_count.yaml`, committed before the Barn change.
- Barn change: `builtins/lists.go`, `builtins/objects_hierarchy.go`,
  `builtins/catalog.go` (appended to `builtinLayout`),
  `db/store/store_lifecycle.go` (`Store.WaifCountByOwner`).
- Managed candidate lane on Barn: before the change 19 skipped ("Requires
  builtin"), after it 20 passed (19 + admission).
- Toast first: `validate_string_lines` 11 of 11 and `waif_owner_count` 8 of 8
  passed on the fork built at the live commit, through the managed harness
  (profile `profiles/toast/mongoose-0ff6fd7-wsl-testdb.json`, added by the
  branch `managed-toast-live-commit`). What this note says below about no
  managed Toast having the builtins describes the state before that build.
- Pull request open.

## Decisions and why

- Fork behaviour read from ToastStunt `mongoosemoo/mongoose`: `src/list.cc`
  `bf_validate_string_lines` (rows checked in order; non-string E_TYPE, LF/CR
  E_INVARG; returns 1) and `src/waif.cc` `bf_waif_owner_count` (E_PERM unless
  asking about the programmer itself or a wizard; count of allocated waifs with
  that owner, 0 if none; no validity check).
- Both registered with `withCapability(config.BarnExtensions)`, as `tz_offset`
  is: they are not stock Toast builtins.
- `waif_owner_count` scans `waifRegistry` and counts entries still reachable.
  Callers in the Mongoose database are test fixtures only, so a scan per call is
  acceptable.

## Surprises

- The pinned WSL Mongoose Toast oracle (`profiles/toast/mongoose-wsl-mongoose.json`,
  source commit 2026-02-19) predates both builtins (added to the fork
  2026-10-13), and the candidate lane runs stock Toast on Test.db, which lacks
  them. No managed Toast-first proof is possible until the Mongoose oracle is
  rebuilt from the current fork.
- Waif lifetime differs: Toast frees a waif at refcount zero (or after its
  pending recycle), Barn forgets it only after a Go GC. So the count after
  dropping the last reference can stay higher on Barn until GC. Same model as
  `waif_stats`; not changed here.
- The worktree's `.git` file holds a Windows path, so `git` inside WSL fails in
  it; the managed run was built with `go build -buildvcs=false`.

## Next step

Rebuild the WSL Mongoose Toast oracle from the current fork (needs approval),
then run these two candidate files against it to complete Toast-first proof.
