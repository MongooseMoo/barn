# Checkpoint 2026-09-18: transaction scheduling research
- Two plugin research reports complete: STM irrevocability and cooperative MVCC implementations; full core-paper limitations checked.
- Design and bounded gate model written; model explored 2126 states / 6108 transitions, no runtime implementation claimed.
- Fable review session df8fcba2-e6b1-46de-be94-e07943e7b026 still finalizing; draft flags principal attribution/caps and maintenance admission.
- Next: resolve final review with source-backed contracts, document checks, stage only intentional reports/design/model and commit on current branch.

## Final checkpoint 2026-09-18
- Committed research, design, Fable review and model as a3d6616 on fix/mongoose-workload; runtime implementation remains future work.
- Final choice: dispatch/wakeups first, explicit gate ownership second, shared admission third; retain lease-before-gate ordering initially.
- Fable findings resolved in the design: correct input attribution, separate concurrency caps, retained ownership across replay; pre-start GC handoff deferred.
- Checks: 2126 model states / 6108 transitions, 13 local links with none missing, staged diff check passed; next implementation slice is persistent ready selection.

## Implementation checkpoint 2026-09-18
- Toast tasks.cc enqueue_bg_task appends admitted work FIFO; completion priority applies at admission, not ahead of admitted siblings.
- Barn currently drains a ready snapshot; input tick returns before background dispatch whenever input is queued.
- Next slice: persistent bounded batches and completion-driven server dispatch, retaining the all-ready runtime API for existing callers.
- No implementation edits yet; managed Toast coverage and regression tests precede production changes. Notes remain uncommitted.

## Bounded dispatch checkpoint 2026-09-18
- Implemented retained background admission, one-batch server selection, immediate reselection, and removal of input-driven admission suppression.
- Regression reproduced before fix; engine/server suites and focused race checks pass. Fresh build passes unchanged managed zero-suspend check after Toast verification.
- Full repository Go test is running; commit pending. Existing scripts and exact commands documented in the design.
- This increment has no per-principal ledger, direct helper wake notification, or gate changes; Mongoose latency and cross-concurrency acceptance remain unmeasured.

## Commit checkpoint 2026-09-18
- Committed ef1ec2e on fix/mongoose-workload: retained background tasks and bounded completion-driven server dispatch.
- Full go test ./... exited 0; focused race checks and managed Toast/Barn zero-suspend plus capability admission passed.
- Tracked working tree clean after commit; this checkpoint stays uncommitted and unrelated untracked files were preserved.
- Next: event-driven helper wakeups and principal-aware admission; measure identical Mongoose workloads before claiming latency improvements.

## Wakeup implementation checkpoint 2026-09-18
- Added coalesced manager/task notifications and deadline-driven server loop; Toast yield/admission check passed unchanged.
- Immediate wakeups require a physical VM handoff guard, distinct from logical suspended/queued state.
- Initial engine tests exposed direct tasks absent from the catalog retaining that guard; runtime now retains leased task references until release.
- Next: verify handoff/idle/deadline integration, run race checks and managed Barn checks, document and commit only this wakeup increment.

## Wakeup verification checkpoint 2026-09-18
- Replaced 10 ms dispatch polling with coalesced changes plus the next scheduled deadline; retained physical-handoff tasks until final lease release.
- Full go test ./... passed; broad task/engine/server race run passed (engine 194.992s), and focused handoff/integration checks passed.
- Fresh managed Barn and canonical WSL Toast both passed unchanged yield/admission and external-command/admission scenarios; no conformance changes.
- Implementation and reproduction commands committed on fix/mongoose-workload; notes remain uncommitted. Principal fairness, gate policy, and live latency matrix remain next work.

## Shared admission checkpoint 2026-09-18
- User authorized principal fairness plus commit admission. Fable review session a9ab3ce1-a743-4eff-81c0-8ddac1dbab95 is auditing runtime integration.
- Initial audit found cap deadlocks through suspended Eval, checkpoint/error hooks, login completion callbacks, and blocking forced input.
- Standalone weighted admission and FIFO grant primitives now pass focused race tests after compile-failing regression tests; neither is wired to runtime yet.
- Next: finish review, resolve explicit reservation inheritance/root publication/cancellation, integrate with gate wait accounting and deterministic cap-one tests.

## Shared admission integration checkpoint 2026-09-18
- Fable completed with REVISE: nested reservation borrowing, suspended Eval, queued checkpoints, cancellation, and sweep exemptions need explicit integration.
- Canonical managed Toast dump/exec admission run completed with six passing tests; no conformance expectations changed.
- Admission and FIFO gate primitives passed focused race checks; store gate wiring passed focused tests before runtime plumbing started.
- Runtime integration is incomplete and uncommitted; next wire ownership and cancellation, then test cap-one paths and request fresh review.

## Admission ownership checkpoint 2026-09-18
- Runtime entrypoints now acquire shared admission; nested error/timeout hooks borrow scopes and completion callbacks run after physical release.
- Background dispatch reserves an inspectable task before admission; kills cancel waiters. Direct arguments have explicit GC root pins while waiting.
- Exclusive grants now bind transactions; shared commits remain noncancellable, with gate wait observers surviving renewal. Dump requests use the server loop.
- Compile-only checks passed for affected packages; next fix forced-input backpressure, add cap-one regressions, run behavior/race checks and fresh Fable review.

## Admission regression checkpoint 2026-09-18
- Six new cap-one runtime regressions passed under race: eval yield, login callback, nested error hook, queued kill, argument roots, and input-close/system-hook split.
- Broad package run passed server/store/task/primitives; engine had one GC observer failure, corrected to observe the earlier argument-pin barrier; focused rerun passed.
- Input lanes now use nonblocking mailboxes, including queued phantom forced input; cancellation cannot resurrect a killed task at physical lease acquisition.
- Fable implementation review is running (session a9ab3ce1-a743-4eff-81c0-8ddac1dbab95); next finish accounting/selection tests, managed conformance, review fixes, and commit.

## Admission verification checkpoint 2026-09-18
- Full go test ./... passed; further cap-one gate cancellation, service-based background ordering, force-input flood and configuration checks passed under race.
- Canonical Toast managed force-input and scheduling suites passed all 40 cases, including admission; no conformance expectations changed.
- Full managed Barn conformance matrix at limits 1/16 is running via new scripts/test-shared-admission.ps1; broad race run also active.
- Fable draft findings on raw cancellation errors, input hook attribution, and phantom lane retirement have been addressed; await final review, document limits, then commit.

## Admission review closure checkpoint 2026-09-18
- Fable accepted architecture with F1/F2 follow-ups; queued shutdown cancellation now preserves state and player connection hooks use input admission.
- Broad race run passed (engine 199.130s, server 2.315s, store 4.046s); post-review cap-one/config focused checks also passed.
- Removed obsolete tokenless gate APIs, documented settings, metrics, reproduction script and limits; fresh full Go run and bounded final Fable review are active.
- Full managed cap-one suite reached 88 percent with cap-sixteen queued; no terminal conformance summary yet. Next collect summaries, rebuild/recheck final changes, and commit explicit files only.

## Checkpoint consistency investigation 2026-09-18
- Full cap-one run: 80 failed, 12874 passed, 24 skipped; focused pre-change 83751cc recheck reproduced 79 failures, leaving the WAIF suspended-restart case as the delta.
- Source shows task snapshots precede the separately gated store snapshot; resumed WAIF mutation can therefore combine with an older continuation. Canonical Toast passes the unchanged case.
- Added a failing callback-result lifetime regression, then pinned the returned value through callback completion; focused race reruns pass. Started slices now keep their service floor on coarse clocks.
- Cap-sixteen/full and final focused runs continue. Fable is reviewing a checkpoint admission pause plus sweep/start barrier; implement a deterministic regression and repair before committing.

## Coherent checkpoint implementation checkpoint 2026-09-18
- Deterministic server regression reproduced execution between task/store capture; checkpoint now pauses admission, drains reservations, holds SweepMu/VMStartMu through serialization, and resumes before hooks.
- Full Go suite passed after repair; focused checkpoint/admission race tests passed. Broad race and full rebuilt limit-1/16 conformance runs are active.
- Initial limit-16 full run had 82 failures; baseline focused recheck reproduced 81, leaving suspended-task exactly-once restart as its delta. Both remaining deltas are checkpoint consistency cases.
- Additional canonical Toast connection/checkpoint suites passed all 28 cases. Await Fable checkpoint review and final matrix before staging explicit files; notes remain uncommitted.

## Final matrix checkpoint 2026-09-18
- Rebuilt checkpoint cap-one run completed: 79 failed, 12875 passed, 24 skipped; exact failed-list comparison matches the 79 baseline focused reproductions.
- Current broad race run passed admission, commitgate, engine, server, task and store (engine 189.055s). Cap-sixteen full run continues.
- Fable checkpoint follow-up failed with HTTP 429/out-of-credits before verdict; Gemini fallback failed eligibility at startup. Earlier admission ACCEPT does not cover checkpoint follow-up.
- Local source audit confirms shutdown checkpoints precede runtime cancellation and paused Eval resumes through admission; record independent-review limit explicitly, finish matrix, stage only owned files and commit.

## Commit preparation checkpoint 2026-09-18
- Rebuilt full cap-sixteen run completed: 80 failed, 12874 passed, 24 skipped; all failures reproduced in the focused baseline inventory, with both repaired restart cases passing.
- Full cap-one inventory exactly matches 79 baseline reproductions. Full Go and broad race passed; strengthened checkpoint regression passed ten race repetitions.
- Final validation report records complete runs, focused-baseline limitations, missing independent checkpoint verdict, and unmeasured live Mongoose performance.
- Explicit owned-file staging excludes notes and conformance assertions. Removed the clean temporary baseline worktree, retained binary/reports, and proceeding to branch commit.

## Live measurement checkpoint 2026-09-18
- User requested actual measurement after b4120d8; added uncommitted scripts/measure-mongoose-admission.py for disposable baseline/new runs, GOMAXPROCS 1/16, prompt login and command completion timing.
- Temporarily suspends prior instrumented PID 26992 only while measurement runs and resumes in finally. Credentials are environment-only; source DB/sound/tz copied per trial.
- First pilot invalid: debug HTTP precedes MOO listener; corrected readiness to actual listening log event. Second pilot baseline cap-one failed clean login within 180 seconds with repeated database-not-open confunc errors.
- New cap-one pilot currently runs in exec session 96209 under .tmp/admission-measure-pilot2. Finish pilot, investigate fixture/startup evidence, then repeat valid measurements and document limits; no performance win established.

## Measurement protocol correction checkpoint 2026-09-18
- Both cold cap-one stress pilots failed clean login in 180 seconds (baseline 132 attempts, new 27); baseline cap-sixteen also failed with five-second retries.
- Frozen news/log SQLite backups from the earlier working run did not restore clean baseline login within the same deadline. Preserve startup failures; do not average them into successful-login latency.
- Stopped the initial repeated matrix after its completed baseline trial; unfinished new-16 trial is censored. Explicitly resumed prior PID 26992 successfully before the next experiment.
- Current exec 88244 runs three paired repetitions at caps 16/1 in .tmp/admission-measure-commands, explicitly accepting and flagging unclean authenticated sessions to measure commands, CPU, background log activity and checkpoints. No runtime edits this turn; measurements and scripts remain uncommitted.

## Live latency regression checkpoint 2026-09-18
- Corrected checkpoint probe to watch terminal server events; the prior multi-statement marker was invalid. Real checkpoint failures in both builds report queued task 43199186 environment names 3 versus 38 values.
- Final paired experiment is exec 5810, .tmp/admission-measure-final: three repetitions at GOMAXPROCS/admission 16 and 1, authenticated-but-unclean sessions explicitly flagged.
- First two one-thread new-build p95s are 5616/6033 ms versus baseline 87/95 ms; new run-one gate wait is only 0.5373 ms total. Sixteen-thread results are close; do not claim a speedup.
- Added independent --admission-limit override for a follow-up capacity-two/one-thread experiment after the matrix. Scripts/report remain uncommitted; prior PID 26992 is currently paused by the experiment and must resume before final delivery.

## Measurement completion checkpoint 2026-09-18
- Fifteen final trials completed with identical DB/SQLite seeds: baseline single-thread median/p95 26.52/93.16 ms, admission-one 1232.73/6033.07 ms, admission-two 32.57/102.97 ms (medians across three runs).
- All final sessions were explicitly unclean and all checkpoints failed on queued task 43199186 names/value mismatch. No runtime fix or general performance acceptance claimed.
- Added measurement/summary scripts plus detailed report and aggregate JSON; updated design/validation status to record rejection of the single-slot default. Python compilation and artifact consistency audit passed.
- Prior PID 26992 resumed; CPU advanced 1.047 seconds during a one-second observation. Stage only owned scripts/reports/design and commit on fix/mongoose-workload; leave these notes uncommitted.
