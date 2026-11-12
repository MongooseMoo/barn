# Fuzz retention keeps only records the target accepts

## Goal

Stop `scripts/fuzz_retention.py` restoring and re-capturing corpus records the
current fuzz target cannot read.

## Problem

Run 43127769101's FuzzRestoreVMSnapshotDoesNotPanic lane retained 8533 records:
4320 hold 4 values and 4213 hold 5. The target has taken 5 values since its
f.Fuzz callback changed to `(source string, ip int, stack uint8, handlerIP int,
shift uint8)` (vm/persistence_verify_test.go, commit "Fuzz saved-task restore
through source and positions instead of raw bytecode", 2026-11-10). Go 1.27.1
`internal/fuzz/fuzz.go` readCache ignores MalformedCorpusError, and ReadCorpus
skips every entry failing CheckCorpus ("wrong number of values in corpus entry"),
so the 4-value records were never fuzzed, while retention restored and captured
them every cycle. Every other lane's records all match its target.

## State

Done on branch `fix/fuzz-retention-arity`, not pushed:

- `target_values` reads the value count from the target's literal f.Fuzz
  callback parameters, or, for a callback built by a call
  (`rapid.MakeFuzz`, FuzzMapUpdatesPreserveSnapshotsAndInsertionOrder), from its
  first f.Add seed, which Go checks against the same callback. No per-target
  counts are written down. A target with neither fails the step.
- Restore skips mismatched records; capture deletes them from the retained
  inputs and manifest and does not add new ones. The summary and census.json
  carry `target_values` and `dropped_wrong_value_count`.
- Fixtures: the two real records under
  `scripts/fixtures/fuzz/FuzzRestoreVMSnapshotDoesNotPanic/`, named by SHA-256.

## Decisions and why

- Value count only, not value types: the defect found is a count mismatch.
  A same-count type change would still be skipped silently by Go.
- Signature, not "records Go wrote this cycle": a clean cycle can write none.

## Next step

Push, open the PR against master, watch the fuzz workflow's first capture
report about 4320 dropped records for the restore lane.
