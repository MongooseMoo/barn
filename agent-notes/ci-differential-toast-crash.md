# ToastDifferential: a Toast panic is one probe's result, not a lane failure

## Goal
Issue #811 ("Nightly fuzz infrastructure failure: ToastDifferential", first
error `RuntimeError: toast exited 134; see fuzz-differential/toast_raw.txt`)
was reported by every fuzz cycle from 2026-11-14 to 2026-11-16, so every
cycle failed acceptance. Make the lane finish and say which program Toast
crashed on.

## State
- Evidence, run 43806699303 (artifact fuzz-failure-ToastDifferential,
  toast_raw.txt): the process for inputs 2681..2700 printed results for
  F02681 and F02682, then
  `*** PANIC: Caught signal 11`, `#-1:Input to EVAL (this == #-1), line 7:
  server panic`, wrote a PANIC dump and aborted; the lane shell exited 134.
  The next input, F02683, has on line 7 of its eval:
  `while o (`url_encode("a ") ! ANY'[2147483648..n + n] ? n | n)` (n = 27,
  o = #3).
- Cause in the harness: `run_bounded` in scripts/fuzz_differential.py raised
  RuntimeError on any non-zero exit other than 124 and 137.
- Fix: on the toast lane, exit 134 with `*** PANIC:` in the process's output
  records the probe after the last result as CRASHED, lists it under
  "crashed" in the summary and the report, and restarts the chunk without it
  (the same replay TIMED_OUT and KILLED use). CRASHED counts as aborted on
  Toast, so the pair is compared by the existing aborted rules.
- Tests in scripts/test_fuzz_differential.py, written first: a fake engine
  returning 134 with the panic line, a real shell whose stand-in Toast prints
  the panic line and sends itself SIGABRT, and that Barn exiting 134, or
  Toast exiting 134 without a panic line, still raises.

- This does not make the lane green. Before the crash appeared (reports of
  2026-11-14 07:11 to 11-15 19:12 UTC) the lane already ended non-zero: run
  43737913398's report says outcome "divergence", 6390 completed
  comparisons, 5251 match, 1139 divergence, 488 unknown. Of the divergences
  about 1070 are programs Toast refuses to compile and Barn runs (shortest:
  `{a,} = {1}; return rest;`, `return {.21>.2, 2,.21>.2,  }[^];`,
  `try retur/ 0; except e (ANY) return e; finally 1; endtry`). Of the
  unknowns 478 are Barn printing `Error: E_MAXREC` (internal/dbtool
  resultLine prints a tick or seconds abort that way) where the script only
  knows `=> *Aborted*` as an abort. Issue #811 files all of it as
  "infrastructure" with first error "barn: `Error: E_MAXREC`". Those are
  separate jobs, recorded in the main checkout's
  agent-notes/machine-outage-and-findings.md.

## Decisions and why
- Toast lane only: a Barn process that dies is a Barn defect and must stay a
  hard failure.
- The panic line is required as well as the exit status, so another cause of
  exit 134 is not taken for a Toast panic.

## Unverified
- Why Toast segfaults on that program (not read in Toast's source). The
  pinned Toast is the oracle; nothing in Barn changes for it.
- What Barn returns for F02683 was not read from barn_raw (the lane raised
  before Barn ran).
- A pair where Toast crashed and Barn returned a value is "aborted on toast",
  which makes the run inconclusive (exit 2), as for a time-out.

## Next step
Read the first cycle's ToastDifferential report after merge: F02683 (or
whichever input) under "crashed on toast", and the lane's outcome.
