# Keep fuzz worker crash output (#801)

## Goal
#801 (FuzzCheckpointResumeEquivalence) recurred five times, 2026-11-12 to
11-14, always "fuzzing process hung or terminated unexpectedly: exit status 2"
with "Top Barn frames: Unavailable". Q 2026-11-15: "The hang that keeps coming
back seems rather important". Find and fix its cause.

## State
- Cause of the missing evidence: Go 1.27 `internal/fuzz/worker.go:345` starts
  each worker with Stdout/Stderr unset, so a worker's panic on a non-test
  goroutine or runtime fatal error (both exit 2) is discarded. Not the test
  timeout: fuzz.yml sets `-timeout` to remaining+600 s.
- This branch: `internal/fuzzcrash` (init calls `debug.SetCrashOutput` to
  `$BARN_FUZZ_CRASH_DIR/crash-<pid>.txt`), blank-imported by a
  `fuzz_crash_output_test.go` in every fuzzed package; fuzz.yml sets the dir,
  clears it per run, and appends non-empty crash files to the run log, where
  fuzz_report.py already reads `panic:`/`fatal error:` and Barn frames.
- A local 30-min WSL campaign of the target with this capture is running to
  catch the actual crash.

## Decisions and why
- Crash text goes into the existing log rather than a new report field:
  fuzz_report already classifies panics and extracts frames from the log.
- A reported #801 recurrence will now get a new "panic" signature (different
  top frame), so it opens a fresh issue with the stack; #801 closes with the fix.

## Next step
- Read the local campaign's crash file; fix the cause with a regression test.
