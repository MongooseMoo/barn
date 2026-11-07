# Completed differential comparisons

## Goal
Make the existing Barn/Toast differential lane reject an inconclusive zero-work
result while retaining completed comparisons, exclusions, aborts and native errors.
Root owns publication and CI; this worker owns only the script, its test file,
this note and a private PR draft.

## State
Implemented completed-pair accounting using the existing real lane fixtures.
The report now counts completed comparisons separately from eligible programs,
known aborts, unknown output and excluded corpus inputs. JSON retains existing
rows/exclusions and adds a summary plus parsed lane records. Misaligned output
still produces evidence instead of returning before the report is written.

Focused validation: `python scripts/test_fuzz_differential.py` returned
`Ran 18 tests in 0.247s` and `OK`; `git diff --check` emitted no diagnostics.
No real engine campaign or broader local tests ran. Publication and CI remain
root's responsibility.

## Decisions and why
- Reuse the existing CLI and lane/transcript owner; add no flags or wrappers.
- Known budget aborts remain visible and are not relabeled as Barn defects.
- Distinguish completed keyed outcomes from unknown/malformed lane output.
- Require positive completed-pair coverage for success. Genuine divergences
  remain failures even when another input is inconclusive.
- Root confirmed exit 1 for divergences/recovered Barn panics, exit 2 for zero
  completed pairs, unknown/malformed/unaligned output or one-sided aborts.
  Mixed completed pairs and known both-engine aborts may exit 0 with explicit
  partial coverage. Parsed MOO errors count as completed outcomes.
- Retain explicit recovered panics even when misaligned records prevent source
  attribution; those remain exit 1 with their raw records in the summary.
- Run only scripts/test_fuzz_differential.py locally after adding focused tests.

## Surprises
None yet.

## Next step
Root reviews the scoped commit, publishes its independent PR and runs CI.
The structured corpus decoder and reducer are separate future PRs from a fresh
default branch. This change adds neither flags nor workflow edits.
