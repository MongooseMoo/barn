# Current campaign checkpoint

- Integration architecture for Barn #97/#100 still requires explicit user approval because issue #97 literally says its fix must land independently; no integration PR was opened. A remote integration branch exists but is now stale because #101 gained tests.
- Barn PR #101 head is now `df0f8ac8b77ae9ab8458d78341f71218766e13c8`. Added test-only WebSocket and TLS descriptor-zero lifecycle coverage. Exact tests fail on master, pass 10 consecutive runs on #101, full server tests pass, and no new production defect was found. PR remains draft/red only on preceding #97 admission.
- Conformance issue #18 worktree: `C:\Users\Q\code\.worktrees\moo-conformance-issue-18-paired-ci`, branch `agent/issue-18-paired-candidate-ci`, base `0615fc7`.
- TDD red captured: five workflow contract tests failed because paired workflow was absent; paired-result tests initially failed because module was absent.
- Current uncommitted issue #18 implementation adds `.github/workflows/paired-candidates.yml`, `src/moo_conformance/paired_result.py`, `tests/test_paired_candidate_workflow.py`, and `tests/test_paired_result.py`. Focused 15 tests pass.
- Design hardened after independent exploration: trusted `repository_dispatch` on protected conformance main; fixed repos; exact lowercase 40-hex SHAs; read-only token; pinned actions; no credential persistence/cache/secrets; expected failure requires pytest exit 1 and an exact named bad case; exact JUnit identity surface.
- Remaining issue #18 work: add Barn build/unit and conformance quality prerequisites, generate/gate explicit oracle and Barn target profiles, isolate final validation/provenance in a trusted job, expand validator edge tests, run full gates/adversary review, then publish a bootstrap PR without prematurely closing #18. Workflow must merge before live red/green dispatch proof can exist.
- Do not read historical Markdown. Preserve all dirty/untracked work and notes; stage only explicit intended paths.
