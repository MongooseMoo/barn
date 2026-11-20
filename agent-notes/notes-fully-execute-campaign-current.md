# Current campaign checkpoint — 2026-08-04

## Current findings

- `moo-conformance-tests` PR #33 merged normally as `a46fa56603451cff34533ffc0b0f104dab5bdf1f`; the merge has two parents and issues #14/#16 are closed.
- Live conformance scope has seven open issues: #12, #13, #15, #17, #18, #23, and #24.
- PR #22 is the only existing conformance implementation. Its branch was merged with current `origin/main` as `011b0576876a53d7543fc4a9002d990d8d178196` and pushed. Focused expectation tests pass 45/45; all harness units pass 244/244. Exact-head Toast CI run 30976790482 is pending.
- Repository-wide Ruff currently reports 41 findings; this is a real missing portion of issue #17 and is not being mixed into PR #22.
- Issue #15 has a dedicated clean worktree at `C:\Users\Q\code\.worktrees\moo-conformance-issue-15`. The common cause is implicit session-global lifecycle ownership: setup failures are ignored, suite teardown is not called, cleanup accepts forms its executor cannot run, and mutable connection/permission/capture state leaks.
- Barn PR #94 (issue #93) and PR #96 (issue #95) are draft, mergeable, and have green current checks. PR #94 is paired with conformance PR #22. PR #96 proves the first-boot pending-anonymous preservation needed before conformance issue #24 can prove second-boot finalization.
- Open conformance PR #8 appears superseded by current-main anonymous caller-identity coverage; final equivalence review is required before closing it.

## Current observations

- Stable PR scope is being preserved: PR #22 will not absorb the repository-wide Ruff cleanup.
- The strengthened harness foundation from PR #33 is now the base for all remaining work.
- The original Barn checkout is highly dirty with unrelated user material; all implementation continues in dedicated worktrees with exact-path staging.

## Blockers

- No hard blocker. PR #22 cannot be merged until its exact-head Toast run and independent review finish, and its paired Barn proof is current.
- Issue #17 cannot become a required quality gate until the existing Ruff/mypy baseline is made green.

## Next actions

1. Finish and independently verify PR #22 plus Barn PR #94, then normal-merge and confirm both issue closures.
2. Complete issue #15 lifecycle/isolation TDD and managed Toast verification.
3. Implement issues #17/#18: complete quality gates, exact execution ledger/profile matrix, branch protection, and trusted paired-candidate CI.
4. Complete #23, #13, and #24 in dependency order.
5. Use the hardened conformance surface to cluster and close Barn #82–#92 and validate/merge #95, then audit every remaining scoped issue and merge commit.

## Checkpoint update — 2026-08-04 23:05 local

- Exact managed pairing of conformance PR #22 head `011b0576876a53d7543fc4a9002d990d8d178196` and Barn PR #94 head `d3096fc967c3fa6415c6eec05ed78f1ed53a88b0` collected all 11,569 cases but the three selected new cases all errored in session setup.
- Proven failure: Barn returned integer `1` for `server_version("options.OUTBOUND_NETWORK")`; the pinned-Toast-validated contract is `"ON"`, `"OFF"`, or `#-1`.
- Filed Barn issue #97 for the option-reporting contract. Its focused fix is delegated in clean worktree `C:\Users\Q\code\.worktrees\barn-issue-97-server-version-options`; PR #94 remains NO-MERGE until #97 lands and the exact paired run is green.
- Barn issue #91 is active in clean worktree `C:\Users\Q\code\.worktrees\barn-issue-91-go-hygiene`.
- Issue #91 red baseline on a clean checkout: exactly three tracked Go files fail gofmt (`cmd/barn/main.go`, `task/traceback.go`, `types/base_test.go`); `go vet ./...` fails only because `VM.ReadByte() byte` conflicts with the conventional `io.ByteReader` signature.
- Mechanical formatting and `ReadByte` to `FetchByte` call-site rewrite are currently uncommitted. The workflow gate patch, updated method comment, green tests/vet/build, review, PR, and merge remain next.
