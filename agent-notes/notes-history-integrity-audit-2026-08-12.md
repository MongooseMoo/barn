# Barn merged-PR history integrity audit (2026-08-12)

## Scope and snapshot

- Read-only audit of every merged pull request returned by GitHub for `MongooseMoo/barn`.
- GitHub currently reports 95 merged PRs, from #46 (2026-07-30) through #206 (2026-08-12).
- Current audited remote tip after refresh: `origin/master` = `1884db7320a691f9478cd28495158c164e47f27b` (PR #206).
- The existing large untracked working set was not modified except for this untracked note. No tracked files were changed, staged, committed, pushed, or merged.

## Findings

- All 95 merged PR landing commits are reachable from the refreshed `origin/master` snapshot.
- 82 landings are proper two-parent merge commits. In every case, the merge commit's second parent exactly equals GitHub's recorded final PR head.
- 13 PRs used one-parent history-flattening landings: #134, #143, #149, #150, #151, #152, #156, #167, #187, #188, #194, #195, and #204.
- For all 13 one-parent landings, the stable patch ID of the PR diff equals the landing patch. Their code changes landed, but their commit topology/history was flattened. Several flattened multi-commit PRs lost commit boundaries (#167, #188, #194, #195, #204).
- #188 was intentionally and exactly reverted by normal-merge PR #190 because its squash topology and implementation were invalid. Corrected work was re-delivered with TDD in normal-merge PR #191. This is explicit recovery, not silent loss.
- Branch protection is currently strict, admin-enforced, requires `Build & unit tests` and `MOO conformance suite`, disables force pushes and deletions on `master`, and does not require linear history.

## Force-push audit

- GitHub records 37 force-push events across 18 merged PRs.
- Comparing each before/after head with `git cherry` found 35 patch-equivalent rewritten commits and four non-identical rewrites.
- The four non-identical rewrites were manually inspected:
  - #152: only changed `make([]ActivationFrame, 1, 1)` to `make([]ActivationFrame, 1)` in a regression test; same effective length/capacity.
  - #197: deliberately replaced a non-portable dump-format identity encoding with a sidecar design and expanded coverage; final normal merge preserved the redesigned work.
  - #206: integrated collection-assignment evaluation-order work with already-merged generalized deep-range assignment logic while retaining the 67-line regression test.
- No force-push event inspected shows an unmatched feature commit silently disappearing.

## Internal merge audit

- 28 merged PRs contain 60 internal two-parent commits (typically merging `master` into a feature branch).
- Recomputing each with `git merge-tree --write-tree` found 56 clean merges whose resulting tree exactly matches Git's automatic merge tree.
- Four internal merges required manual conflict resolution:
  - PR #94, `scheduler/eval.go`: combined server-owned eval framing with the newer GC serialization/deferred-finalization path. Combined diff retains both sides.
  - PR #115, `vm/anonymous_gc_test.go`: combined all three compatible regression groups; PR body and combined diff explicitly retain staged reachability, frozen candidates, and allocation independence.
  - PR #116, `builtins/store_reads.go`, `db/store/store_txn.go`, and test relocation to `engine/terminal_commit_test.go`: resolution keeps atomic terminal-transaction behavior and the concurrent runtime package move. Combined diff shown for the core transaction logic and relocated regression.
  - PR #127, `bytecode/disassemble.go`: resolution includes both `OP_END_FINALLY` from master and new dynamic/wide name opcodes from the feature.

## Current assessment

- No evidence of code or regression work being silently lost through force-pushes, rebases, or internal merge conflict resolution in the audited snapshot.
- There is a real history-integrity violation: 13 merged PRs were flattened instead of normally merged. Content patches were preserved, but commit identities/boundaries were not.
- The only explicit recent removal is the intentional #188 -> #190 rollback, followed by corrected #191 re-delivery.

## Final verification

- Current-tree survival checks found the key implementation symbols and regression tests from #94, #115, #116, and #127 on `origin/master`, including server-owned eval framing, all three anonymous-GC regressions, terminal transaction state/tests, and dynamic/wide static-name opcodes and boundary coverage.
- Local tracked HEAD `5fa3612e326f8e4f66c37392b9a3708eba1e15ad` (the master tip immediately before #206) passes `go test ./... -count=1` across all packages.
- Remote exact tip `1884db7320a691f9478cd28495158c164e47f27b` and PR #206 head `b9127194aaa2bf9fbdba6a920ab5a323569835db` both have successful required `Build & unit tests` and `MOO conformance suite` checks.
- Final `git ls-remote` agrees with the fetched `origin/master`; GitHub still reports 95 merged PRs and #206 as the latest merge. This is the report cutoff.

## Next action

- No repair is indicated for lost code because none was found.
- Prevent recurrence by keeping squash/rebase merge disabled or operationally forbidden and by continuing to verify two parents plus second-parent equality before every merge.
