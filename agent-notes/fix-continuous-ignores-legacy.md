# Continuous fuzz chain ignores the nightly legacy run

## Goal

Q (2026-11-09, "fix it"): continuous dedicated discovery must not stop when the
workstation's nightly legacy fuzz dispatch overlaps a continuous cycle.

## State

`replenish()` in `scripts/fuzz_compute.cjs` now considers only dedicated runs
(title prefix `Nightly fuzz / dedicated-`) for its "newer dispatch" and
"another cycle exists" checks. Helper tests extended; 18 pass locally.

## Decisions and why

- Cause: both checks listed every `workflow_dispatch` run of fuzz.yml. The
  Windows scheduled task (`scripts/install-nightly-fuzz-task.ps1`) dispatches
  default legacy fuzz.yml at 03:17 Mountain (e.g. runs 42470762662,
  42599754611). A legacy run newer than, or running during, the end of a
  continuous cycle made the controller submit no successor, ending the chain.
- Legacy and dedicated runs never compete: fuzz.yml concurrency groups are
  `nightly-fuzz-legacy` / `nightly-fuzz-dedicated`, on separate runner pools.
- `run-name` (`Nightly fuzz / <compute-mode>[ / native-only]`) is set at
  dispatch, so queued runs carry the title. Duplicate protection inside the
  dedicated pool is unchanged.

## Next step

PR CI green, merge. Then observe the first continuous completion that overlaps
a nightly legacy run and confirm a successor is dispatched.
