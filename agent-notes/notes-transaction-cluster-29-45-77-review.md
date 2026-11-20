# Transaction cluster review checkpoint

- Reviewed exact head `09992c83716d624d914bfbe427f6d7ccbe6ed02a` in `C:/Users/Q/code/.worktrees/barn-fix-29-45-77-coarse-atomic`; reviewed worktree is clean and untouched.
- Confirmed topology: merge parents `aa33a8659aaa65a5f1d9ee2c26a7b944ad7884c9` and master `b3d730e8bd3579517124fd011126f0d514547ce2`; feature commits are one red-test commit and one implementation commit.
- Confirmed shared `preflightStagedToLiveLocked` runs before clock allocation or publication in coarse commit, flush, and decentralized commit.
- Confirmed failed operation preflight records a sticky terminal error, preserves physical staged/private/read/owned state, makes public `HasWrites` false, and leaves validation conflicts retryable with `HasWrites` true.
- Confirmed failed flush propagates to the generic coarse builtin boundary and does not reset the private view or resolve-cache ownership gate.
- Confirmed contents add is idempotent while remove-then-add still repositions at the requested MOO position.
- Scheduler production change is comments only; existing second lifecycle boundary is suppressed by terminal `HasWrites == false`, while the validation retry predicate remains unchanged.
- Exact-head focused package proof is green: `go test ./db/store ./builtins ./scheduler -count=1`.
- Exact-head race proof is green: `go test -race ./db/store ./builtins ./scheduler -count=1` (`scheduler` completed in 208.006s).
- Red proof used a disposable clone at exact master `b3d730e8bd3579517124fd011126f0d514547ce2` with only test commit `257b9d3` applied. All issue-specific groups failed for the intended reasons: coarse clock/publication changed, failed flush replaced private state, contents duplicated, terminal failure remained recommittable, and scheduler made three commit attempts instead of one.
- Audited all production `Commit`, `CommitAndRenew`, `FlushStagedToLive`, and coarse-flush call sites. Scheduler lifecycle commit sites consistently gate on public `HasWrites`; the validation retry branch still additionally requires `ValidationFailed`, preserving legitimate retries.
- GitHub issues #29, #45, and #77 remain open and their current bodies match the implemented regression contracts. No PR exists yet for head branch, so closing metadata and required CI/conformance remain pending external gates; the eventual PR must literally close all three issues.
- No actionable code finding. Remaining work: finish call-site/release edge audit and send final `CODE APPROVE` with metadata conditions.
