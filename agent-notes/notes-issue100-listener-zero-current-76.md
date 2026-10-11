# Current issue 100 listener-zero state

- Barn PR #98 merged normally as `977f8cb04c1bfd522a6f553fcf71ef9b4012662a`; verified two parents; issue #97 closed.
- PR #101 worktree: `C:/Users/Q/code/.worktrees/barn-issue-100-listener-zero`.
- Toast source behavior previously verified: accepted connections through runtime descriptor zero retain source port zero in `connection_name(..., 0)` and `connection_info()`.
- Added tests-only commit `7bcd1c06b40cfc6d7d9bd8e200b72827607a1af6`.
- Red evidence: explicit listener descriptor zero was rendered as startup port 7777 by both connection metadata builtins; unset metadata already used the intended fallback.
- Current uncommitted implementation adds explicit listener-port presence to the connection contract, sets it in `SetListener`, and uses fallback only when metadata was never set.
- Focused regression is green. Server and scheduler dependent packages compile. `git diff --check` is green.
- Current modified implementation paths: `builtins/network.go`, `server/connection.go`, `scheduler/eval_fork_test.go`, `scheduler/mongoose_real_bench_test.go`.
- Blocker: none. Full local gates, implementation commit, master integration, independent review, managed paired verification, and PR merge remain.
- Next action: run focused/full Go gates, commit implementation explicitly, merge current Barn master into the feature branch, repeat gates, then push and run paired conformance.
