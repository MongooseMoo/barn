# PR 94 current-state audit checkpoint

- Scope: read-only audit of Barn PR #94 / issue #93 against `origin/master` `95b5758` in `C:\Users\Q\code\.worktrees\barn-conformance-issue-12`.
- Constraints: current source/tests and live GitHub issue/PR only; do not read historical Markdown; no implementation edits.
- State: audit starting.
- Blocker: none yet.
- Next action: read the GitHub workflow skill, verify exact refs/worktree status, then inspect issue contract, PR files/commits/body/checks, current diff/conflicts, and framing tests.

## Findings

- Worktree head is clean except pre-existing untracked `reports/runs/`; PR head is `d3096fc`, one commit, six intended files, literal `Closes #93`.
- `origin/master` is `95b5758` (normal merge of #115), 47 commits ahead from merge base `aa82abf`; master push checks are green for build/unit and corrected default conformance.
- The PR behavior remains correct: server input sends the prefix before after-verb-miss intrinsic dispatch; result-only `EvalCommandOutput` lets notifications occur between that prefix and one result, and the server emits the suffix once afterward. Compile/runtime/panic recovery implementations each return one body record.
- Current tests cover exact prefix, notification, result, suffix order and compile/runtime delimiter singularity. The issue also names recovered panic; add an induced-panic framing regression during update for literal coverage.
- Classic `merge-tree` proves a real conflict in `scheduler/eval.go`: #115 added direct-eval execution leases, context ownership cleanup, GC sweep/start barriers, and deferred GC handling to the same function. These changes are compatible but must all be retained while applying only PR #94's result-only signature/output ownership delta.
- Current master also added `EvalCommandOutput` callers in `scheduler/deferred_gc_lifecycle_test.go` and `scheduler/run_gc_task_roots_test.go`; update them mechanically to the result-only API. No compatibility wrapper is allowed by issue #93.
- `scheduler/eval_fork_test.go` is changed by both sides; preserve #115's `ListenerPort() (int64, bool)` stub and cyclic-chain tests alongside #94's result-only caller changes.
- Expected final feature diff is eight files: original six plus the two new master test callers; recovered-panic coverage can stay in `server/input_processor_eval_test.go`.
- Update by normal merge of `origin/master` into the PR branch, not rebase. Resolve, then keep any added panic regression as a separate follow-up commit if practical.
- PR body provenance is inaccurate. `d3096fc` is not an ancestor of paired candidate `2d6e150`; they have different parents and trees. The six PR paths are byte-identical between those historical commits, so describe `2d6e150` as a byte-for-byte replay, not the exact PR commit. Treat that run as historical supporting evidence and replace verification with the exact updated-head results.
- The packaged `step_notifications_are_enforced` case is a full-suite safety check, not direct proof of intrinsic fallback framing: current conformance `Test.db` defines an executable `eval` verb on room #2, so `; code` normally takes the command-task path.

## Disposition

- UPDATE APPROVE, not merge-ready yet.
- Required gates after update: focused server framing including panic; all affected scheduler callers and #115 GC regressions; `go test ./... -count=1 -timeout=15m`; `go build ./...`; `go test -race ./server ./scheduler -count=1 -timeout=15m`; touched-file gofmt; `git diff --check`; managed focused notification safety case; corrected full default-profile conformance; GitHub build/unit and conformance required checks.
- After exact-head green and independent review, mark ready and normally merge with `gh pr merge 94 --repo MongooseMoo/barn --merge`, then verify the merge commit has two parents and issue #93 closed.
