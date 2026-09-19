# Fable final review: shared admission — F1/F2 fixes and follow-up changes

Reviewer: Claude Fable (external, independent). Date: 2026-09-18.
Tree: `C:/Users/Q/code/barn`, HEAD `83751cc` plus the uncommitted working tree.
`AGENTS.md` read. Scope: only the changes listed in
`prompts/fable-admission-final-20260918.md` and any concrete blocker they expose.

Source review only. This file is my only write. **I ran no tests, builds, or
conformance**; the parent's statements that admission tests, full Go, broad
race, and the managed cap 1/16 runs pass or are running are not my
verification and are not relied on below. Other work was ongoing; line numbers
are from the state I read.

## Verdict: ACCEPT

Both required fixes are present and correct in source, the five follow-up
changes do what is claimed, and the plan's new implementation section matches
the code. I found no new blocker.

## F1 — kill / shutdown cancellation: fixed

- **Queued tasks survive `Runtime.Stop`.** `runTask`
  (`engine/task_runtime.go:51–68`) returns nil on `context.Canceled` from
  `enterBackground` and no longer calls `SetState(TaskKilled)`. A killed task
  was already marked by `Task.Kill`; a shutdown-cancelled one keeps
  `TaskQueued` and its saved VM. `ReleaseAdmission` still runs via defer.
  `runTaskBatch` only closes `Done` for Completed/Killed, so a preserved task
  is not falsely signalled.
- **No raw error to players, no ERROR record.** `runTaskAdmitted`
  (`:70–92`) maps any `context.Canceled` from the slice to nil *before* the
  `OnComplete` branch, so: `ExecuteVerbTaskSyncWithStart` gets nil and
  `executeCommandMatch`'s `conn.Send(err.Error())` is not reached; the
  scheduler result carries no error, so `runTaskBatch` logs nothing; and a
  killed login task does not fire its completion callback. All three
  cancellation exits inside the slice return `context.Canceled`
  (`acquireTaskExecution` refusal `:116`, retry-top `taskCtx.Err()` `:182`,
  entry `AcquireExclusive(taskCtx)` `:195–199`, post-VM `Done` branch `:566–570`),
  so the single `errors.Is` check covers the kill/admission and kill/gate
  races. Internal panics are still returned and logged.

Note, not a defect: a task that has already been **claimed** and is then
cancelled by `Runtime.Stop` (at `:182` or while waiting at `:195`) is still
marked `TaskKilled`. That matches the pre-existing post-VM shutdown branch and
happens after `server.shutdown`'s final checkpoint. "Stop preserves queued
continuations" is accurate for unclaimed work, which is what was asked.

## F2 — principal attribution: fixed

`callUserHook` now calls `RunServerVerbTaskWithArgstr(handler, verbName, args,
player, "", nil)` (`server/input_login.go:192`), which admits with
`system = false` → `inputAdmissionKey(player)` (anonymous for negative ids).
`RunServerVerbTask` (`system = true`) has exactly four remaining callers, all in
`server/server.go`: `server_started` (`:504`), `checkpoint_started` (`:513`),
`checkpoint_finished` (`:522`), `shutdown_started` (`:531`). Grep shows no
other non-test caller.

Consequence to be aware of: user hooks now depend on the input admission
context, so a `user_client_disconnected` hook that starts after
`CloseInputAdmission` during shutdown is refused and skipped silently
(`callUserHook` returns on error). Lanes were already being cancelled at that
moment, so this is not a new loss.

## Follow-up changes

| Change | Evidence | Result |
|---|---|---|
| Background order from one locked snapshot | `Scheduler.order func([]*task.Task)` replaces the comparator; the runtime computes each key once, then `Controller.Order` sorts indices under a single `c.mu` hold with a stable sort, clamping idle principals to the watermark (`engine/runtime.go:184–193`, `internal/admission/controller.go` `Order`). FIFO within a principal is preserved by stability | correct |
| Phantom lanes retire | `connectionWorker` deletes its map entry and returns when `connID < 0` and the queue is empty, under `workersMu`; `dispatch` appends under the same mutex, so no event can land on a retired lane; the deferred cleanup's `p.workers[connID] == ch` guard cannot delete a successor lane | correct |
| `EnqueueInput`/`Stop` serialization | `EnqueueInput` holds `enqueueMu.RLock`, checks `ctx`, and selects on `ctx.Done` (closing `Done`); `Stop` cancels first, then takes the write lock and drains `inputQueue`, closing each `Done`. Cancel-before-lock means a blocked sender always wakes; every accepted event is received exactly once (run loop → `dispatch` closes under `ctx.Err`, or `Stop` drains, or lane cleanup closes), so no double close | correct |
| `ResumeBackground` keeps group | `s.key.Principal, s.key.Class = principal, Background` (`internal/admission/scope.go`) | correct |
| Tokenless gate APIs removed | grep for `EscalationLock`, `EscalationUnlock`, `ExemptFromCommitGate`, `legacyGrant` across all `.go` files: no matches | correct |
| Maintenance accounting | `flushDeferredGC` records sweep cost via `admission.NoteMaintenance` (`engine/waif_lifecycle.go:269`); exposed as `barn.admission_maintenance_ns` | present |

## Plan section "Shared admission and commit grants (2026-09-18)"

Checked each factual claim against source; all hold:

- principals (player input, shared anonymous, active programmer, separate
  system for the four lifecycle hooks); selection rule; 1 µs floor; 1/8 EWMA
  clamped 1 µs–100 ms; watermark clamp keeps debt; 3:1 stated as
  intra-principal only;
- nested borrowing, completion callbacks after release, Eval
  release/reacquire-as-background, sweep exemption with separate occupancy,
  unclaimed/cancellable/inspectable waiters, pinned call arguments,
  input-only close;
- gate: FIFO cohorts, live-token exemption, tokenless APIs removed,
  only entry-exclusive waiting cancellable, owner-only release,
  `dump_database()` queues a main-loop request;
- configuration keys, zero-means-default, 0–1,000,000 bound
  (`config/options.go` `Validate`); the five `/debug/vars` gauges
  (`server/server.go:117–121`); `scripts/test-shared-admission.ps1` exists;
- the replay-ownership correction now describes the code's actual behaviour.

One wording nit, not an inaccuracy: "explicit values must be positive
integers" — zero is also accepted as "default", as the same sentence says.

I did not read `scripts/test-shared-admission.ps1` or execute any command in
the "Reproduce checks" block, so the description of what that script does is
unverified by me.

## Remaining scope

The documented limits (O(principals) watermark scan, unbounded forced-input
mailbox, cooperative slices, pre-existing direct writes, deferred-maintenance
exemption, unmeasured Mongoose performance) are accurately stated in the plan
and are not new requirements. The 1/16-client live performance matrix remains
the outstanding acceptance evidence before any performance claim.
