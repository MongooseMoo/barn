# Current checkpoint 49

- PR #49 merged normally as `6068c535159e352c159eb1c453aec61f013bedbb`, with exactly two parents: prior main `7df5d189...` and head `a738975...`. Both parents are ancestors; issue #47 intentionally remains open.
- Final prerequisite run `31024918228` passed quality, 32-bit Toast, 64-bit outbound-off Toast, 64-bit outbound-on Toast, and Full suite.
- New `origin/main` was merged into PR #48 branch with normal merge commit `e4273354d3fe9ed70670fa61ecf8afb30a9c00f7`; parents are prior strict head `adc097d...` and new main `6068c53...`. No rebase.
- Conflict resolution: semantic `gc.yaml`, `anonymous.yaml`, `verb_matching.yaml`, and `limits.yaml` exactly match new main; strict branch keeps the legacy baseline deleted and keeps its baseline-free ledger tests.
- PR #48 final net diff versus new main is exactly five files: workflow, deletion of `ci/toast-never-executed.json`, `execution_ledger.py`, ledger tests, and workflow tests.
- Local strict-head proof: 333 harness units passed; 11,581 items collected (one admission plus 11,580 packaged); Ruff and mypy passed; diff check clean; worktree clean.
- Next action: push `e427335`, let PR #48 managed workflow execute exact literal head and compare candidate inventory against trusted main; require three Toast profiles and Full suite exact 11,580/11,580. Then fresh analyst/adversary review, update body/ready state, audit, normal-merge, verify two parents, and confirm issue #47 closes.
- Preserve dirty original checkout and all untracked Barn notes; do not read historical project Markdown.
