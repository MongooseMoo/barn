# fix/fuzz-signature-check-line

## Goal
Stop different failed property checks in one fuzz target from sharing a
signature in `scripts/fuzz_report.py`. When a failed check has no Barn frames,
use the check's `file.go:NNN` location as `top` instead of "unavailable".

## State
Done, committed locally on `fix/fuzz-signature-check-line` (from origin/master).
Not pushed, no PR.

- `scripts/fuzz_report.py`: the `check` regex captures `file.go:NNN`; `report()`
  sets `top = check[1]` when `kind == "failed check"`, a check line exists and
  there are no Barn frames. Panic, divergence, recovered-panic, hang, crash
  signal and infrastructure signatures are unchanged. `publish()` untouched.
- `scripts/test_fuzz_report.py`: `test_failed_check_signature_is_its_location`
  uses the real native tee log `fixtures/fuzz/native/42692025227-baseline.log`
  (FuzzReadDatabase, `reader_fuzz_test.go:82:`): exact signature is
  sha256("FuzzReadDatabase|failed check|reader_fuzz_test.go:82"); another line
  number gives another signature; same line with different message values gives
  the same signature.
- `python -m unittest discover -s scripts -p test_fuzz_report.py`: 30 tests OK,
  none skipped (campaign-step tests ran under Git Bash).

## Decisions and why
- Location only, not message: messages embed input values (#707's message
  contains MOO map contents; #681's contain task numbers).
- Captured group in the existing `check` regex rather than a second regex, so
  first_error and the signature read the same line.
- Real producer: the native fixture is real Go 1.27.1 output with the format
  `        reader_fuzz_test.go:82: <msg>`; GitHub-prefixed logs have the prefix
  stripped before matching, so the same capture applies.

## Surprises
- Impact check over open "Nightly fuzz failure" issues (recomputing the old
  signature from each body matched the stored marker for all five):
  - #707 FuzzCheckpointResumeEquivalence, failed check at
    `checkpoint_resume_test.go:29`: signature changes.
  - #693 FuzzStructuredSourceExecution, failed check at `fuzz_test.go:47`:
    signature changes.
  - #681 FuzzReadDatabase, failed check at `reader_fuzz_test.go:82`: signature
    changes.
  - #695 (hang) and #692 (panic with Barn frame): unchanged.
  The next repeat of each changed bug, while its issue is still open, opens one
  new issue (with a claude-fix dispatch) instead of commenting on the old one.
- #681's comments show two different messages from the same line 82
  ("object #0 last_move differs" and "suspended task N frame 0 locals
  differs"). One round-trip check covers several bugs, so the location rule
  still merges those. Not changed here: the task said location only.

## Next step
Caller decides on push/PR. Possible follow-up for the caller: the README line
"These logs show no ordinary failed-check case" in `scripts/fixtures/fuzz/README.md`
is out of date (the native fixture is one), but that file is outside this task's scope.
