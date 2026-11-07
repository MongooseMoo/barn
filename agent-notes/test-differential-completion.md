# Completed differential comparisons

## Goal
Make the existing Barn/Toast differential lane reject an inconclusive zero-work
result while retaining completed comparisons, exclusions, aborts and native errors.
Root owns publication and CI; this worker owns the differential script and test,
the existing fuzz reporter and test, this note and a private PR draft.

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

Repaired the existing reporter consumer after root's private review: an explicit
recovered Barn panic summary is a panic finding even without a source block.
The native text remains first_error/log evidence; source, reproduction command
and frame stay unavailable. An offline issue/dispatch regression verifies its
bug label and repair routing. `python scripts/test_fuzz_report.py` returned
`Ran 23 tests in 1.477s` and `OK`.

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
- Run only the owned differential or reporter test file for its focused change.

## Surprises
The existing consumer recognized only source-attributed differential divergence
blocks, so an unattributed explicit panic initially reached infrastructure.
The fix extends that existing consumer rather than inventing source attribution.

## Next step
Root reviews the reporter repair, publishes the PR update and runs CI.
The structured corpus decoder and reducer are separate future PRs from a fresh
default branch. This change adds neither flags nor workflow edits.
