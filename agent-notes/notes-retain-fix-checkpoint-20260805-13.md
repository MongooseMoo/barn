# Retain/fix checkpoint

- Conformance PR 70 merged normally; issue 69 closed; merge `3cc62c8` verified with parents `a71127d` and `b90dd1a`.
- PR109 code independently approved at exact head `3153fec`; merge still blocked only by red base workflow.
- PR96 global cross-VM root normalization pushed at `8bd2630`; focused 7/7, full Go/build/race/scoped vet green; independent re-review pending.
- Draft Barn integration PR 115 opened from `agent/restore-default-conformance` after normal-merging source branches #112, #109, #114, #96.
- PR115 preserves source commit histories and has literal closings for #72, #95, #107, #111, #113. It intentionally excludes unrelated delete-authority PR110.
- Combined local gates on PR115: full Go tests/build green, focused race across bcrypt/store/VM/server/scheduler green, scoped vet green, diff clean.
- PR115 must remain draft until corrected full default-profile CI is green and PR114/PR96 exact-head reviews approve.
- Next: watch PR115 CI; finish PR114 and PR96 reviews; update integration if needed; audit and normal-merge PR115 only after green, verify two parents/issues, then update/rerun PR110 separately.
## 2026-08-05 PR #96 final-review blocker

- Exact reviewed head: `8bd26306dc8f0860a12e1bb9722139998ea6924d`.
- Store-level pending-root normalization is sound and race-safe.
- Production shutdown still checkpoints before `scheduler.Stop()` cancels and joins sibling tasks. Those tasks publish pending roots only after cancellation, so their roots miss the persisted final checkpoint.
- The current server regression directly appends roots and snapshots; it does not exercise the wired shutdown lifecycle or persisted output database.
- PR #115 remains draft and must not merge while this blocker exists.
- Next action: add a red persisted-shutdown lifecycle regression, repair shutdown ordering so all task roots are collected before the final snapshot, rerun focused/full/race/build and managed default conformance, then obtain an exact-head independent review.

## 2026-08-05 active repair state

- PR #115 remains draft at `f00082ea1a2f4497fcf296a516b756ee5440a796`; its prior build and conformance checks are green but do not cover the cross-VM boundary.
- Existing `TestShutdownFinalCheckpointRunsHooksBeforeSchedulerStops` confirms checkpoint hooks require a usable scheduler, so blindly moving `scheduler.Stop()` before checkpoint would break an established lifecycle contract.
- Barn worker owns a real persisted shutdown regression and minimal lifecycle repair on PR #96; no changes were present yet at the last inspection.
- A separate moo-conformance-tests worker owns a managed WSL Toast-proven cross-VM shutdown persistence case on branch `agent/cross-vm-shutdown-conformance`, based on suite master merge `3cc62c8`.
- No merge or ready-state change is authorized until the Barn repair, managed default conformance, and exact-head independent reviews are green.

## 2026-08-05 lifecycle repair refinement

- The first quiescent-snapshot approach was replaced with `Scheduler.PrepareFinalCheckpoint`.
- Production sequence under test: stop input dispatch; mark scheduler shutdown-prepared; cancel scheduler context; join all worker goroutines so running VMs publish shutdown pending roots; install a fresh context only for synchronous checkpoint lifecycle hooks; keep `ProcessReadyTasks` disabled; write final checkpoint; call normal `Stop` to cancel the hook context.
- The real regression now starts two scheduler VMs holding opposite members of one anonymous cycle, resumes them into running infinite loops, invokes real server shutdown, reloads `<db>.new`, and requires one normalized pending root plus both anonymous objects.
- This avoids the false premise that suspended task snapshots themselves prove pending-finalization capture.
- Existing checkpoint hook ordering still needs focused/full/race proof and exact-head review.
- The provisional interactive conformance case was rejected because explicit `dump_database()` serialized suspended tasks before shutdown. The suite worker is replacing it with an early-exit startup fixture that invokes graceful shutdown and directly asserts `1 values pending finalization` in the output database.

## 2026-08-05 oracle and production-fidelity correction

- The first early-exit Anon7 fixture passed both Toast and current unfixed PR #115. It was invalid as a differentiator because the shutdown caller created/passed the cycle and retained references in its own VM; Barn's existing caller-only capture masked the sibling gap.
- The suite fixture is being redesigned so the input database initially owns the A/B cycle. Two no-argument holder tasks each load and delete only one persistent property, then suspend. The shutdown caller never loads either anonymous value. Current PR #115 must be red for the case to qualify.
- The cancel/join scheduler design was rejected before commit because worker cancellation could race an undispatched `taskWork` send and because it did not capture already suspended VMs.
- Current Barn design uses the real `InputProcessor.Start()` ticker to drive sibling tasks to suspension, then real shutdown calls `input.Stop()` and a scheduler-locked capture of stable live VM roots. The capture fails closed on a running task and normalizes through the existing store sink; checkpoint hooks remain unchanged and usable.
- At the last diff inspection, scheduler capture and real lifecycle tests were uncommitted; the server call wiring still needed confirmation before commit.

## 2026-08-05 definitive Toast adjudication

- Caller-free Anon7 pinned managed Toast result: `0 values pending finalization`, `2 suspended tasks`, and both A/B payloads serialized.
- Current PR #115 at `f00082e` produces the same result. Suspended VM roots stay owned by serialized task state; they are not converted into pending-finalization roots at shutdown.
- Therefore the proposed Barn scheduler capture was semantically wrong and would duplicate roots/diverge from Toast. All experimental Barn edits were reverted; PR #96 worktree is clean at exact committed head `8bd26306dc8f0860a12e1bb9722139998ea6924d`; nothing was committed or pushed.
- The prior independent #96 lifecycle blocker was based on an incorrect model and is being re-adjudicated against the direct oracle evidence. Review must now focus on the committed caller-VM capture and store normalization only.
- The conformance worker is retaining Anon7 with the actual 0-pending/2-suspended contract because it prevents future implementations and agents from making this same wrong repair. It must verify both Toast and Barn and remove generator/temp artifacts before its draft PR.

## 2026-08-05 task-snapshot graph gap and suite PR split

- Strengthened Anon7 proves the actual Barn red: Barn writes exact `0 values pending finalization` and `2 suspended tasks`, but omits the anonymous A/B object payload reachable only from the suspended task VM snapshots. Toast emits both payloads.
- Correct Barn repair: feed every serialized queued/suspended task value surface into the same anonymous reachability and identity-to-serialization rewrite plan. Do not reclassify task roots as pending and do not change shutdown ordering.
- Independent exact-head PR #96 review is BLOCK at `8bd2630` only for this task-owned anonymous graph omission. Store normalization remains sound/reachable for multiple direct shutdown callers; PR body must narrow that claim and disclose Anon7.
- Conformance issue #71 and draft PR #72 exist at `ac02fbfe3fba6115e2ec90809f1a9f63fd17346d`, four files, literal `Closes #71`. Pinned Toast passes 2/2; current Barn fails the A payload assertion; 471 unit tests and 11,586 collection pass locally.
- PR #72 CI failed because the trusted change-boundary classifier rejects mixing controller `tests/test_startup_fixtures.py` with fixture data. Exact log: land controller changes first, then dependent data.
- Recovery plan: controller-only PR makes the repository fixture inventory test data-independent while retaining manifest/file exactness; normal-merge it after green; then replace PR #72 with a clean three-file data-only PR from updated main and close #72 as superseded. No force-push or mixed history.

## 2026-08-05 implementation and controller progress

- Barn worker established a real `WriteCheckpoint` red: headers retain 0 pending and 2 suspended, but reload has zero anonymous objects instead of the two linked task-owned objects.
- Chosen ownership: `task.Snapshot` owns a reusable transform over exactly persisted task values; `db/store` owns the anonymous graph plan plus immutable rewriter; `db/format.WriteCheckpoint` coordinates task-root collection, store snapshot planning, and task-value rewriting. Its input changes from a prebuilt store snapshot to `*store.Store`, making the combined plan unavoidable.
- Persisted-value traversal is section-aware: ordinary suspended tasks include wake/task-local/VM values; interrupted tasks exclude wake because writer does not emit it; queued forks include serialized fork variables and activation this. This avoids retaining graphs from non-persisted memory.
- Barn uncommitted diff currently spans checkpoint/store/task/server plus tests; shutdown ordering and pending classification remain untouched.
- Suite controller issue #73 and draft PR #74 exist at exact head `0538acfa1c313377d485cf8f98fe9cc67b1f3fee`. One commit/one file, literal close, local 471/harness/duplicate/Ruff/mypy gates green. Test compares manifest evidence count/names to sorted actual `*.db` inventory instead of hardcoding fixture names.
- PR #74 trusted boundary/controller/quality jobs are green; three protected Toast matrix jobs remain pending before audit/normal merge.

## 2026-08-05 controller merge and explicit-GC prerequisite

- PR #74 received independent CODE APPROVE at exact head `0538acfa1c313377d485cf8f98fe9cc67b1f3fee`; all protected checks passed.
- Final audit found one commit, one file, CLEAN/mergeable, literal `Closes #73`, issue open, and no scope mismatch.
- Normal merge command used: `gh pr merge 74 --repo MongooseMoo/moo-conformance-tests --merge`.
- Merge commit `4413c85dbef80d20c2aaf6af8ec153ddec3cccee` verified with parents `3cc62c8...` and `0538acf...`; issue #73 verified CLOSED.
- Conformance worker is creating a clean three-file Anon7 data PR from the new main and will close mixed draft PR #72 as superseded without force-pushing.
- Managed Anon7 exposed a second Barn prerequisite: explicit `run_gc()` recycled A/B before checkpoint because the registry hook supplied no sibling VM roots. This caused task locals to rewrite to anonymous `#-1`; the checkpoint planner cannot emit already recycled objects.
- Approved narrow repair: explicit run_gc uses existing scheduler `collectSiblingGCRefs(current task)` and `AutoRecycleOrphanAnonymousSince(..., 0, siblingRefs)`, preserving caller VM roots and suspended sibling roots. Separate regression proves suspended sibling cycle survives explicit run_gc. No shutdown or pending classification change.
## 2026-08-06 final PR #96 exact-head review

- Scope: read-only review of live Barn PR #96 at exact head `cc436313d892cba6b6e2cb6385849e9a360f0392`; no historical Markdown or plans.
- Current finding: review not yet performed; latest requested scope is the three-file lifecycle delta only, while reusing prior approval of serialization and physical-lease work.
- Required checks: deferred GC after lease release for all `runTask` exits, defer ordering around recovery/transaction/escalation cleanup, caller coverage, recursion/double-flush safety, regression strength, PR body/`Closes #95`, and current CI failure accounting.
- Blocker: none known.
- Next action: inspect applicable skill instructions, fetch the live PR metadata/diff/check logs, inspect current files/tests, then return CODE APPROVE or CODE BLOCK with exact evidence.

### Final exact-head finding

- Reviewed live PR #96 at `cc436313d892cba6b6e2cb6385849e9a360f0392`; latest commit is exactly three files: `scheduler/task_runtime.go`, `scheduler/waif_lifecycle.go`, and `scheduler/deferred_gc_lifecycle_test.go`.
- CODE BLOCK: the lifecycle defer releases the task execution lease and then calls `flushDeferredGC`, but `flushDeferredGC` only checks `executingTasks` while briefly holding `s.mu`; it releases that mutex before draining and sweeping. Another direct or worker `runTask` can acquire a lease after that point and create or acquire VM-only anonymous/waif roots while the sweep proceeds from the stale quiescent snapshot. The two new regressions are single-threaded normal and recovered-`OnComplete`-panic cases and do not cover this start-during-flush interval.
- Defer ordering itself is correct for existing task cleanup: `cancel`, escalation unlock, and transaction release run before panic recovery, and the outer lifecycle defer releases the physical lease and flushes last on normal, suspend, early-return, and recovered-panic paths.
- Batch draining is mutex-protected, so simultaneous flush callers do not process the same drained batch twice; the blocker is loss of quiescence across the sweep, not duplicate draining.
- Live PR body contains literal `Closes #95` and accurately reports the current CI result.
- Exact-head CI: build/unit green; conformance finished `82 failed, 11108 passed, 396 skipped, 1 warning`. This restores exactly the prior four waif lifecycle failures; remaining known breakdown is 71 `ONLY_32_BITS` admission failures, 8 bcrypt failures, and 3 cyclic-GC failures.
- Next action: keep a scheduler-wide GC/quiescence exclusion across root capture, batch drain, and sweep (or make task acquisition participate in an equivalent barrier), add a deterministic regression where another `runTask` attempts to start during the flush, then re-review the new exact head.
