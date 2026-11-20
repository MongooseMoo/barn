## Checkpoint 2026-09-29
- User requests a plan first for PR #345: retryable continuations, sort optimization, and completion-aware benchmarks.
- Review prototype lives at C:/Users/Q/code/barn-pr345-perf-review; PR head is b33a487a7078f40c0a4f21f6d7c483e82dd66faf.
- Existing persistence snapshots omit live LoopStack and restore recycle guards; plan a separate in-memory retry checkpoint with explicit ownership and GC roots.
- No implementation for this new request yet; oracle behavior needs fresh managed verification before implementation. Canonical installed WSL distribution is Debian.
- Next: finish rollback/effect/completion design and write the concrete implementation and verification plan. Leave this checkpoint uncommitted.

## Execution checkpoint 2026-09-29
- Worktree C:/Users/Q/code/barn-pr345-retry, branch perf/pr345-continuation-retry; merged origin/master ab78a08 into PR head as 77e05a3.
- Managed Debian WSL Toast session at conformance commit 378ef52: 47 passed, 13027 deselected; log .tmp/oracle-focused.log in implementation worktree.
- Completion-aware harness committed as 1cddf1b; focused completion tests output `ok github.com/MongooseMoo/barn/engine 0.123s`.
- Implementation now changes files; VM checkpoint/runtime retry integration remains pending. No blocking oracle failure.
- Next: capture a corrected-harness control, implement typed VM retry checkpoints and guard rollback, then admit and test resumed slices concurrently.

## Execution checkpoint 2026-09-30
- VM checkpoints, task restore, resumed-slice retry admission, and recycle effect guards are implemented but uncommitted in barn-pr345-retry.
- Focused continuation race run passed; broader engine run failed and its exact failing tests still need inspection (.tmp/go-focused-full.log).
- Broader Toast session has a reproducible next_recycled_object assertion failure; isolated oracle log confirms it, separate from targeted continuation behavior.
- Corrected-harness control executable exists at .tmp/control-engine.test.exe; sort/all_members production optimization remains pending.
- Next: identify broader test failures, finish optimizations and deterministic regressions, then run managed conformance and serialized A/B measurements.

## Execution checkpoint 2026-09-30, cancellation repair
- Full VM/task/engine/builtins/store run now passes (.tmp/go-focused-full-v2.log); added pool coverage and cancellable shared commit admission.
- Sort uses direct Elements and generic stable index sorting; all_members traverses Elements once. Production changes remain uncommitted.
- Retry budgets now retain the logical slice deadline; auditing tick accounting and completion flush accounting next.
- Broader Toast next_recycled_object assertion remains a confirmed oracle-side failure, with no change planned to that behavior.
- Next: add differential builtin benchmarks/tests, run full managed Barn conformance, and collect serialized corrected-control performance evidence.

## Execution checkpoint 2026-09-30, broader verification
- Managed focused Barn suite: 223 passed, 29 skipped; full managed suite running in session 76099, log .tmp/barn-managed-full.log.
- Full Go run passed before the final budget/completion adjustments; full VM/task/engine/builtins/store race run now running in session 72453.
- Benchmark harness now waits for post-flush Done for synchronous and resumed commands and invalidates measurements with unsettled commands.
- Added frozen-control differential sort test, callback benchmarks and continuation capture/restore benchmarks; focused retry/completion tests pass.
- Next: complete exactly-once async/effect regressions, finalize commits, finish terminal conformance results, then measure A/B serially.

## Execution checkpoint 2026-09-30, control and terminal results
- VM checkpoint commit is 1c1d5a4; exact control worktree barn-pr345-control at f4cfa16 includes matching terminal-flush measurement only.
- Candidate full managed result: 3 memory_usage failures, 12631 passed, 440 skipped; control full managed run session 66646 is in progress.
- Full race run passed for VM/task/engine/builtins/store. Vet and staticcheck exited 0; latest focused race run session 67115 covers added regressions.
- Added forced-conflict coverage for sort/all_members/read/SQLite/curl, committed effects and task locals, checkpoint WAIF roots and retained recycle guards.
- Next: finish retry integration commits, refresh terminal control comparison, then run serialized callback/checkpoint/application A/B evidence and update PR.

## Execution checkpoint 2026-09-30, measured tradeoffs
- Implementation commits through 4a4d12d; full Go/static checks passed and full control/candidate managed suites have identical three memory_usage failures.
- Ten-sample callback/continuation benchmarks completed under serialized Global\BarnBenchmark mutex: read-heavy 16-player +92.14% goodput; shared-write -14.08%.
- Callback sort time improved 5-22%; membership no/sparse matches improved about 9-10%, dense match difference inconclusive.
- Real Mongoose control measurement failed: five home commands lacked terminal flush acknowledgement at the 10-second completion deadline; no valid application A/B result yet.
- Next: diagnose completion timeouts with retained task state and CPU evidence, resolve measurement validity, then consider contention recovery and finish PR evidence.

## Execution checkpoint 2026-09-30, measurement follow-up
- Mongoose control home probes fail at both 3s and 60s: queued, non-reading, ready continuations with lease=0 and saved VM; CPU profile shows #1007:s_run at 78.62%.
- Harness now models production bounded batches/wake notifications; focused control still leaves queued home commands at 10s, so no validated application gain is claimed.
- Experimental continuation escalation after two conflicts preserves read-heavy gain but does not repair shared-write cost (-16.38% goodput, ten samples); change is uncommitted and needs rejection or further evidence.
- Latest committed implementation remains 4a4d12d. Original ten-sample callback/checkpoint/continuation logs and full suite comparisons remain available under .tmp.
- Next: preserve proven retry policy, collect both application variants through terminal inventories, and finish transparent performance/CI evidence without hiding contention or drain failures.

## Execution checkpoint 2026-09-30, final measurements
- Rejected early escalation experiment; committed a3fcc7b keeps the original 63-conflict escalation policy.
- Replayed instruction counts now start at the slice-entry count; retries retain the original seconds deadline, with 64-attempt regression coverage.
- Control aefe85c and candidate a3fcc7b use identical bounded-batch, terminal-flush measurement code and compact timeout diagnostics.
- Final ten-sample micro/continuation and five-pair Mongoose run is active in session 92188, output .tmp/perf-release.
- Next: review source while measurements run, collect terminal inventories, publish evidence and PR update, and monitor exact-head CI; merge remains unauthorized.

## Execution checkpoint 2026-09-30, terminal evidence review
- Final ten-pair continuation results: +77.06% read-only 16-player goodput, -18.81% shared-write goodput; one-player results inconclusive.
- Callback/checkpoint raw samples are copied into untracked experiments/2026-09-30-pr345; report draft documents allocation and contention tradeoffs.
- Seven of ten application processes have terminal inventories; both revisions have command timeout failures, so a whole-application throughput ratio is invalid.
- Added uncommitted ordinary/renewed shared-commit cancellation regression, queued behind a real exclusive owner; source review preserves waiter/owner cleanup.
- Benchmark session 92188 is still active; defer CPU-heavy verification until it finishes, then test/commit evidence and push PR345 without merging.

## Execution checkpoint 2026-09-30, published PR
- All five application pairs reached terminal inventories: control 5/5 invalid, candidate 4/5 invalid; all logs and counts are tracked, with no application speedup claim.
- Final Go all/vet/staticcheck/focused race/build returned 0; cancellation mutation fails ordinary/renewed cases, restored source passes; driver negative probe retains ten logs then rejects.
- Source d5ae6a1 and evidence e20c636 are committed; safe fast-forward push updated feat/threaded-builtins-yield and PR345 title/body.
- Exact-head CI run 36686374215 is in_progress for e20c636f81b275e87fa9c859a1a23cc6821d05d2; both Build & unit tests and MOO conformance are in progress.
- Implementation worktree is clean. Next: monitor terminal exact-head CI, repair only in-scope failures if needed, update PR evidence links, and report completion without merging.

## Execution checkpoint 2026-09-30, CI ordering assertion
- Exact-head run 36686374215: build success, conformance 1 failed / 12646 passed / 420 skipped; only suspend_zero_after_resume_observes_resumed_slice fails (expected 1, got 0).
- CI uses newer conformance 7f05b7070f2954a200f7e8c2a06c814ef7981c4e; isolated read-only dependency worktree C:/Users/Q/code/mct-pr345-ci added.
- No Barn root-cause work yet: first oracle admission rejected missing target manifest; corrected manifest admitted, but the row skipped because queued_tasks provider was deselected.
- Canonical Debian WSL Toast managed retry now selects capability_admission, queued_tasks providers and the exact assertion; session 27324 active.
- Next: confirm exact Toast behavior, compare candidate/control, then repair only proven continuation-ordering regression and rerun exact-head CI.

## Execution checkpoint 2026-09-30, causal continuation repair
- Oracle exact assertion passes with queued_tasks provider selected: 19 passed / 4 skipped; standalone Windows control passes, initial candidate fails the same assertion.
- Added failing multi-worker scheduler regression and physical/admission-handoff regression before fixing readiness; reused the task's readier relation until lease release.
- Scheduler excludes only dependent readiers from ready selection and wake timers; unrelated work can still batch, and future delayed forks do not hold their parent.
- Source 903caeb committed after Go all/vet/staticcheck and focused scheduler/task/engine race returned 0; fixed managed fork/queued-task/threaded family: 27 passed / 4 skipped.
- Next: rerun original frozen threaded selectors, remeasure ten continuation pairs for final source, update evidence and PR, then monitor a new exact-head full CI run without merging.

## Execution checkpoint 2026-09-30, final source published
- Original frozen threaded selectors pass on final source: 47 passed; current conformance fork/provider family passes 27 with 4 skips.
- Final source 903caeb ten-pair continuation evidence: +77.26% read-only 16-player goodput, -18.49% shared-write; one-player results inconclusive.
- A final bounded Mongoose pair also has terminal timeout failures on both sides; raw final and initial inventories are retained, with no whole-application gain claim.
- Evidence commit d3bb0fe pushed safely to PR345; implementation worktree clean, body updated for final measurements and causal ordering.
- Exact-head CI 36691093076 is in_progress for d3bb0fe1267fe8ab76b441e073fb68394a9a7202; next collect terminal full-suite result, repair only scoped regressions if any, report without merging.

## Execution checkpoint 2026-09-30, delivered
- Exact-head CI 36691093076 completed success for d3bb0fe1267fe8ab76b441e073fb68394a9a7202: Build & unit tests SUCCESS; MOO conformance suite SUCCESS.
- Full Linux managed summary: 12647 passed, 420 skipped, 1 warning in 336.74s; conformance dependency is exact 7f05b7070f2954a200f7e8c2a06c814ef7981c4e.
- PR345 is OPEN, mergeStateStatus CLEAN; no merge performed. PR body links exact-head CI, report and retained raw performance/timeout evidence.
- Final measured tradeoff remains +77.26% read-only 16-player goodput and -18.49% shared-write; application inventories remain invalid on both revisions.
- Plan status updated to delivered; implementation worktree clean. Requested execution is complete.
