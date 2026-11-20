# Protocols Campaign skill cleanup checkpoint

- Active repository: `C:\Users\Q\code\protocols-plugin`, branch `master`, starting HEAD `d3ba0b60dc1fa029708b9bc04202ea88748934d5`; tracked state was clean.
- Installed Codex checkpoint hooks now resolve Git top-level first and explicitly emit accepted notes paths plus a deterministic `notes-<task>.md` fallback. Syntax and functional threshold/stop-gate tests passed. The installed hook directory is not Git-owned.
- Campaign Ward-removal slice is edited but not committed: all exact Ward/Foreman references were deleted from `plugins/protocols/skills/campaign/SKILL.md`.
- The focused repository contract now forbids Ward/Foreman in Campaign while leaving other skills' Ward expectations unchanged.
- Unrelated untracked files in `protocols-plugin` remain untouched. The protocols-repo checkpoint notes file is excluded from commits.
- Current blocker: none.
- Next action: run the focused contract and hygiene gates, commit Ward removal alone, then remove Campaign's redundant documentation/ledger commit ceremony in a separate slice.

## Checkpoint 2

- Standalone Ward-removal commit completed: `d6b6bf2` (`refactor(campaign): remove Ward coupling`). It changes only Campaign and the focused contract test; the targeted test passed.
- Active second slice deletes committed prior-art artifacts, manager prohibitions on measuring/editing, worker-owned ledger commits, ledger prerequisites, per-probe ledger duties, failed-evidence copying, dispatched synthesis updates, and the ledger template.
- Substantive campaign controls remain: metric/baseline/noise framing, budget and kill criteria, isolated candidates, directional triage versus confirmation, independent verification, and explicit authority before holdout.
- Search gate now has no remaining `ledger` or documentation-commit ceremony in Campaign after the final anti-pattern wording cleanup.
- Current blocker: none.
- Next action: inspect the complete reduced skill, update focused contracts if required, run lint/tests/hygiene, and commit the cleanup separately from Ward removal.

## Checkpoint 3

- The complete reduced Campaign skill was reread. It contains no Ward/Foreman references and no ledger, committed-search-artifact, manager-no-benchmark/source restriction, candidate-start documentation gate, evidence-copyback, or dispatched-synthesis requirement.
- A focused regression test now forbids those phrases from returning.
- Validation passed: frontmatter lint, two focused contracts, all 19 tests in `scripts/test_protocols_plugin.py`, and `git diff --check`.
- The second-slice diff is 70 deletions and 39 insertions across Campaign plus its regression contract; the skill itself is substantially smaller.
- Current blocker: none.
- Next action: stage only Campaign and `scripts/test_protocols_plugin.py`, verify the staged diff, and create the separate cleanup commit.

## Final checkpoint

- Ward removal is isolated in commit `d6b6bf2` (`refactor(campaign): remove Ward coupling`).
- Documentation-ceremony deletion is isolated in commit `2d62bce` (`refactor(campaign): delete documentation ceremony`).
- `protocols-plugin` tracked state is clean; pre-existing untracked investigation/Ghidra files remain untouched.
- Validation passed: all 19 protocol-plugin contract tests, all 3 Codex-native acceptance unit tests, all skill frontmatter lint, zero forbidden Campaign search hits, and `git diff --check`.
- The top-level `scripts/verify.sh` was not run because it explicitly invokes Ward. Installer doctor was also skipped because its implementation probes Ward. No Ward command was run.
- Installed Codex hook fix remains active outside Git: exact Git-root notes patterns are emitted by both checkpoint and stop gate; syntax and functional path-contract tests passed.
- Current blocker: none.
- Next action: none; handoff complete.
