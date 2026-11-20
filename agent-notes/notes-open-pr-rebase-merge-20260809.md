# Open PR rebase and merge checkpoint (2026-08-09)

## Request

Rebase every open PR onto `master`, watch required CI to green, then normal-merge each PR while preserving commit history.

## Repository state

- Repository: `MongooseMoo/barn`; base/default branch: `master`.
- Main checkout is on `master` with no tracked modifications and extensive unrelated untracked material. Preserve all of it.
- Open PRs at inventory time: #153, #154, #155, #157. All are non-draft, authored in the main repository, and initially reported `BEHIND`.
- Required Barn checks are `Build & unit tests` and `MOO conformance suite`.
- Merge method is normal merge only: `gh pr merge <number> --merge` after the final audit.

## PR audit findings

- #153 `Honor task list limits in decode_binary`: one feature commit; files are `builtins/crypto.go` and `builtins/crypto_test.go`; body contains `Closes #71`; patch changes grouped `decode_binary()` to `CheckListLimitForTask` and adds the pending server-options regression test.
- #154 `Document hash and HMAC string-limit exemption`: one feature commit; files are `builtins/crypto.go` and `builtins/crypto_test.go`; body contains `Closes #73`; patch documents the Toast exemption at all six hash/HMAC sites and tests all six with `max_string_concat = 1`.
- #155 `Lock reading and completion task fields`: feature commit plus an intermediate merge-from-master commit; six scoped task/runtime files; body contains `Closes #63`; patch adds locked accessors and atomic callback take semantics. Rebase should remove the intermediate merge commit.
- #157 `Use opaque WAIF identity tokens`: feature commit plus empty `Retry CI` commit; seven scoped WAIF/map/store/consumer files; body contains `Closes #139`; patch introduces opaque comparable `types.WaifIdentity`, typed map hashes, consumer migrations, and a zero-allocation lookup assertion without adding WAIF lifecycle behavior.
- `git diff --check` is clean for all four pre-rebase PR diffs.

## CI observations

- Pre-rebase `Build & unit tests` passed on all four PRs.
- Pre-rebase `MOO conformance suite` failed on all four PRs.
- The available failed-log output shows a shared six-failure conformance shape: bcrypt 2a wizard `E_QUOTA`, three fixture byte mismatches, panic shutdown status mismatch, and deferred WAIF recycle shutdown timeout; one summarized run ended `6 failed, 11185 passed, 420 skipped`.
- This failure cluster is not specific to the narrow diffs in #153/#154/#155 and was also present on #157. Exact per-run tail extraction remains to be completed because the first log collection was too large and truncated.

## Blocker

- Issue #63 explicitly requires a deterministic tie-break for multiple reading tasks (oldest `QueueSeq`) or an ambiguity error. PR #155 still returns the first matching task from Go map iteration. Merging it with `Closes #63` would close an incompletely implemented issue and violates the final scope/contract guardrail.
- Do not choose between oldest-task selection and ambiguity error silently. Surface this mismatch before merging #155 unless the current `master` rebase resolves it independently.

## Next action

1. Extract concise failure summaries for the pre-rebase conformance runs and compare against current `master` CI.
2. Rebase PRs sequentially in oldest-first order against the then-current `origin/master`, using an isolated temporary worktree and `--force-with-lease` against the audited old head SHA.
3. Watch both required checks after each push. Do not manually launch Barn or Toast.
4. Before each merge, re-audit exact files, commits, closing phrase, issue scope, and checks; use `gh pr merge <number> --merge` only.
5. Verify merge commit has two parents and the closing issue is closed. Stop #155 before merge if the deterministic-reader requirement remains unresolved.

## Progress checkpoint after sequential merges

- Current repaired base before this delivery was `240be17117f03e24bc85373eb399812d75c81d66`; its CI run `31340424115` was green. The earlier shared six-failure cluster was repaired on `master`, so it was appropriate to rebase rather than alter the feature PRs.
- PR #153 rebased to `51056c42ffe4162bc9bfd3618e9e261a2a17ba6e` directly on `240be171`; focused/full Go tests passed locally. CI run `31349860885` passed both required jobs. Its artifact had four passed admission probes and JUnit totals 11,611 tests, 11,191 passed, 420 skipped, 0 failures/errors. Final audit remained one commit, two files, and `Closes #71`. Normal merge command `gh pr merge 153 --merge` produced two-parent merge `1ba92cb40b97b566e8e7ed2b2d0b3e73f8994d58` with parents `240be171` and `51056c42`; issue #71 closed.
- PR #154 then rebased to `0ebe32e4f3f197467a94958677e3d04a9ff8922d` directly on `1ba92cb`; focused/full Go tests passed locally. CI run `31350168096` passed both required jobs. Its artifact had four passed admission probes and the same clean JUnit totals. Final audit remained one commit, two files, and `Closes #73`. Normal merge command `gh pr merge 154 --merge` produced two-parent merge `a2577ceeabce56c0d51b7ab86ff9cb555b6ac03a` with parents `1ba92cb` and `0ebe32e`; issue #73 closed.
- PR #157 rebased without conflicts onto `a2577cee`. Rebase preserved two commit boundaries: feature `062b57136a25b009becb779acd84c3d83a7b03ed` and genuinely empty `Retry CI` commit `cab9098110bfa6c5e21d8107b187d5d1ecc84aea`. The empty commit was intentionally preserved rather than silently deleting history. The seven-file feature diff was unchanged and `git diff --check` was clean. Exact leased force-push succeeded.
- Correction: the reported focused/full local Go tests for #157 accidentally ran in the main checkout because the commands did not set the temporary worktree as their working directory. They did not validate the rebased head. CI correctly caught this mistake.
- CI run `31350493124` failed early on both jobs at head `cab9098`: build/unit failed in Vet and conformance failed in Build barn. Current master added `db/store/store_reachability.go` and `db/store/store_snapshot.go` consumers keyed by `unsafe.Pointer`; after #157 changes `WaifIdentity()` to return `types.WaifIdentity`, those map indexes no longer compile. The feature branch must mechanically migrate all new reachability/snapshot WAIF visited/rewrite maps to `types.WaifIdentity` and remove now-unused `unsafe` imports, then run the actual branch tests and CI.

## Current state, blocker, and next action

- Live work is blocked pending approval for the focused #157 rebase integration repair described above. Do not merge its failing head.
- After an approved repair, run focused and full tests from the actual isolated worktree, plus vet/format/diff checks, push a scoped repair commit, and watch both required checks. If green, inspect admission/JUnit and repeat the final files/commits/scope/closing-phrase audit before `gh pr merge 157 --merge`; then verify two parents and issue #139 closure.
- After #157 merges, rebase #155 onto the then-current master. Its unresolved blocker remains: issue #63 requires deterministic handling when multiple tasks read the same player, while the PR still selects the first matching Go-map entry. Do not merge #155 with `Closes #63` until that architecture choice is implemented or explicitly directed.

## Authorized continuation checkpoint (2026-08-10)

- User explicitly authorized the #157 repair and delegated the #155 deterministic policy choice.
- Chosen #155 policy: smallest `QueueSeq` (oldest enqueued task), with smallest task ID as a deterministic fallback when sequences tie. This preserves the existing one-reader delivery behavior and avoids introducing a new ambiguity error into the connection path.
- #157 repair enumerated every current consumer of `WaifIdentity()`. The initial compiler errors named two `db/store` files; complete enumeration found five new-master consumers: `db/format/writer_vm.go`, `db/store/store_reachability.go`, `db/store/store_snapshot.go`, `vm/anonymous_gc.go`, and `vm/waif_gc.go`. All visited/rewrite maps were mechanically migrated from `unsafe.Pointer` to `types.WaifIdentity`; no tracked `map[unsafe.Pointer]` remained.
- #157 actual-worktree verification passed: repository formatting, `git diff --check`, `go vet ./...`, focused `types`/`builtins`/`vm`/`db/store`/`db/format` tests, full `go test ./... -count=1`, affected-package race tests, and focused identity/allocation tests. Repair commit `879c815601ae3a4f816415f4be5b148cb5104c20` preserved the prior feature and empty retry commits.
- #157 CI run `31403340684` passed build/unit in 1m34s and managed conformance in 4m41s. Artifact audit: four admission probes passed; JUnit 11,611 total, 11,191 passed, 420 skipped, 0 failures/errors. Final audit was 12 identity-consumer files, three commits, CLEAN/mergeable, literal `Closes #139`.
- `gh pr merge 157 --merge` produced normal two-parent merge `5e1153935f9a206ca247683b6649880b1bc0082e` with parents `a2577ceeabce56c0d51b7ab86ff9cb555b6ac03a` and `879c815601ae3a4f816415f4be5b148cb5104c20`; issue #139 closed.
- #155 rebased cleanly onto `5e115393` as feature commit `e4feb11`; its old intermediate merge commit was removed by the requested rebase. The rebased pre-policy diff is six files, 96 insertions, 11 deletions, and passes `git diff --check`.
- Added red tests in `task/manager_test.go` for oldest sequence and equal-sequence ID fallback. Against current `FindReadingTask`, the tests failed immediately: it selected task 1024/sequence 24 instead of task 1001/sequence 1, and task 1051 instead of lowest-ID task 1001.

## Current next action

1. Implement an atomic task-mutex reading-order snapshot and scan all matching readers for the minimum `(QueueSeq, ID)` pair.
2. Run the focused tests green, then race/full/vet/format/diff verification from the actual #155 worktree.
3. Commit the deterministic selection as a separate post-rebase issue-completion commit, push with an exact lease against old remote head `b18c9714ce244eeee31a0f9baad8981d4ee48cec`, and watch both required CI checks.
4. Only after artifact and final scope/commit/closing-phrase audit, normal-merge #155 and verify two parents plus issue #63 closure.

## Final completion checkpoint

- Implemented #155 selection with an atomic `readingOrder` snapshot under `t.mu`, minimum `QueueSeq`, and minimum task ID for equal sequences. Focused tests passed 100 runs after the observed red failures; focused race tests passed 10 runs.
- Full #155 actual-worktree verification passed: repository formatting, `go vet ./...`, `go test ./... -count=1`, full `go test -race ./task -count=1`, affected engine/builtins race checks, and `git diff --check`.
- Deterministic completion commit `241b361c36da096c2c4fd2b914f55c3a224fa360` was added after rebased locking commit `e4feb113f72b013bef78d9fe0ae9cce39032f346`; exact force-with-lease against old remote head `b18c9714ce244eeee31a0f9baad8981d4ee48cec` succeeded.
- #155 CI run `31404226456` passed build/unit in 1m42s and managed conformance in 4m44s. Artifact audit: four admission probes passed; JUnit 11,611 total, 11,191 passed, 420 skipped, 0 failures/errors.
- Final #155 audit: seven scoped files, two commits, CLEAN/mergeable, literal `Closes #63`, and issue contract fully covered (locked `ReadingPlayer`, atomic callback consumption, deterministic duplicate-reader selection).
- `gh pr merge 155 --merge` produced normal two-parent merge `b94d855aca55990ed1cd46a483a01a539ca1f63b` with parents `5e1153935f9a206ca247683b6649880b1bc0082e` and `241b361c36da096c2c4fd2b914f55c3a224fa360`; issue #63 closed.
- GitHub open PR inventory after the merge is empty. Remote `origin/master` is `b94d855aca55990ed1cd46a483a01a539ca1f63b`.
- Final post-merge master CI run `31404734753` at `b94d855aca55990ed1cd46a483a01a539ca1f63b` passed: build/unit 1m38s and managed conformance 4m51s. A transient GitHub API HTTP 502 interrupted one watch poll only; direct run inspection confirmed the job remained in progress, and the resumed watch completed successfully.
