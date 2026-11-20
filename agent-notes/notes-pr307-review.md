PR #307 has two P1 findings: suffix is not terminal completion; basename-based Toast cleanup can kill an unrelated listener.
Isolated worktree C:/Users/Q/code/barn-review-307 merged master c150b59; cleanup changes and five mocked regressions remain local.
Cleanup now records the launched Linux PID and uses pidfd plus exact executable/input/output DB paths; tests passed after reproducing old unsafe calls.
Mongoose do_command can return zero and fall through to native dispatch, so acknowledging only that hook is insufficient; terminal completion remains unresolved.
MONGOOSE_LOGIN_SCRIPT is absent; asked for an existing credential file/launcher path. Corpus benchmark can proceed after #305 merges; its queue CI is in cleanup.

#305 merged as 5249141; merged that master into the isolated #307 worktree.
Completed serial comparison: 17 realistic and 10 tight-loop workloads, five samples per engine, all values match; resized five rows into Toast 20-200ms.
Recorded report/raw results and profiling order; fixed stale callers mismatch prose; six ownership/error-preservation regressions pass and run in CI.
Pushed 3ec9f4d to tooling/bench-players-and-corpus. PR description explicitly retains terminal completion as a merge blocker; no queue action.
CI 35189297787 is running. Credential-path question remains unanswered; terminal protocol and live multiplayer report are still outstanding.

CI 35189297787 completed successfully on 3ec9f4d, including the new benchmark driver tests and full MOO conformance suite.
PR #307 remains open and unqueued; the terminal command-completion P1 is unresolved, as documented in its body.
The suffix-only mock still reports counted_ok=1 with terminal_completion=False; no live multiplayer goodput is claimed.
MONGOOSE_LOGIN_SCRIPT remains absent and no credential file/launcher path has been supplied.
Next: obtain the benchmark login path, implement and verify terminal acknowledgement across do_command and native dispatch, then run 1/16-player comparisons before queueing.
