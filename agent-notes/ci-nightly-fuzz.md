# Nightly fuzz workflow

## Goal
Item 5 of the fuzzing expansion Q approved 2026-11-01: a scheduled job runs
every fuzz target for a fixed time (plan in barn master checkout
agent-notes/fuzzing-expansion.md).

## State
.github/workflows/fuzz.yml: daily 09:17 UTC plus workflow_dispatch; one matrix
job per target, max-parallel 2, 10 minutes each with -parallel=2, 25-minute
job limit; on failure uploads <package>/testdata/fuzz/<target>/ for 30 days.
actionlint passes. docs/parser-fuzzing.md updated.

## Decisions and why
- Runner labels, /dev/null check, checkout and setup-go copied from ci.yml so
  the job runs where PR CI runs.
- max-parallel 2: the eight runners share Q's workstation with PR CI.
- checkout cleans the workspace, so a crasher from an earlier night does not
  linger; Go's fuzz cache in GOCACHE persists on each runner, so coverage
  carries over.

## Merge order
- FuzzCommandLine exists only after Barn #569 merges; merge #569 first.
- FuzzRestoreVMSnapshotDoesNotPanic fails on master within a second (verifier
  has no stack-height check) until the stack verifier lands. That red job is
  the known defect, not a workflow problem.

## Next step
PR CI (this workflow does not run on PRs); after merge, one workflow_dispatch
run to confirm each job starts.
