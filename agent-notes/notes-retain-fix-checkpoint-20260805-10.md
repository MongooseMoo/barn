# Retain/fix checkpoint

- Conformance issue 67 and PR 68 now track the optional `finished_tasks` guard.
- Red managed stock WSL Toast: admission passed, three behavior cases failed because the builtin is absent.
- Green focused proof after literal capability guards: admission passed and all three cases were declared skipped under `--fail-on-unexpected-skip`.
- PR 68 head `ad5791e`; harness 459 passed, duplicate baseline exact, Ruff/mypy/diff checks green. Exact diff needs immediate audit because commit stats show 10 additions/7 deletions rather than the expected three net added guards.
- PR 112 corrected final-checkpoint run is active; PR 96 update/audit is active; PR 109 exact empty vector update is active.
- Conformance lifecycle commit `6676536` is under independent review; PR 114 GC independent review remains queued.
- Do not merge until exact diffs/commits/checks/closing phrases are audited and required checks are green; use normal merge and verify two parents.
