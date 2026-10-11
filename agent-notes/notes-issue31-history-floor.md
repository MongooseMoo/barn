# Issue 31 history-floor repair checkpoint

## Findings

- `historyFloor` scans the 16 `readTSShards` independently. A reader can complete registration in a shard that the scan already visited, allowing the returned floor to exceed a live reader timestamp.
- Deterministic regression `TestHistoryFloorDoesNotMissCompletedReaderDuringScan` failed 10/10 on `master`: floor 18 was returned after a reader at timestamp 16 completed registration.
- Issue workload regression `TestHistoryGCConcurrentReadersKeepLiveObjects` uses 8 writers, 8 readers, live objects `#1..#3`, and pruning enabled.

## Current state

- Branch: `agent/issue-31-history-floor-snapshot`
- Red commit: `389e325 Test history floor reader registration`
- Working implementation adds a synchronization boundary between read-timestamp sampling/registration and floor scans, while retaining the 16 shard locks.
- Green evidence so far:
  - invariant test: 100 normal runs and 10 race-instrumented runs
  - invariant + workload tests: 10 normal runs
  - focused workload under `-race`: 3 runs
  - `go test ./... -count=1`
  - `go vet ./...`

## Observation / risk

- The current working implementation uses an RW gate with registrations shared and floor scans exclusive. It is correct, but exclusive scans could serialize decentralized commits and partially undo the measured reason the registry was sharded. The prior experiment attributed 88 percent of contention to the old global floor mutex and measured the sharded design at about 1.7x better throughput.

## Blocker

- PR #159 build/unit CI passed, but conformance failed with six cases that are identically failing on current `master` at `5685dca`.
- Last green `master` used moo-conformance-tests `325bf454`; current CI uses `71e2ba2`, which added five shutdown/WAIF fixture and lifecycle cases Barn does not yet pass.
- The sixth failure is the bcrypt cost-vector regression introduced by Barn PR #156. PR #158 repairs bcrypt but still inherits the five shutdown/WAIF failures.
- No open Barn PR currently repairs those five new external-conformance failures. Project guardrails forbid importing them into issue #31's four-file feature PR.
- The user explicitly authorized a separate base-repair campaign.
- PR #158 is confirmed narrow (two bcrypt files, one commit) and its CI isolates exactly the remaining five WAIF/shutdown failures.
- Clean Barn repair worktree: `C:\Users\Q\code\.worktrees\barn-base-waif-shutdown-20260809`, branch `agent/base-waif-shutdown-20260809`, base `5685dca`.
- Clean conformance worktree: `C:\Users\Q\code\.worktrees\moo-conformance-current-20260809`, detached at current `origin/main` `71e2ba2`.
- WSL Toast identity was verified before the oracle run: `/root/src/toaststunt/build-release/moo` at `aecc51e9449c6e7c95272f0f044b5ba38948459e`; current NAT IP is `172.17.144.45`.
- Managed Toast wrapper run selected exactly six cases: five passed; the panic-abort case reached the expected Toast abort but Windows saw WSL relay status `134`, which the harness does not normalize as abort.
- A documented all-inside-WSL managed run from a clean WSL-native checkout at `71e2ba2` removed that transport ambiguity: all six selected cases passed against pinned Toast in 5.22 seconds.
- Unmodified Barn `5685dca` was built in the clean repair worktree. Its corrected forward-slash managed Windows command passed capability admission and failed all five behavioral cases, reproducing the base defect without running any server manually.
- The five Barn failures are three fixture checkpoint mismatches/missing local outputs, panic termination status `1`, and deferred-WAIF shutdown failing to exit within 5000 ms. Linux CI remains authoritative for the exact three checkpoint byte mismatches.
- PR #96 (`da859ad`) already landed the large pending-anonymous/root serialization repair; the new fixtures exercise WAIF roots held by suspended tasks and shutdown invoked reentrantly from a deferred WAIF recycle hook beyond that earlier coverage.
- Historical branch `3a70158` contains a broader unpublished shutdown-boundary design but is not in PR #96 ancestry. A detached managed experiment against its exact head passed admission and only two of the five behaviors; it still failed panic fixture bytes, recycle fixture bytes, panic termination, and deferred-recycle exit. It is not suitable to cherry-pick wholesale.
- Conformance PR #79 explicitly pins four contracts: deferred WAIF recycle returns and exits cleanly with zero pending roots; a direct pending WAIF that contains another WAIF remains one top-level pending root; panic aborts and dumps the suspended task with WAIF and anonymous roots; generated fixtures byte-match pinned Toast output.
- The expected nested and recycle shutdown fixtures each contain exactly `1 values pending finalization`; the panic fixture contains `0 values pending finalization` and a suspended task rather than promoting its live roots into the pending queue.

## Next action

- Run the five current WAIF/shutdown selectors plus `capability_admission` through the documented managed WSL Toast workflow first, using the clean conformance worktree and disposable harness databases.
- If Toast is green, reproduce the same focused set through managed Barn from the clean base-repair worktree, add deterministic Go regression coverage before implementation, and deliver the WAIF repair as its own PR.
- Inspect the current task/runtime shutdown, WAIF finalization, and checkpoint root-capture boundary to reduce the five external failures to deterministic Go regressions. Keep the repair separate from PR #158 and issue #31.
- Add current-package red tests for shutdown handoff, nested pending-WAIF topology, deferred recycle reentrancy, and panic task-root checkpointing. Use the historical branch only as an invariant reference, then implement the smallest current-engine ownership boundary that makes the tests and managed cases green.
- Land the WAIF repair and PR #158 separately only after their own required checks are green. Then merge repaired `origin/master` into #159, rerun both required checks, re-audit, normal-merge with `gh pr merge 159 --merge`, and verify two parents plus issue closure.

## 2026-08-09 TDD implementation checkpoint

- The documented Toast-first and same-selector Barn-red phases are complete. The WSL-native Toast run passed all six selected cases; current Barn passed admission and failed all five WAIF/shutdown behaviors.
- The historical `3a70158` experiment proved that its unpublished shutdown work is incomplete and must not be cherry-picked wholesale.
- Current code still conflates generic runtime cancellation with shutdown publication, synchronously invokes shutdown from the shutdown builtin, exits panic shutdown with status 1, and does not transfer/reduce deferred WAIF roots at a producer-safe boundary.
- The GitHub workflow instructions were loaded. CI logs remain a `gh` responsibility; publishing must use explicit file staging in the clean worktree, and final merging remains subject to the repository's stricter normal-merge audit.
- Next: add deterministic current-package red tests for top-level WAIF root reduction, deferred-recycle shutdown return/publication ordering, suspended-task root ownership on panic, and the platform abort contract. Commit the red tests separately, implement the narrow current-engine boundary, then run focused Go tests, race/full tests, and the exact managed conformance selectors.
- The issue #31 feature branch and its untracked checkpoint note remain outside the clean base-repair worktree. This note must remain untracked and unstaged.

## Exact current conformance assertions

- Fixture provenance runs the three recipe databases to managed process exit, then byte-compares each generated `.new` file against the checked-in Toast fixture.
- The deferred-recycle startup case requires exit code 0 within 5000 ms, both `ORACLE WAIF RECYCLE ENTERED` and `ORACLE WAIF RECYCLE RETURNED`, and `0 values pending finalization` in the new checkpoint.
- The nested-WAIF startup case requires exit code 0, both entered/returned log markers, exactly `1 values pending finalization`, and the serialized parent-to-child WAIF topology.
- The panic startup case requires portable abort termination, the suspended-holder and panic log markers, `0 values pending finalization`, exactly one suspended task, and that task's WAIF class/owner plus anonymous-object/owner roots.
- These assertions reinforce two separate ownership rules: deferred roots become pending only after the producer returns; roots retained by a suspended task remain task-owned and must never be promoted to the pending queue.

## Red commit and root-cause refinement

- Clean base-repair branch red commit: `4fe5739 Test shutdown finalization ownership`.
- Focused red command covered the new VM, checkpoint, and engine tests. It failed because pending WAIF roots were empty, VM root walkers missed task-context and nested-WAIF references, checkpoint reload lost the WAIF's anonymous property, `Runtime.BeginShutdown` was absent, and the deferred recycle lifecycle could not return/publish.
- `Database.NewStoreFromDatabase` installs loaded pending finalizations in `Store`, but neither `Server.LoadDatabase` nor `Runtime` adopts that queue for finalization. The `PendingWaifRecycleShutdown.db` startup case therefore has no runtime owner to invoke its loaded WAIF's `:recycle` hook.
- The current snapshot anonymous-root collector and rewriter traverse lists/maps but not WAIF properties, so a suspended task can keep the WAIF value while losing the anonymous object nested inside it. This directly explains the panic task-root fixture mismatch.
- The historical shutdown branch's deferred-recycle unit expected an already-claimed loaded root not to be requeued, which is correct for the second startup. Its recipe-generation path still lacked a way to retain the producing task's WAIF when shutdown was requested from that WAIF's recycle hook; this is consistent with the historical experiment still failing the recycle fixture bytes.
- Implementation next: complete VM direct-root capture/reduction and nested-WAIF traversal; add store WAIF-aware reachability/rewriting plus a take/adopt pending queue; add an explicit runtime producer/publication boundary; retain task-produced roots when shutdown begins during their recycle but consume loaded pending roots; then wire asynchronous server lifecycle and platform abort termination.

## Implementation checkpoint after red commits

- Second test-only red commit: `b8969e9 Test panic shutdown abort status`. The Windows subprocess test fails at compile time because `terminatePanicShutdown` is intentionally absent; the POSIX counterpart pins direct `SIGABRT`.
- Working-tree implementation now makes the VM direct-root and suspended-task checkpoint tests green: root walkers traverse WAIF properties and complete VM/task-context surfaces, frame pops retain shutdown identities, nested WAIF candidates reduce to one covering parent, and snapshot planning rewrites anonymous references inside WAIF graphs.
- The store now has a take-and-clear pending-finalization transfer and WAIF semantic-identity handling in progress. The runtime has a producer-counted `BeginShutdown` publication boundary, explicit shutdown state distinct from cancellation, deferred-GC ownership, and pending-root adoption in progress.
- Server lifecycle wiring now waits for runtime publication asynchronously when shutdown originates from a task or deferred recycle activation, so MOO code can return before cancellation/checkpointing starts.
- Still unverified: integrated compilation after the runtime changes, startup adoption behavior, recipe-generation retention when shutdown begins inside task-produced WAIF recycle, exact fixture bytes, and platform abort implementation.
- Next: add tagged Windows/POSIX termination helpers, replace the command's panic status-1 exit, run focused compilation/tests, repair any ownership races or fixture mismatches, then rebuild Barn and run the exact managed conformance selectors.

## First fixed-Barn managed run

- Full touched-package unit suites passed after adding the VM pool-reset expectation for `PendingFinalizations`.
- The first Windows managed invocation was rejected before behavior because a Linux Toast manifest was paired with a Windows target; rerunning the documented Windows deployment lane without that cross-OS oracle manifest passed capability admission.
- Exact managed selection result: `3 passed, 3 failed, 11605 deselected`.
- Passed: capability admission, nested-WAIF recipe byte equality, and deferred loaded-WAIF recycle startup/clean exit. This confirms startup pending-root adoption and the one-parent nested topology.
- Failed: panic recipe byte equality remained the original offset 2126 / length 2661 versus 2653; recycle recipe generated `0 values pending finalization` (length 2512 versus 2530); panic startup reached termination but did not emit the literal `PANIC: ORACLE PANIC WITH LIVE ROOTS` log assertion.
- The recycle recipe assigns `WAIF = 0` before calling shutdown. Barn only scanned locals at frame pop, so it lost the WAIF at assignment; Toast reference-count finalization retains it immediately. New test-first commit `c3f89f6 Test overwritten WAIF finalization` fails at `OP_SET_VAR` with no pending roots.
- Next: capture the overwritten local value before `OP_SET_VAR` replaces it, add the exact Toast panic log prefix, rerun focused/unit tests and the managed set, and capture/diff the remaining panic recipe bytes if that mismatch persists.

## Panic path and ordinary-GC checkpoint

- Capturing the overwritten local made the recycle recipe byte-identical to Toast. The next managed run passed admission plus four of the five behavioral cases; only the panic recipe and panic startup remained.
- Passing the raw unclean-shutdown message into `Server.Panic` made the literal `PANIC: ORACLE PANIC WITH LIVE ROOTS` assertion pass. The panic startup then advanced to the next contract and proved Barn wrote the emergency checkpoint to `.new` instead of Toast's required `.new.PANIC` path.
- Test-only red commit `1ec00e8 Test panic checkpoint filename` pins that emergency dumps use `<database>.new.PANIC` and do not create an ordinary `<database>.new` checkpoint. The implementation now has a separate atomic `WritePanicCheckpoint` path and the focused server test is green.
- A full touched-package run then exposed a deterministic ordinary-GC regression: the new historical `PendingFinalizations` ledger was treated as current liveness, so an overwritten anonymous object survived explicit `run_gc()` and its recycle hook did not update the caller's transaction. `TestRunGCSweepRecycleHookRetainsTaskTransaction` failed five of five focused runs.
- Root cause: shutdown-lossless identities and live GC roots are different domains. `PendingFinalizations` (and WAIF candidates in `PendingWaifs`) must be consumed by shutdown publication, not included by ordinary anonymous/WAIF liveness walkers. The working fix removes those candidate ledgers from ordinary liveness while retaining them in direct shutdown-root capture.
- Current blocker: confirm the ordinary-GC regression is green, rerun all touched packages, then rebuild and rerun the exact managed admission plus five WAIF/shutdown cases. The remaining expected conformance investigation is the panic recipe byte mismatch; the panic startup may reveal checkpoint-content assertions after the filename fix.
- The issue #31 feature branch remains unchanged. All base-repair changes are isolated in `C:\Users\Q\code\.worktrees\barn-base-waif-shutdown-20260809`; this note remains untracked and must not be staged or pushed.

## Task-owned panic root checkpoint

- The managed admission plus six behavior slice reached `5 passed, 2 failed`. The only failures were the still-known panic recipe byte mismatch and panic startup checkpoint ownership.
- Panic startup now finds `.new.PANIC` and gets past the literal panic log assertion. Its checkpoint contained `2 values pending finalization` even though the same WAIF and anonymous identities were serialized in the one suspended holder task; Toast requires `0 values pending finalization` there.
- Two additional test-only commits pin checkpoint ownership: `b9b17eb Test task-owned roots are not pending` and `9908e44 Test direct task-owned roots are not pending`. The red checkpoint reload contained both the overlapping WAIF and an independent anonymous root in the pending section.
- The implementation resolves overlap where all ownership information is simultaneously available: `Store.SnapshotWithRoots` now removes pending WAIF or anonymous identities covered by queued/suspended task roots from the snapshot only. It does not mutate the live runtime queue. Nested WAIF traversal is cycle-safe and participates in the ownership comparison.
- The focused checkpoint test and the full `db/store`, `db/format`, `engine`, and `server` suites are green. The earlier explicit-`run_gc()` regression remains fixed after separating historical finalization candidates from ordinary live roots.
- Current next action: rerun managed panic startup against the rebuilt Windows Barn target. If its pending count and suspended-task root assertions pass, capture and structurally diff the sole remaining panic recipe output mismatch using only the managed harness flow and disposable `.tmp` artifacts.

## Remaining suspended-WAIF failure

- After snapshot overlap filtering, managed panic startup now writes `0 values pending finalization`, so task-owned versus pending ownership is correct.
- The next assertion fails because the suspended task's two-element state retains anonymous object `#10` but its first element is no longer a WAIF with class `#9` and owner `#3`. This narrows the defect to premature WAIF finalization or loss in live task state, not checkpoint root promotion or anonymous serialization.
- The ordinary pending-WAIF recycle and nested-WAIF startup/recipe cases remain green. The remaining panic recipe mismatch is still offset 2126 with actual length 2661 versus expected 2653 and is likely the same WAIF lifecycle discrepancy.
- A disposable wrapper under the repair worktree's ignored `.tmp` directory is being used only through the documented managed harness to copy generated checkpoint output before harness cleanup. No Barn/Toast process has been launched manually and no tracked fixture has been modified.
- The first PowerShell and Python capture attempts caused the wrapper itself to return status 1 after the child exited, so they did not preserve output. The wrapper is now guarded so capture errors cannot replace the child's status; next run will either preserve the managed recipe output or print the exact capture exception.

## Managed-debugger recipe evidence

- A direct managed-harness run with pytest post-mortem debugging paused before fixture cleanup and exposed both in-memory byte strings without copying or manually launching the server.
- At mismatch offset 2126, Barn writes `waif = class:new();`, `anon = create(...)`, and preserves two-space indentation inside both forks. Toast writes canonical `WAIF = ...`, `ANON = ...`, and unindented fork bodies. The eight extra Barn bytes are exactly the four two-space indents.
- The recipe intentionally passes lowercase `waif` and `anon` through `set_verb_code`; Toast treats MOO variable identifiers case-insensitively, colliding those names with the canonical built-in type names `WAIF` and `ANON`. Barn's bytecode compiler currently keys its variable map case-sensitively and stores the lowercase spellings.
- This compiler discrepancy explains both remaining failures: the recipe cannot byte-match Toast, and the startup execution interprets uppercase `WAIF`/`ANON` references from the Toast-generated fixture as built-in type constants rather than the lowercase variables assigned earlier, so the suspended task's first state element is not the WAIF instance.
- Next: add a red compiler/runtime test proving mixed-case variable assignment/reference identity and canonical `WAIF`/`ANON` variable slots; normalize compiler variable lookup keys case-insensitively while retaining canonical type-name display names. Separately canonicalize the database-written program text through the established code path needed to remove indentation, without changing the parser package's stored-source ownership rule.

## Six-of-seven managed checkpoint

- Test-only commits `ee0ad3a Test case-insensitive Toast variable output` and `817ee84 Test set_verb_code canonical storage` pin mixed-case execution and the dynamic-code storage boundary.
- Compiler variable lookup is now case-insensitive and canonicalizes type-name variable slots to `WAIF`/`ANON`. Parser canonical formatting uses the same language spellings. `builtinSetVerbCode` stores its newly compiled program in Toast's unindented canonical form.
- An attempted global writer canonicalization was rejected by the exact managed slice because it changed all three previously-correct fixtures. That broad production change was removed, and its incorrect writer test was removed in the next test-only commit. Database-loaded source remains byte-preserved; only newly compiled `set_verb_code` source is canonicalized.
- All affected package suites passed after the narrowed implementation. The exact managed admission plus six behavior run now reports `6 passed, 1 failed`: all three recipe byte comparisons, both ordinary WAIF startup cases, and admission pass.
- Sole remaining failure: panic startup writes the correct `.new.PANIC`, abort status and log pass, pending count is zero, and anonymous root `#10` survives; the suspended holder's first state element is not the expected WAIF class `#9`, owner `#3`.
- Current next action: reproduce the live sibling-WAIF retention boundary in a deterministic engine test. Inspect fork task creation, VM handoff, and deferred WAIF candidate liveness so a WAIF shared with a suspended sibling cannot be recycled when the parent/panic task completes. Then rerun the focused startup selector and the complete seven-case slice.

## Pinned Toast task-environment evidence

- Managed post-mortem inspection of Barn showed the WAIF was not lost: Barn serialized local `WAIF` as `13 / c 0 / class 9 / owner 3`, then serialized `state` as a list containing `13 / r 0`. The conformance parser intentionally inspects `state`, so it reported a reference rather than a WAIF definition.
- The pinned WSL Toast oracle was rerun through the managed harness using `/root/src/toaststunt/build-release/moo` with output `{db}.new`. Its panic checkpoint has 27 environment variables, clears the direct `ANON` and `WAIF` slots to `NONE`, and serializes `state` with the WAIF creation `c 0`, class `9`, owner `3`, plus anonymous `#10`.
- Therefore the remaining failure is alias-definition ownership/order, not premature recycling. Barn retains more dead direct locals than Toast but can preserve semantics by emitting a composite task owner before a redundant direct finalizable alias; the reader restores locals by variable name, so environment entry order is not positional.
- Test-only red commit `f63d51b Test composite task root definition order` pins that a `state` list sharing a WAIF with direct local `WAIF` owns the creation record and is emitted first. Current output defines the direct alias first and writes `state` as `r 0`.
- Next: implement stable environment ordering that moves only direct WAIF/anonymous locals covered by another composite local behind that owner, while preserving every name/value pair and alias identity on reload. Rerun the focused writer round trip, managed panic startup, and exact seven-case slice.

## Published base-repair CI checkpoint

- Stable composite-owner environment ordering completed the repair: the exact managed admission plus six behavior slice passed all seven cases, including all three byte-identical Toast recipes and the panic checkpoint task-root assertions.
- Local verification is green: `go test ./... -count=1`, the affected-package race suite (including the 268-second engine stress run), `go vet ./...`, pinned `staticcheck@v0.7.0`, gofmt, and diff checks.
- Base-repair PR #160 preserves nine red test-only commits (`4fe5739` through `f63d51b`) followed by green implementation commit `f116d5a`. Its published file and commit lists match the audited shutdown/finalization/compiler repair scope and do not include issue #31.
- GitHub CI run `31339526208` has reported a build/unit failure while the managed conformance job is still running. GitHub has not exposed failed-step logs because the overall workflow remains in progress, so the exact CI cause is not yet known.
- Next: wait for run `31339526208` to finish, inspect the failed build job with `gh`, reproduce and fix only the demonstrated branch defect, rerun all required checks, then audit and normal-merge PR #160. PR #158 and issue #31 PR #159 remain separate and unmerged.
- This checkpoint file remains untracked, unstaged, and unpushed.

## CI dependency integration and rollback checkpoint

- PR #160 run `31339526208` had exactly two full-conformance failures: the already-isolated cost-14 bcrypt failure repaired by PR #158, and Linux panic termination reported as status 134. The latter also failed the new POSIX unit contract because Go's SIGABRT handler exits with status 2.
- Commit `e4aeef2 Preserve POSIX abort signal status` replaces the Unix Go process with a POSIX shell abort path. A Linux-cross-compiled `cmd/barn` test binary run under WSL now reports the required SIGABRT wait status; Windows tests, the full Go suite, vet, and pinned staticcheck remain green locally.
- PRs #158 and #160 form a circular base-CI dependency: each standalone merge ref lacks the other current-master repair. A dedicated integration branch is therefore needed so the combined base repair can earn green CI without importing either fix into issue #31.
- During initial integration setup, the new worktree was created correctly but two local merge commands mistakenly ran in the main issue #31 checkout, producing unpushed commits `54f467e` and `70a9e2a`. Per the completion guardrail, work stopped and the exact rollback was performed immediately.
- Rollback verification: `agent/issue-31-history-floor-snapshot` is restored to `d1e84f1`, has no tracked working-tree changes, and this note remains untracked. Neither invalid merge commit was pushed.
- The clean dedicated worktree `C:\Users\Q\code\.worktrees\barn-ci-integration-20260809` is on `agent/restore-master-ci-20260809`. It now correctly contains merge commits `cb85094` (bcrypt repair) and `e8d6183` (shutdown repair), based on `5685dca`; its audited diff is exactly the union of PRs #158 and #160.
- Current state: integration full tests are still completing; vet and pinned staticcheck are green. Next: finish the full suite, publish the dedicated integration PR, watch both required checks, audit and normal-merge it, then update issue #31 from repaired master.
- This checkpoint file must remain untracked, unstaged, and unpushed.

## Final merged state

- Base-CI integration PR #161 passed build/unit plus the complete managed MOO conformance suite and was normal-merged as `c0db35c661e1c5950e66dbf9b7e11d28a5f16b2f`. The merge commit has two parents. GitHub recognizes focused repair PRs #158 and #160 as merged through that preserved history.
- Repaired `origin/master` was merged into issue #31 with `72fb472`. The feature diff remained exactly four files: `db/store/cow_history_gc_test.go`, `store_core.go`, `store_history_gc.go`, and `store_txn.go`.
- Refreshed local verification passed: the new reader-registration regression 100 times, `go test -race ./db/store`, `go test ./... -count=1`, `go vet ./...`, and pinned `staticcheck@v0.7.0`.
- PR #159 retained red commit `389e325`, green commit `9885def`, literal `Closes #31`, and GitHub's sole closing reference to issue #31. Build/unit and the complete managed conformance suite both passed.
- PR #159 was normal-merged as `240be17117f03e24bc85373eb399812d75c81d66`, verified with two parents and as current `origin/master`. Issue #31 is closed with reason `COMPLETED`.
- The earlier accidental local integration commits were never pushed and remain absent from the delivered feature history. This checkpoint file remains untracked, unstaged, and unpushed.
