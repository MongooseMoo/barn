# Current checkpoint 30

- PR #45 hardening is committed locally as `e1cae88` (`Prove natural exits and regenerate Toast fixtures`) on top of original `1cd9dd2`; 20 intended files, 390 insertions, 13 deletions.
- Final implementation: natural managed exit action with explicit timeout/exact code/no kill; byte-exact fixture comparison; deterministic startup recipe inputs and three managed provenance suites; exact panic suspended-task WAIF/anonymous ownership assertions; checksum and CI provenance gate; false-positive and workflow regressions.
- Gates green: focused provenance 3/3, focused shutdown/GC behavior 3/3, Ruff, mypy, 291 harness units, 11,593 packaged collection, duplicate baseline, checksum manifest, diff check.
- Full documented WSL Toast run: 11,511 passed, 79 skipped, 3 failed in 632.30s. Only failures are unrelated `finished_tasks_*` cases because that installed oracle binary lacks the builtin; one sibling correctly skipped on the same missing builtin. New PR #45 cases passed. CI builds pinned Toast with `SAVE_FINISHED_TASKS=15`, so the configured matrix is the authoritative proof.
- Next: push `e1cae88`, update draft PR #45 and issue #44 with exact evidence, watch the current three-cell Toast CI matrix and quality/ledger to green, then report new head to parent. Do not merge.
