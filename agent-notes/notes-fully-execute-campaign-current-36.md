# Campaign checkpoint 36

- Draft PR `MongooseMoo/moo-conformance-tests#45` remains open, draft, and unmerged. Adversary repairs are uncommitted in `C:\Users\Q\code\.worktrees\moo-conformance-shutdown-gc-edges` on top of `5c5132b`.
- All four blockers are implemented: ambiguous positive 134 rejected; natural exit terminal with log/file-only evidence and no cleanup/teardown reconnect; descriptor-based fresh/stable file evidence with traversal and symlink/reparse swap rejection; structural suspended-task activation/state parsing with exact WAIF/anonymous owners.
- Structural parser and freshness behavior passed the six managed WSL Toast focus cases.
- Full installed WSL Toast run completed: 11,511 passed / 79 skipped / exactly 3 known `finished_tasks` failures because the installed binary lacks the CI-enabled builtin; no new lifecycle/file/panic failures.
- Current local gates: 331 harness units passed; packaged collection 11,593; duplicate baseline 493 YAML / 4,525 tests exact; Ruff/mypy/checksums/diff green before the latest fail-closed snapshot adjustment.
- `ci/toast-never-executed.json` has 41 identities. PR #45 enables/removes none; no ledger edit is in scope. Root was informed.
- Latest hardening removed silent snapshot error suppression so freshness inventory fails closed. Next: rerun focused gates, final diff audit, delete temporary profile (already deleted), stage only six intended files, commit/push, update PR body with PR #22 reconciliation follow-up, then wait for all-profile CI.
