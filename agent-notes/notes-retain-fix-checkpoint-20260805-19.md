# Retain/fix checkpoint 19

## Current implementation in progress

- Worktree: `C:\Users\Q\code\.worktrees\barn-pr96-normal-merge-update` on remote head `cc436313d892cba6b6e2cb6385849e9a360f0392` plus uncommitted barrier repair.
- Deterministic red: `TestDeferredGCSweepBlocksNewVMStartUntilSweepCompletes` proves a new task starts and creates anonymous `#2` while the old deferred sweep is paused after root capture.
- Current uncommitted scope is five scheduler files: `scheduler.go`, `waif_lifecycle.go`, `eval.go`, `call_verb.go`, and `deferred_gc_lifecycle_test.go`.
- Two-lock protocol is being implemented: `gcSweepMu` serializes sweeps; `vmStartMu` blocks VM starts from before root capture through recycle callbacks; physical lease publication stays refcounted under `s.mu`.
- Scheduler-private context provenance maps distinguish execution-owned and sweep-owned direct VM contexts. Nested calls use the same standalone CallVerbWithArgstr semantics while inheriting ownership instead of reacquiring the start barrier.
- Direct `EvalCommandOutput` and top-level `CallVerbWithArgstr` now acquire physical execution leases; `CallVerbInContext` remains inherited; waif/anonymous recycle callbacks are sweep-owned.
- Eval inline floor GC now holds both barriers and defers waif/anon work if any non-caller lease exists, instead of skipping active siblings and sweeping.

## Design requirements still to verify before commit

- Lock order everywhere must be `gcSweepMu -> vmStartMu -> s.mu -> task.mu`; no `pendingWaifMu` may be held while acquiring the first two.
- No code holding `s.mu` may wait for `vmStartMu`.
- Top-level direct-call cleanup must release context provenance, release the physical lease, then flush; panic recovery and transaction/effect cleanup must finish first.
- Recycle-hook nested verbs must inherit sweep ownership without deadlock; nested `run_gc()` must TryLock `gcSweepMu`, return success/no-op, and not recurse.
- Nested Eval/server-hook calls must inherit physical ownership without changing their transaction/caller/argstr/task-ID semantics.
- Queued forks from sweep callbacks must not inherit synchronous sweep provenance.
- Explicit global, deferred, and inline floor sweeps must all hold the start barrier for complete root-capture-to-recycle spans.

## Next action

- Finish implementation and deterministic coverage for runTask, Eval, CallVerbWithArgstr, inherited nested calls, reentrant run_gc, floor deferral, and sweep-blocked starts; inspect for deadlocks and provenance leaks; run focused tests x20 and race before full gates.
## Checkpoint 20 - PR #96 GC start/sweep barrier audit

- Current worktree: `C:\Users\Q\code\.worktrees\barn-pr96-normal-merge-update`; branch remains uncommitted over remote head `cc436313d892cba6b6e2cb6385849e9a360f0392`.
- Modified implementation/test files are limited to scheduler call-verb, eval, scheduler ownership state, task runtime, waif lifecycle, and two GC lifecycle/root test files.
- The patch now separates complete-sweep serialization (`gcSweepMu`) from VM-start exclusion (`vmStartMu`) and holds the start exclusion across root capture, candidate processing, and recycle callbacks.
- All physical VM entry paths under review publish execution ownership: scheduled `runTask`, direct eval, and server-hook `CallVerbWithArgstr`; nested independent VMs increment the same opaque execution owner, while sweep-owned callback VMs inherit sweep provenance.
- Sweep provenance is checked before the StoreTxn route so recycle hooks do not reacquire the start barrier and deadlock. Reentrant `run_gc()` while another sweep owns `gcSweepMu` is a successful no-op.
- `runTask` retry restoration replaces `t.Context`; the patch now removes the prior context mapping and registers the replacement context on every retry, then removes the current mapping before releasing the physical lease and flushing deferred GC.
- The nested-GC tests now use real `create()` to `initialize()` callbacks whose initialize verb calls `run_gc()`, for both Eval's nil-transaction path and runTask's StoreTxn path.
- Deterministic barrier tests cover scheduled tasks, direct Eval, and direct server hooks starting while a sweep is blocked; the sweep-owned anonymous recycle callback also calls a nested verb and reentrant `run_gc()`.
- Focused tests were reported green x20 by the implementation agent. Exact repeated/race/full gate output is still pending; no commit or push is authorized yet.
- Current review concern: confirm all lock acquisition follows `gcSweepMu -> vmStartMu -> Scheduler.mu`, that no provenance map survives a retry or panic, and that CallVerbInContext's extra same-owner refcount spans the entire separate VM.
- Blocker: PR #96 cannot be accepted or merged into integration PR #115 until focused repetition, scheduler race, full Go/build/vet/gofmt/diff gates, and a fresh independent exact-diff review all pass.
- Next action: finish local gates, independently review the finalized diff, then commit/push only the approved patch and verify GitHub build plus the known legacy conformance failure set before merging PR #96 normally into PR #115.

## Checkpoint 21 - PR #96 audit corrections and gate restart

- The first pre-audit scheduler race run passed (`go test -race ./scheduler -count=1 -timeout=15m`, package duration 244.919s), but is explicitly excluded from acceptance because the audited corrections were not yet present.
- Audit correction 1 restores the existing StoreTxn-and-task dispatch to `CallVerbInContext` before nil-transaction provenance routing. `CallVerbInContext` itself recognizes sweep provenance and avoids reacquiring the VM-start barrier, so shared transaction and caller semantics are preserved without deadlock.
- A new scheduled-task regression creates an orphan anonymous object, calls explicit `run_gc()`, runs its `recycle` hook, and requires the hook's property write to be visible in and committed by the invoking task transaction.
- Audit correction 2 moves direct `CallVerbWithArgstr` ownership publication to immediately after lightweight-task construction, before tracing, verb lookup, compilation, anonymous-target inspection, or argument use. This makes anonymous/waif arguments covered by a fail-closed active lease before a VM frame exists.
- Focused tests are running on the corrected diff; a fresh scheduler race run will follow and is the only race result that counts.
- Observation: an older managed-conformance Barn process remains live from `2026-08-05 22:58` with command `barn.exe -db C:/Users/Q/AppData/Local/Temp/moo_conformance_m7niw151/Test.db -port 63506`; its parent PID is no longer shown among the active Go gate processes. Do not terminate it without confirming ownership and the managed harness state.
- Current active gate processes are the corrected `go test -race ./scheduler -count=1 -timeout=15m` invocation started at `2026-08-06 04:50` (shim and real Go process).
- Blocker: no commit or push until the corrected focused/race/full gates pass and a fresh independent agent approves the exact uncommitted diff.
- Next action: let the corrected race finish, obtain exact focused and gate results, investigate whether the old managed-harness process is safely owned/stale without destructive action, then dispatch the final independent review.

## Checkpoint 22 - Refcounted provenance green; Toast oracle pending

- The scalar context-ownership entry was rejected because duplicate same-context executions could erase each other's claim, and the first ambiguity repair could delete a real claim on a mismatched release.
- Current implementation uses `map[*TaskContext]map[ownerID]count`: unknown-owner releases are no-ops, same-owner duplicates survive until the last release, a single owner is attributable, and multiple owners remain explicitly claimed but ambiguous.
- Explicit `run_gc()` returns success without sweeping when context ownership is claimed but ambiguous. Nested execution under ambiguity takes the reserved internal owner `-1<<63` as a physical blocker so owner-count changes during the nested call cannot expose a falsely excludable sole caller.
- Focused provenance and GC tests passed 20 consecutive runs on this model. A scheduler race run began just before the hold instruction and is provisional only.
- Independent review found a separate batching hazard in `vm/anonymous_gc.go`: persistent reachability is captured once, but per-request candidate lists are recomputed after earlier recycle callbacks. A callback-created and persistently stashed anonymous object could be selected by a later request against the stale reachability snapshot.
- Barn candidate-freezing work is paused because this observable GC behavior is uncertain and project rules require Toast verification first.
- A separate conformance worker is adding a default-profile case in `C:\Users\Q\code\moo-conformance-tests` for a recycle hook that creates and persistently stores a new anonymous object; only the documented managed pinned WSL Toast oracle may establish expected behavior. No manual Barn or Toast launch is authorized.
- The old PID 161984 was identified as a stale orphaned managed-conformance Barn server from an earlier process, not from the current worker; it listens on 63506 against a disposable temp DB and does not affect in-process scheduler gates. It remains untouched.
- Blocker: final PR #96 diff and acceptance gates cannot stabilize until the Toast oracle result determines whether candidate batches must be frozen before callbacks.
- Next action: obtain exact Toast result and conformance diff; if Toast preserves the new stashed anon, freeze every batch request's candidate slice before any callback and add a Barn unit regression, then rerun focused/race/full gates and exact final independent review.

## Checkpoint 23 - Isolated conformance oracle setup

- The first oracle attempt was invalid and rejected: a WSL-side `uvx --from` invocation selected only one cached capability-admission row and did not collect the new GC test.
- The conformance worker restored its root-checkout edit exactly; `src/moo_conformance/_tests/builtins/gc.yaml` is clean again in the dirty root checkout, preserving all unrelated untracked material.
- Clean isolated worktree: `C:\Users\Q\code\.worktrees\moo-conformance-gc-recycle-hook`, branch `agent/gc-recycle-hook-created-anon-conformance`, based on suite merge head `49f08f5b8b025109ab9885ea90f60756b0faae16`.
- Isolated diff is exactly one tracked file and 41 inserted lines, with no deletions and clean `git diff --check`.
- The case creates an ordinary class with `recycle`, creates a self-cyclic anonymous orphan, and requires exactly one recycle callback plus an anonymous, valid value created by that callback and stored in the class's persistent `stash` property.
- Required valid oracle path: fresh current-source Windows conformance harness plus `C:\Users\Q\code\barn\scripts\run_toast_wsl.sh`, targeting `/root/src/toaststunt/build-release/moo`; collection must prove the exact named test is selected before its result counts.
- Independent Barn reviewer agrees behavior is uncertain and leaves CODE BLOCKED until Toast adjudicates it; no candidate-freeze implementation has been made.
- Provisional post-provenance scheduler race passed (`go test -race ./scheduler -count=1 -timeout=15m`, 251.239s), but final acceptance remains pending because Toast may authorize additional vm changes.
- Blocker: fresh local conformance source invocation has not yet produced an exact named Toast result.
- Next action: finish exact-source collection and managed Toast run; then either implement and test frozen batch candidates if Toast says the callback-created root survives, or retain current batch behavior and document the contrary oracle result.

## Checkpoint 24 - Toast contract established; final Barn patch gating

- Fresh exact-source managed Toast verification is authoritative: 44 rows collected, exactly admission plus `builtins/gc.yaml::gc_recycle_hook_created_anonymous_survives_current_sweep` selected, both passed, 42 deselected.
- Conformance worktree remains isolated at `C:\Users\Q\code\.worktrees\moo-conformance-gc-recycle-hook`; sole diff is `src/moo_conformance/_tests/builtins/gc.yaml`, 54 insertions and 0 deletions, with clean diff check.
- Independent test review caught that cleanup had no `return`, causing runner expression-wrapping to make later cleanup statements unreachable. An explicit final `return 0;` now makes the whole cleanup block execute; exact managed Toast rerun remains `2 passed, 42 deselected`. Final independent re-review is pending.
- Toast's passing contract authorizes Barn to preserve a callback-created, persistently rooted anonymous object through the current sweep.
- Barn now freezes every `RecycleOrphanAnonymousBatch` request's candidate slice against the one reachability snapshot before invoking any recycle callback. A deterministic two-request VM regression creates and persists a new anon in the first callback and requires only the two pre-snapshot candidates to be recycled.
- Scheduler ownership uses per-context per-owner refcounts, ambiguous explicit GC no-op, and a reserved nested blocker. A real CallVerbInContext regression releases the second owner during the nested VM and proves nested `run_gc()` cannot sweep before blocker cleanup.
- A forced MVCC conflict test observes two attempts, distinct context pointers, old-context claim removal during retry, replacement attribution, and no claim leaks after runTask returns.
- Barrier tests now wait for a scheduler-private observer fired immediately before actual `vmStartMu` acquisition; their no-entry assertions use nonblocking channel checks rather than sleep-based absence.
- Final Barn diff currently spans ten intended files: seven scheduler implementation/test files, one new scheduler provenance test, and VM anonymous GC implementation/test files. `git diff --check` passes; focused set passed 20 consecutive runs.
- Final `go test -race ./scheduler -count=1 -timeout=15m` is running; full Go/build/vet/gofmt/diff gates follow. No commit or push.
- Blocker: final acceptance gates and fresh exact-diff independent Barn review must pass; conformance test must receive final independent TEST APPROVE before commit/PR.
- Next action: finish gates, obtain both independent approvals, then create narrowly audited commits/PRs with correct closing scope, wait for all protected checks, and normal-merge only after explicit files/commits/checks/merge-command audit.

## Checkpoint 25 - Conformance merged; Barn O(M*N) memory blocker

- Conformance PR `MongooseMoo/moo-conformance-tests#76` contained exactly commit `5d9154e84667319b6deca0db5152310d399fcc79` and one file (`gc.yaml`, +54/-0), with independent TEST APPROVE and every protected check green.
- Exact normal merge command used: `gh pr merge 76 --repo MongooseMoo/moo-conformance-tests --merge`.
- Merge commit `21add5a254843de871d1daf8ec929b26f5e3e55f` is verified on `origin/main` with exactly two parents: prior main `49f08f5b8b025109ab9885ea90f60756b0faae16` and feature `5d9154e84667319b6deca0db5152310d399fcc79`.
- Barn final local gates before independent review were green: focused new set x20; scheduler race 258.468s; VM candidate race x20; full `go test ./...`; build; scheduler vet; touched-file gofmt; diff check. VM vet reports only the pre-existing untouched `vm/stack.go` ReadByte signature warning tracked for PR #99.
- Final exact-diff reviewer found no remaining semantic, race, deadlock, ownership, retry, or cleanup blocker, but CODE BLOCKED the candidate freeze's memory shape.
- Current `candidateBatches` stores a complete `AnonymousRecycleCandidates` slice per request. For M deferred requests and N shared candidates this retains O(M*N) IDs and can OOM after a throttled busy period; old behavior was O(M*N) time but O(N) memory.
- Required repair: call `AnonymousRecycleCandidates(reachable, minFloor)` once, keep requests outermost, filter frozen IDs by each request's MinID, and preserve recycled de-dup plus request/context order. Candidate memory must remain O(N).
- Required proof: scale/allocation-shaped routing coverage that would reject per-request candidate materialization without brittle wall-clock timing, followed by VM focused/race and full gates.
- No Barn commit or push exists; branch and remote remain at `cc436313d892cba6b6e2cb6385849e9a360f0392`.
- Blocker: O(M*N) retained candidate memory repair and fresh exact-diff approval.
- Next action: implement one frozen list and scale proof, rerun gates, obtain re-review, then commit/push the ten intended files to PR #96.

## Checkpoint 26 - PR #96 exact head pushed and CI running

- O(M*N) memory blocker is repaired: `RecycleOrphanAnonymousBatch` obtains exactly one frozen candidate slice at the minimum request floor, then a factored helper filters by per-request floor in request order and globally de-duplicates candidates. Retained candidate IDs are O(N).
- Allocation regression prebuilds 128 requests and 64 candidates, then requires the routing helper to stay at no more than 8 allocations across `testing.AllocsPerRun`; a per-request slice implementation would add roughly 128 allocations. Behavior and allocation tests passed 20 runs.
- Final independent re-review verdict is CODE APPROVE; no remaining correctness, race, deadlock, ownership, retry, cleanup, semantic-order, or memory blocker was found.
- Final affected gates: VM focused x20 green, VM race x20 green, full Go tests green, build green, scheduler vet green, touched gofmt and diff check green. The scheduler race remains green from the final scheduler-stable patch at 258.468s. VM vet still has only the known untouched ReadByte warning.
- Exact ten intended Barn files were staged explicitly; no notes/unrelated material were staged. Commit `5e1b25802768956e713c618fafc937e75a92d01a` (`fix: serialize GC sweeps with VM execution`) has parent `cc436313d892cba6b6e2cb6385849e9a360f0392`.
- The commit was pushed exactly to `origin/agent/issue-95-pending-finalizations`; PR #96 now reports head `5e1b25802768956e713c618fafc937e75a92d01a`.
- PR #96 body was rewritten to accurately describe checkpoint serialization, physical leases, two-lock sweep/start ownership, nested provenance, retry transfer, O(N) frozen candidates, exact local evidence, merged conformance PR #76, and current GitHub CI as pending. Literal `Closes #95` remains.
- GitHub Build & unit tests and MOO conformance suite are running on exact head 5e1b258. PR remains draft and is not authorized for direct GitHub merge; after validation its history will be normal-merged into integration PR #115.
- Blocker: exact-head GitHub result and subsequent integration update/review.
- Next action: wait for CI, update PR #96 body with exact GitHub counts, then normally merge the final PR #96 branch into PR #115's isolated worktree, run corrected default-profile conformance, independently audit integration, and normal-merge PR #115 only after all checks are green.

## Checkpoint 27 - Merged conformance case exposes Barn transaction-visibility gap

- PR #96 exact head remains `5e1b25802768956e713c618fafc937e75a92d01a`. Build and unit tests passed; the legacy conformance job failed with `83 failed, 11108 passed, 396 skipped`.
- Exact failure breakdown is 71 known `ONLY_32_BITS` capability-admission failures, 8 known bcrypt failures, 3 pre-existing cyclic-chain failures, plus the newly merged Toast-backed `gc_recycle_hook_created_anonymous_survives_current_sweep` case.
- The new case expects the orphan recycle hook count to be 1 after `run_gc();` and a following `suspend(0);`; Barn reports 0. PR #96 remains draft and is not eligible for integration or merge.
- The case clears the orphan's last persistent root and calls `run_gc()` in the same evaluation. Current source shows `run_gc()` performs an inline live-store reachability sweep while the clearing property write is still staged in the caller's `StoreTxn`; the committed store can therefore still appear to root the orphan. The task then commits the clear, but no new anonymous object was created in that task, so the eval-end floor sweep is not scheduled. This is the leading transaction-visibility hypothesis, pending two independent source investigations.
- Existing coverage proves a recycle hook invoked by inline `run_gc()` shares the task transaction, but does not cover a root removal staged immediately before `run_gc()`. The correct boundary must preserve same-transaction recycle semantics where possible while ensuring reachability observes staged removals or collection is safely deferred until commit.
- The three cyclic-chain failures may share the same staged-root/sweep interaction but this is not yet proven.
- Blocker: independently prove the root cause and smallest correct lifecycle repair, then add a focused red regression before implementation.
- Next action: collect both investigator reports, assign the repair in the isolated PR #96 worktree, run focused/race/full gates and the exact managed Barn conformance case, obtain fresh independent review, and only then update PR #115.

## Checkpoint 28 - Root cause proven; first transaction projection blocked

- Managed Barn exact reproduction on PR #96 head `5e1b258` selected capability admission plus `gc_recycle_hook_created_anonymous_survives_current_sweep`: admission passed and the case failed with expected hook count 1, actual 0.
- Proven command path: the managed Test.db room has an executable `eval` verb, so `; code` runs through `ExecuteVerbTaskSync` / `runTask` with a `StoreTxn`; intrinsic `EvalCommandOutput` is only a fallback and was the reason the first isolated Go reproduction incorrectly passed.
- Root cause: the task stages `class.subject = 0`, but `run_gc` discovers persistent roots through the live store, which still contains the old subject->orphan edge. The clear commits only after `run_gc`; no later floor-GC request revisits the old orphan. Cyclic lengths 3/5/6 share the staged anonymous-edge/live-discovery split.
- A four-file transaction-projection patch made capability admission, the new lifecycle case, and cyclic lengths 3/5/6 all pass under the managed harness (`5 passed, 11582 deselected`). Full Go tests, build, focused race, vet, gofmt, and diff checks were green.
- Independent final review CODE BLOCKED that patch: if T1 stages root A->0, T2 commits the same property as B, and T1 then projects current-live plus its staged clear, T1 can destroy B immediately and only later fail transaction validation. The late-root test used a different object and missed this same-key overwrite race.
- The projection patch remains uncommitted and must be replaced, not committed.
- Candidate recovery under adversarial review: make `run_gc` an explicit validated coarse boundary. After sweep/start locks and sole-caller proof, validate and publish all caller writes atomically, mark the context live-mutated only on success, perform live-store GC while sibling VMs remain excluded, then publish recycle-hook writes before releasing the barriers. Add a same-key conflict regression proving the newly rooted B is never recycled.
- Blocker: reviewer approval of the validated-boundary design, implementation, repeat of all local and managed gates, and fresh final code review.
- Next action: obtain the design verdict, replace the projection, prove red/green for same-key conflict plus the four managed cases, then audit/commit/push only if all checks and independent review approve.

## Checkpoint 29 - Projection removed; validated transaction-boundary recovery implemented

- The same-key destructive race is now a deterministic RED regression: T1 stages subject A->0; a nested independent T2 transaction commits subject B; T1's old projection returned E_INVARG but had already recycled both A and B. This exactly proves the independent review blocker.
- Recovery design received conditional SOUND approval: `run_gc` is an explicit coarse transaction boundary. After sweep/start locks and sole-caller proof, all staged writes go through the ordinary validated commit path before any candidate scan. A conflict leaves the transaction and writes intact, performs no recycle, and permits the normal scheduler retry behavior.
- After successful commit (or an empty read-only prefix), the old `StoreTxn` is released and replaced with a current-clock transaction. Escalation-gate exemption is preserved on the replacement, preventing self-deadlock during escalated attempts and avoiding stale numbered/anonymous caches.
- The live-store sweep then runs while sibling VM starts remain excluded. Recycle-hook writes are validated/committed and the transaction renewed a second time before releasing the sweep/start barriers.
- `LiveStoreMutated` is set only after a prefix actually publishes writes or existing anonymous recycle logic performs a live mutation. A no-write/no-candidate run_gc only renews its transaction and does not poison retryability.
- The rejected `StoreTxn.AnonymousRecycleCandidates` projection and its store reachability tests were removed; `vm/anonymous_gc.go` is restored with no semantic diff.
- Current uncommitted scope is `db/store/store_txn.go`, `db/store/store_txn_test.go`, `scheduler/scheduler.go`, and new `scheduler/eval_gc_lifecycle_test.go`. Added StoreTxn renew success/conflict/gate-exemption tests are being finalized.
- Focused scheduler lifecycle/conflict tests are green on the replacement. No commit or push.
- Blocker: finish focused/full/race/build/vet/format gates, rerun managed admission + lifecycle + cyclic 3/5/6, and obtain fresh independent CODE APPROVE on the exact final diff.
- Next action: complete gates and managed proof, audit the four intended files, commit/push a narrow PR #96 repair, update the PR body/checks, then resume integration PR #115 only after exact-head CI is green.

## Checkpoint 30 - PR #96 finalized; exact GitHub residuals audited

- Final recovery commit `b7d2a9acccea41be8da3086d3ade99739d1e62cb` (`fix: make run_gc a validated store boundary`) has parent `5e1b25802768956e713c618fafc937e75a92d01a` and exactly four intended files: `db/store/store_txn.go`, `db/store/store_txn_test.go`, `scheduler/scheduler.go`, and new `scheduler/eval_gc_lifecycle_test.go`.
- Exact final local gates are green: focused transaction/lifecycle/conflict/edge tests, focused race, full affected packages, full `go test ./... -count=1 -timeout=15m`, `go build ./...`, scoped vet, touched gofmt, and diff check.
- Final managed Barn selection is green: capability admission, `gc_recycle_hook_created_anonymous_survives_current_sweep`, and cyclic lengths 3/5/6 all passed (`5 passed, 11582 deselected`).
- Independent final exact-diff verdict is CODE APPROVE; no remaining correctness, race, deadlock, retry, lifecycle, or scope blocker.
- Commit was pushed to `origin/agent/issue-95-pending-finalizations`; PR #96 exact head is `b7d2a9a`, remains draft, and retains literal `Closes #95`.
- GitHub Build & unit tests passed. Legacy conformance reports `79 failed, 11112 passed, 396 skipped`; downloaded JUnit audit proves exactly 71 known `ONLY_32_BITS` capability-admission failures and 8 known bcrypt failures, with zero other failures. The new lifecycle case and cyclic GC 3/5/6 are all repaired.
- PR #96 body now records the exact implementation, root cause, verification, and residual CI breakdown. It will not be merged directly; its history is to be normal-merged into PR #115.
- Integration PR #115 worktree is clean at `f00082ea1a2f4497fcf296a516b756ee5440a796`; remote source #96 head is fetched at `b7d2a9a`.
- Blocker: normal-merge finalized #96 history into PR #115, rerun corrected exact-head CI/default conformance, independently audit integration, and merge only if all required checks are green.
- Next action: create the normal merge commit in the isolated PR #115 worktree, run local gates, push/update PR #115, wait for corrected GitHub checks, then perform the full pre-merge files/commits/issues/closing-phrases/exact-command audit.

## Checkpoint 31 - Integration PR #115 green and normal-merged

- Finalized #96 was normal-merged into integration with local merge commit `da859adf0ed05016b53ddb24dd97379fa2e2a10a`, verified with exactly two parents: prior integration `f00082ea1a2f4497fcf296a516b756ee5440a796` and source `b7d2a9acccea41be8da3086d3ade99739d1e62cb`.
- The sole merge conflict in `vm/anonymous_gc_test.go` retained all three compatible regressions. Independent verdicts: MERGE RESOLUTION APPROVE and VM IMPLEMENTATION APPROVE. Full merged Go tests, build, scoped vet, gofmt, and diff check passed.
- PR #115 exact-head GitHub checks both passed. Downloaded corrected-gate JUnit: `11587` total, `11177 passed`, `410 declared skipped`, `0 failures`, `0 errors`; admission evidence artifact exists.
- Literal pre-merge audit: local/remote/PR head all `da859ad`; mergeable CLEAN; 31 PR files exactly match `origin/master...HEAD`; 22 commits preserve all source histories; required checks both SUCCESS; issues #72/#95/#107/#111/#113 were open; all five exact `Closes #N` phrases present.
- PR body was corrected to remove a literal newline escape artifact before merge.
- Exact authorized normal merge command used: `gh pr merge 115 --repo MongooseMoo/barn --merge`.
- GitHub reports PR #115 MERGED at `2026-08-06T13:39:31Z`, merge commit `95b5758cbf964f7b8a2fc2443c99090a4c32176d`.
- Required post-merge verification is still pending: fetch origin/master, prove merge commit has exactly two parents, verify all five issues closed, then close superseded source PRs with accurate comments.
- Next action: perform post-merge ancestry/issue checks, update source PR dispositions, then continue the retained cleanup queue (#110 authority, #99 Go hygiene, #94 eval framing) against the new master.
