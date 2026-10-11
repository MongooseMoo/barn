# Current checkpoint 37

## State

- Worktree: `C:\Users\Q\code\.worktrees\moo-conformance-shutdown-gc-edges`
- Branch: `agent/toast-shutdown-gc-edges`
- Draft PR: moo-conformance-tests #45, issue #44; do not merge.
- Pushed head: `a9af53c48b272355050435b2179155b5386ec54f` (`Harden terminal exit evidence`).
- Six intended files were explicitly staged and committed: startup panic YAML, runner.py, schema.py, server.py, test_managed_server.py, test_schema.py.
- Latest fail-closed snapshot change now propagates directory/stat errors instead of silently skipping them.

## Evidence

- Focused schema/managed tests: 125 passed.
- Full harness unit tests after latest change: 331 passed.
- Ruff: green.
- Mypy: green across 19 source files.
- `git diff --check`: green.
- Prior managed WSL Toast focus: 6 passed.
- Managed WSL Toast focus after the final fail-closed snapshot change: 6 passed.
- Main advanced to `7df5d1891a9849a10e3b5ec4f80235f1b5e03714` via merged PR #46. PR #45 was `CONFLICTING/DIRTY`, which proved why GitHub emitted no pull-request run for `a9af53c` even after an authorized close/reopen retrigger.
- Merged repaired main into the feature branch and reconciled three conflicts. PR #46's trusted-controller, candidate-root confinement, and staged admission behavior are preserved.
- Combined focused units: 173 passed; combined full harness units: 385 passed; Ruff and mypy green across 22 source files.
- Combined collection: 11,594 items (11,593 conformance plus one admission test); duplicate baseline remains exact at 493 YAML files / 4,525 definitions; all startup checksums pass.
- Workflow provenance now runs after canonical admission, consumes the exact admission evidence/context, and remains before the full packaged Toast suite.
- Local managed WSL Toast staged proof: admission 1 passed in 205.23s; then six provenance/behavior cases consumed its evidence and passed 6/6 in 14.18s.
- Prior full installed WSL Toast run: 11,511 passed, 79 skipped, 3 known `finished_tasks` failures because the installed oracle lacks CI's `SAVE_FINISHED_TASKS=15` build option.

## Findings

- Positive process status 134 is rejected; accepted abort forms are direct negative POSIX SIGABRT and Windows CRT status 3 only.
- Natural exit is terminal: only post-exit log/file evidence is allowed; cleanup and teardown do not reconnect or delay.
- `assert_file` uses a single descriptor with path/component validation, swap detection, stable reads, and launch/test-boundary freshness.
- Suspended-task evidence is structurally parsed from one activation/state block with exact WAIF/anonymous-object ownership.
- PR #45 changes none of the 41 never-executed ledger identities; those require separate campaign work.
- PR #22 touches runner/schema and needs landing-order reconciliation, without importing its expectation changes here.

## Final state

- Conflict-free pushed head: `5f304844bccce2ef77c9c568a72e9449457a1b21`, a two-parent merge of `a9af53c` and main `7df5d18`.
- PR #45 is OPEN, DRAFT, MERGEABLE, and CLEAN; literal `Closes #44` is present. It remains unmerged, so issue #44 remains open.
- Exact-head workflow 31019308002 is fully green: quality, three Toast profiles, and exact profile-union ledger.
- CI quality: 384 passed / 1 platform skip; 11,594 collected; duplicates, Ruff, and mypy green.
- CI admission and provenance: 1/1 admission and 3/3 fixture provenance on each profile.
- Full CI: 64-bit outbound on 11,522 passed / 71 reviewed skips; 64-bit outbound off 11,507 / 86; 32-bit outbound on 11,503 / 90.
- Exact union: 11,552 of 11,593 packaged cases executed; the same exact 41 reviewed never-executed identities remain.
- PR body and issue #44 comment contain the final exact evidence and PR #22 landing-reconciliation note.
- Worktree is clean.

## Blocker

- No blocker. PR #45 is ready for review but intentionally remains draft and unmerged.

## Next action

- Parent campaign decides review/landing order. If PR #22 lands first, reconcile its runner/schema expectations before merging PR #45. Do not squash; if eventually authorized to merge, use a normal merge commit and verify two parents plus issue closure.
