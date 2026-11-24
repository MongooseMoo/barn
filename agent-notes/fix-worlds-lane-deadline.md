# World comparison lane deadline

## Goal

The nightly ToastReadsBarnDatabase lane must finish inside its step limit and
write results, so cycle acceptance can judge the cycle.

## State

- Cause: `scripts/toast_reads_barn_db.py` `worlds_main` ran every retained
  FuzzWorldCheckpointEffects world serially with no deadline. Run 45396507340
  reached W03977 at the 35-minute step limit, wrote no results.json, and
  acceptance threw `ToastReadsBarnDatabase: outcome missing with step failure`.
  Every cycle from 2026-11-22 20:29 UTC to 2026-11-24 failed this way.
- Change: `--minutes` and `--seed`, as `scripts/fuzz_cross_engine.py` has.
  Built-in seeds run first, corpus worlds in sha256(seed + origin) order, and
  no world starts after the deadline. fuzz.yml passes `--minutes 25 --seed
  "$GITHUB_RUN_ID"`. Results record `ran`, `seed`, `minutes`.
- `test_toast_reads_barn_db` and `test_fuzz_compute`: 56 tests OK locally
  (Python 3.13). actionlint not run locally (blocked); CI checks the workflow.

## Decisions and why

- 25 minutes leaves 10 of the step's 35 for rendering overrun, one slow world
  and the report.

## Next step

After merge: the first cycle's ToastReadsBarnDatabase job writes results and
acceptance reports no `outcome missing` problem.
