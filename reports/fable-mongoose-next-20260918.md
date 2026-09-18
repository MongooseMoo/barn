# Mongoose next fixes — review

Reviewer: Claude (fable)
Date: 2026-09-18. Branch `fix/mongoose-workload` at `1219a0e`. Read-only review; this file is the only thing written.

Method: read the notes/docs/scripts, the engine/scheduler/store/server code, the ten CPU profiles and the live
debug log under `.tmp/mongoose-account-20260917`, and the Toast mongoose source in WSL
(`/root/src/toaststunt-mongoose-login-20260917/src`). I also used the documented inspection mode of the run
directory's `barn.exe` (`-verb-code`, scalar `-eval`, with `-log-dir "" -debug-addr off`, against the disposable
`mongoose.db.new` copy) to read four scheduler verbs. No server was started or stopped, no oracle run was made.
One offline `-eval` attempt to time a simulation cycle failed with `E_INVARG` and produced no data. No verb source,
property values or account data are reproduced below — only structure and verb references.

## Headline

The problem is mis-framed as "cold start". **Barn never reaches idle.** The live log for the current instance spans
2026-09-17 16:52:48 → 2026-09-18 14:09:59 (76,631 s) and contains 21,156 `slow task slice` records; 76,192 s of
them are `#1007:s_run` — a **99.4 % duty cycle**, median slice 3.69 s, max 18.98 s, back-to-back, with zero
connections. Today's profile (`workload-20260918-140632-905`) still shows 95 % of a core in `#1007:s_run`.
Every foreground commit or irreversible builtin therefore queues behind a multi-second exclusive-gate hold, all
day. The 14.4 s first slice and the 11.25 s login block are the first instance of a steady state, not a boot transient.

I found one **source-proven semantic divergence** that plausibly drives this (finding F2), and the evidence gap that
has kept it hidden (F1). Fix F2 first, measure, and only then spend more on VM speed.

## Findings

### F1 (proven) — "`#1007:s_run` dominates CPU" is an attribution artifact

`runTask` labels a slice with the task's *root* verb (`engine/task_runtime.go:50`, `t.This`/`t.VerbName`).
`#1007:s_run` collects due jobs, forks up to `max_concurrent` (8) workers, and those workers **run the jobs inline**,
calling `yin()` between jobs. A forked task inherits the root verb name, so every scheduled job in the database —
the 1 Hz `#4143:cycle` simulation frame over 56 objects, ~30 `:tick` jobs at 300 s, the one-shot
`#9655:fire_transition` chain, etc. (128 entries) — is reported as `s_run`. The scheduler loop itself is cheap.
The label tells us "scheduled jobs are expensive", nothing more.

`yin()` matches Toast (`builtins/tasks.go:565-598` vs `execute.cc:3564-3574`): it yields only under 2000 ticks or
2 s remaining. With the database's 20M-tick/30 s background budget it effectively never yields, so **a slice's
length is the job list's full run time**. That is equally true on Toast; Toast simply finishes sooner. Nobody has
yet measured by how much — see "Required oracle comparisons".

### F2 (divergence proven from source; workload impact is a hypothesis) — ready-but-not-yet-run tasks vanish from `queued_tasks()`

- `Scheduler.Ready` pops **every** ready task and claims it immediately (`engine/internal/scheduler/scheduler.go:93-101`,
  also the catalog arm at `:111-119`). `TryClaimQueued` flips state to `TaskRunning` (`task/task.go:273-281`).
- `ProcessReadyTasks` then runs that whole snapshot serially, one solo batch per forked/resumed task
  (`engine/runtime.go:467-485`, `scheduler.go:126-146`). Its doc comment ("executes at most one task") is stale.
- `queued_tasks()` lists only `TaskQueued`/`TaskSuspended` (`task/manager.go:79-99`).

So while the first task of a pass runs — for seconds — every sibling that is ready but has not started is invisible to
`queued_tasks()`. Toast's `bf_queued_tasks` enumerates `active_tqueues->first_bg` (ready background tasks that have
not run) as well as `waiting_tasks` and suspended tasks (`tasks.cc:2509-2530`, second walk `:2546-2571`).

Why it matters here: `#1007:schedule` (also `schedule_every`/`schedule_at`) ends with "if the recorded scheduler task
id is not in `queued_tasks()`, fork `reset_scheduler`". When the scheduler is behind, the next `s_run` is forked with
delay 0 and lands in the same ready snapshot as the workers; the workers' jobs call `:schedule` (the transition chain
reschedules itself on every fire) and, on Barn only, conclude the scheduler is dead.

Evidence consistent with that: the log holds **394,403 `irreversible-effect boundary` records from tasks rooted at
verb `schedule`** — 5.15/s for 21 h. The only fork in that verb is the reset path. (`s_run`-rooted: 4,953; everything
else: ~20.) I have not proven that the resets are what keeps the job backlog permanently non-empty; that is what the
post-fix measurement decides.

Likely second symptom of the same mechanism (unverified): `kill_task()` on a claimed-but-unstarted sibling.
Toast removes it before it runs.

### F3 (proven) — the login block mechanism, and why it is duration-bound

Foreground input runs on per-connection goroutines (`server/input_processor.go:237-274`), concurrent with the
background pass, so it blocks only on the store commit gate:

- A resumed slice cannot be re-run, so it takes the gate exclusively for the **whole slice**
  (`engine/task_runtime.go:125-128`; `db/store/store_core.go:280`).
- A first-run forked slice takes it at its first irreversible builtin and keeps it to slice end
  (`task_runtime.go:189-199`, released at `:481`). `kill_task`, `sqlite_*`, `file_*`, `exec`, `server_log`,
  `set_connection_option` are all irreversible (`builtins/base_descriptors.go`).
- Ordinary commits take the gate shared (`db/store/store_txn.go:2645-2651`); login crosses
  `set_connection_option`, so it needs it exclusively.

`gate_wait` over 399,375 boundary records: p50 = p99 = 0, exactly one wait above 1 s — the 11.25 s
`do_login_command` at 16:53:12, queued behind the 14.44 s slice that ended at 16:53:12.556. The gate is correct and
matches Toast's serial model; latency equals background slice length. **Do not redesign it.** Shorten the slices.

### F4 (proven ordering, Toast-equivalent) — cold-start `database is not open`

`#0:server_started` forks SQL initialisation. Checkpoint-loaded queued tasks keep their past `StartTime`
(`engine/task_load.go:110-115`), so the overdue scheduler backlog sorts ahead of the new fork — as it does on Toast.
In the log, the `server_started`-rooted `sqlite` boundaries land at 16:53:14, after the 16:53:12 login. The order is
right; the window is 16 s wide because of F1–F3. **Do not reorder tasks or special-case SQL init.** Note the
comparison is currently unfair: nothing measures Toast's own window.

### F5 (known, unchanged) — 66 Toast-suspended tasks are dropped at boot

`failed to restore suspended task … empty bytecode program` ×66 (`engine/task_load.go:30-41`). Toast resumes them.
This changes the workload Barn runs (and legitimately triggers *one* scheduler reset). Real, but a larger project
(Toast PC → Barn bytecode mapping); keep it out of this sequence.

### F6 (hypothesis, do not chase) — Go preemption cost in one profile

`workload-20260917-165343-420` shows 27.9 % in `newstack → gopreempt_m → goschedImpl → wakep → semawakeup`.
Across the ten captured profiles the same path ranges 0.8 %–28 % on identical code; stacks truncate at `morestack`
and carry no task label. That pattern says host contention plus Windows thread-wake cost (Toast and a second Barn
were running), not a Barn code path. If it recurs, take a 2 s `/debug/pprof/trace` rather than another CPU profile.

### Review of the three recent commits — no critical defect found

- `591aeb8` (txn-local verb memo after unrelated writes): sound by inspection. Entries are refused when any step is
  owned (`store_resolve_cache.go:394-399`), re-validated for ownership and pointer identity on every hit
  (`:227-234`), every privatisation still wipes the memo (`store_txn.go:360`), and missing/recycled objects are
  recorded as steps (`store_txn.go:3487-3491`) so a later `create()` reusing that id invalidates a negative entry.
  Residual cost: a hit still replays read marks (`markVerbRead`/`markVerbScan` ≈ 3 %). Correct, leave it.
- `1219a0e` (compact temporaries): the remapper's opcode set is exactly the verifier's set of slot-bearing opcodes
  (`bytecode/compact_locals.go:48-72` vs `bytecode/verify.go:141-181`), named slots are untouched, widths and jump
  targets unchanged, and persisted VM snapshots carry their own program. Maintenance hazard only: a future
  slot-bearing opcode must be added in both places — worth a test that asserts the two lists agree.
- `f776c7f` (pprof labels, slow-slice log): harmless, but see F1 — `moo.verb` is the root verb and reads as if it
  were the hot verb. The slow-slice record lacks ticks, which is the one number that makes Barn and Toast comparable.

## Ordered recommended changes

**1. Make slow slices comparable (tooling, ~15 lines, no semantics).**
In the slow-slice record (`engine/task_runtime.go:58-64`) add `ticks` (VM ticks consumed this slice) and
`gate_held` (whether the slice ended escalated). Rename the label to `moo.root_verb` or document it in
`docs/mongoose-login.md`. This yields Barn's ticks/second on the real workload and tells you whether a 5 s slice is
500k ticks (Barn slow per tick) or 15M ticks (the MOO code is doing far more work than on Toast).

**2. Toast-first conformance test for F2, committed in `moo-conformance-tests` on its own.**
Test-owned object with a driver verb: fork A (0) then fork B (0); A's body stores whether B's id appears in
`queued_tasks()`; a later step reads the stored value. Expected from source: present. Add a second case: A calls
`kill_task(B)`; B must never run and the call must succeed. Prove both on the managed WSL mongoose oracle before
touching Barn. If Toast disagrees with my source reading, stop — F2 is dead and step 3 must not be made.

**3. Smallest Barn fix: claim at dispatch, not at snapshot.**
`Scheduler.Ready` stops calling `TryClaimQueued` (both arms); `runReadyTasks` claims each batch immediately before
`scheduler.Run` and drops members whose claim fails (killed, or taken by a nested `YieldReadyTasks` pass —
`engine/runtime.go:512`). Unstarted siblings stay `TaskQueued`, so `queued_tasks()`, `kill_task` and
`task_stack` see them as Toast does. Keep `WakeDue`/`Resume` where it is; only the claim moves. Fix the stale
`ProcessReadyTasks` comment. Two things to check while there:
  - a popped-but-unclaimed task must not be lost if the pass exits early (shutdown): either re-push unclaimed
    leftovers or confirm the pass cannot abandon;
  - `collectAllGCRefs` fails closed on any `TaskRunning` task (`engine/runtime.go:541`), so today deferred GC cannot
    settle mid-pass. After this change it can, per slice. `gc_sweep_last_ms` is 192–220 ms on this database — watch
    `barn.gc_sweeps` rate before/after; if it jumps, that is a new latency source to throttle, not a reason to revert.
Regression tests: a scheduler unit test (Ready leaves state `TaskQueued`; a task killed between snapshot and dispatch
never runs) and an engine test mirroring the conformance case. Run the perf gate (TestMongooseRealWorkload 1p+16p
plus the CI-style conformance suite) — this touches dispatch ordering.

**4. Re-measure before doing anything else** (numbers below). If the `schedule`-rooted rate collapses and the duty
cycle falls, cold start and warm latency improve together and F4 needs no code.

**5. Only if the duty cycle stays high: per-job cost, Toast vs Barn.** See oracle comparison (b). If ticks match and
Barn is merely slower per tick, the profile's cheapest safe target is the protected-builtin path:
`maybeProtectedRedirect` resolves `#0:bf_<name>` and then `pushProtectedVerb → startVerbCall` resolves it again, plus
a `"bf_"+name` allocation per call (`builtins/registry.go:258-264`); that path is 19 % cumulative in today's profile,
7 % of it the duplicate lookup. Pass the resolved verb through. Behaviour-neutral, benchmark exists
(`engine/protected_redirect_bench_test.go`). Verb dispatch (`startVerbCall` 23 %) and frame finalisation scans (7 %)
come after that. If ticks *differ*, it is another semantic divergence and the differ/YAML pipeline applies, not pprof.

**6. Make the cold-start comparison fair (script only).** Add a "time to first clean login" mode to
`scripts/mongoose-login.ps1`: from process start, retry the login probe until no connection-hook error appears;
record the elapsed time. Run it on both engines with `-Port 7777`. That replaces "Barn shows the SQL error" with a
number Toast also has.

Not recommended now: changing tick/second limits, reordering boot tasks, making `set_connection_option`
non-irreversible, VM cloning to make resumed slices retryable, any scheduler redesign, or F5.

## Required oracle comparisons

a. **Step 2's test** — the only one that gates a Barn semantic change.
b. **Per-job cost** (only if step 5 is reached): same wizard eval on both engines for three or four members of the
   simulation's object list — ticks and `ftime()` delta around a single `:cycle()` call — via the login script's
   `-Commands`. Avoid the whole-frame verb: it is latched and will return early while the scheduler is mid-frame.
c. **Toast's idle cost, which the notes never recorded**: after the same 10-minute soak with no connections, read the
   WSL oracle's CPU time against wall time (`ps -o etimes,cputime -p <pid>`). This is the parity target for the duty
   cycle; without it "responsive like Toast" has no number.

## Measurements that define success

Baselines are from the current instance's `barn/logs/latest.jsonl` and expvars (21.3 h, debug log level). A 10-minute
soak after boot is enough to compare.

| Metric | Baseline | Source |
|---|---|---|
| `s_run`-rooted slow-slice duty cycle | 99.4 % (76,192 s / 76,631 s) | `slow task slice`, sum of `elapsed` |
| slow slice median / max | 3.69 s / 18.98 s | same |
| `schedule`-rooted boundary records | 5.15 /s (394,403) | `irreversible-effect boundary`, `verb=="schedule"` |
| tasks started | 11.3 /s (866,715) | `barn.tasks_started` |
| first slice after boot / login gate wait | 14.44 s / 11.25 s | same log, 16:53:12 |
| deferred GC | 3,081 sweeps, last 220 ms | `barn.gc_sweeps`, `barn.gc_sweep_last_ms` |

Success for step 3: the `schedule`-rooted rate falls to near zero and the duty cycle drops materially; then compare the
remainder against (c). Success overall: time-to-first-clean-login and warm `look`/`who` latency (p50/p95 over ~20
samples from the script's events file) within a small multiple of Toast on the same fixture, with max slow slice
under ~1 s so no foreground command can wait longer than that.

## Uncertainties, stated plainly

- F2's divergence is read from both sources, not executed. Step 2 settles it. Its contribution to the 99 % duty
  cycle is a hypothesis; the reset path might be cheap and the jobs simply slow on Barn.
- I do not know Toast's slice durations, tick counts, or idle CPU for this fixture. Neither do the notes.
- The live binary was built at 16:52:46, eleven minutes before the three commits were recorded; per the notes it
  contains those changes from the working tree, but I did not verify the binary against `1219a0e`.
- The boundary and slow-slice records exist only at `-log-level debug`; at 91 MB/day that logging is itself a small
  load on the instance being measured.
- Three MCP connectors (Google Drive, Kiwi.com, Melon) are unauthorised in this session; none was needed.
