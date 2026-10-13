# Retain/fix checkpoint

- PR 112 head `54298f4` keeps normal final checkpoints; new CI run is active.
- Existing PR 96 is being audited for the sole real Anon2/#95 mismatch; current base passes Anon1/3/4/5/6 with normal checkpointing.
- Conformance lifecycle repair commit `6676536` is clean and under independent review: 7 files, 467 unit tests plus Ruff/mypy green.
- Corrected default-profile WSL Toast full run passes all startup cases and has only three failures: optional `finished_tasks` behavior tests run despite the stock build omitting that builtin.
- Fresh conformance branch `agent/guard-optional-finished-tasks` reproduces admission pass plus all three `finished_tasks` failures against pinned WSL Toast.
- Added the existing canonical `skip_if: "missing builtin.finished_tasks"` condition to each of those three behavioral cases; green proof, issue/PR, and full harness gates remain next.
- PR 109 exact empty-password known-answer assertion is being added; PR 114 independent review remains queued after lifecycle review.
