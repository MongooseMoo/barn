# Current campaign checkpoint

- Conformance recovery is complete: PR moo-conformance-tests#35 merged normally as `0615fc78544b806390906971942835e08a34d844`, two parents verified, issue #17 closed, and post-merge main run 30985285334 passed all three Toast profiles plus exact Full suite ledger.
- Barn PR #98 head `9d22061` fixes issue #97 and passes build/unit tests, but current conformance now fails closed at the next admission probe: `feature.ephemeral_listen` returns `E_INVARG`, producing 11,553 errors before cases run.
- Barn PR #101 head `4fe05db` fixes issue #100 by separating runtime descriptor port 0 from the OS-bound port; it is independently green for build/unit tests but conformance requires #97 first.
- Managed WSL Toast verification used the canonical managed server flow and current suite/profile. Focused server-admin cases `listeners_listen_unlisten_roundtrip`, `listen_duplicate_description_rejected`, and `unlisten_missing_listener_rejected` passed 3/3. Do not manually launch Toast or Barn.
- Current repo already contains a clean integration worktree `C:\Users\Q\code\.worktrees\barn-conformance-admission-integration`, branch `agent/conformance-admission-repairs`, head `5438206`. It preserves both component commits and has two explicit merge commits: `7541972` merges #97 and `5438206` merges #100. Diff is six intended files.
- Next action: obtain subagent integration-strategy verdicts, audit existing integration worktree, run local gates, publish an explicitly scoped PR closing #97 and #100, then require exact current conformance CI before any normal merge. Component PRs must not absorb each other's code.
- Do not read historical Markdown. Preserve all unrelated dirty/untracked material and checkpoint notes; never stage them.
