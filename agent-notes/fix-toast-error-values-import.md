# Fix: Toast-written error values import wrong in Barn

## Goal

Find and fix why Barn reads wrong integers for three error values (E_NONE,
E_RECMOVE, E_FILE) out of a checkpoint that Toast wrote. Failing test:
`server/error_dump_persistence.yaml::all_named_errors_survive_dump_and_restart`
in the Toast-to-Barn direction of the cross-engine checkpoint job.

## State

Fixed and committed on `fix/toast-error-values-import`; not pushed (the team
lead pushes and opens the pull request).

- Fix: `db/format/reader_value.go`, the `case 3: // ERR` branch of
  `readValueAfterType`, now keeps the low 32 bits (`int32(errCode)`).
- Go test: `TestReadValueKeepsLow32BitsOfStoredErrorNumber` in
  `db/format/reader_value_test.go`. Red before the fix (E_UNKNOWN for the four
  wide numbers), green after. `go test ./db/format -count=1` passes; `gofmt`
  and `go vet ./db/format` clean.
- Managed run, Toast writes and the fixed Barn resumes, selection
  `-m "admission or kept_db_restart" -k "capability_admission or
  error_dump_persistence"`: `2 passed, 13984 deselected in 13.97s`.
  Fixture `_db/Test.db` 2035 bytes, SHA-256
  `1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`; Toast
  `/root/src/toaststunt/build-release/moo`; harness files from the checkout at
  the `--restart-server-command` merge.
- The database Toast wrote in that run was kept by a scratch restart wrapper
  and holds, for the 19 errors in order: 96512210108416, 1, 2, 3, 4, 5, 6, 7,
  139281494441992, ...993, ...994, ...995, ...996, ...997, 96512210108430,
  96512210108431, 139281494442000, 96512210108433, 96512210108434. Twelve of
  the nineteen had a nonzero upper half in this run (three in the earlier
  runs), so which values go wrong varies from run to run.

Cause, from the source on both sides:

Cause (Toast source in WSL, `/root/src/toaststunt/src`):

- `include/structures.h:167-169`: `Var.v` is a union holding `Num num`
  (`int64_t`, line 48) and `enum error err` (4 bytes).
- `parser.y:381-382`: an error literal is built with
  `alloc_var(TYPE_ERR); $$->e.var.v.err = $1;`. Only the low 4 bytes of the
  union are assigned; the upper 4 bytes keep whatever the allocation held.
- `db_io.cc:369-375` (`dbio_write_var`): for `TYPE_ERR` it writes
  `dbio_write_num(v.v.num)`, the whole 8 bytes, so the file can carry a
  number whose upper 32 bits are leftover memory.
- `db_io.cc:177-182` (`dbio_read_var`): reads the number back into `v.num`.
  Every consumer then reads `v.err`, the low 32 bits
  (`numbers.cc:138-139` for `toint`). So Toast itself only ever uses the low
  32 bits of the stored number. A correct reading exists.

Barn: `db/format/reader_value.go:56-61` turns the whole number into
`types.ErrorCode`. The numbers Barn returned are the file's numbers, and each
has the right code in its low 32 bits:

- 8030606863672279040 = 0x6F72....00000000, low 32 bits 0 (E_NONE)
- 104994770518024 and 106734232272904 (two runs), low 32 bits 8 (E_RECMOVE)
- 104994770518032 and 106734232272912, low 32 bits 16 (E_FILE)

The upper halves differ between runs and look like heap addresses, which
matches leftover memory.

## Decisions and why

- Fix Barn's reader to keep the low 32 bits of a stored error number, since
  that is what Toast's own reader effectively does. No change to the test.

## Surprises

- The harness has no option to keep the database a server wrote. The file
  bytes were captured with a restart command that copies `{db}` before
  starting Barn (untracked scratch, `.tmp/barn-restart-keep-db.sh`).
- On 2026-11-19 `/mnt/c` inside the WSL Debian distro returned
  `Input/output error` for every path, so nothing under `C:\` could be run from
  WSL. Not diagnosed and not repaired (a remount or WSL restart would touch
  the CI runners). The managed run used copies piped into
  `/root/tmp-toast-error-values` in the Linux filesystem instead; that
  directory and the uv environment `~/.cache/barn-toast-error-values-linux`
  are left in place and can be deleted.

## Unverified

- The full cross-engine script was not rerun; only this one test plus
  admission, in the Toast-writes direction.
- Error codes read elsewhere from Toast task state (`db/format/reader_vm.go`,
  handler code lists and the pending error) go through integer values, not
  this branch, and were not examined for the same width problem.

## Next step

Team lead pushes the branch and opens the pull request.
