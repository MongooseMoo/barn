# Fix: continuous fuzz chain stops after controller-dispatched cycles

## Goal

Q, 2026-11-10: fix the machine so the dedicated-continuous fuzz chain does not
stop again. "Restart after any cycle."

## State

- `fuzz.yml` has a final `Submit continuous successor` job (`needs: [route, accept]`,
  `if: always() && ... dedicated-continuous && route success`). It calls
  `fuzz_compute.cjs successor()`, which dispatches the next cycle with
  `GITHUB_TOKEN` unless the stop switch is off or another dedicated cycle is
  queued/running (the current run is excluded by run id).
- `fuzz-controller.yml` keeps `replenish` (now restarts after unaccepted cycles
  too) and `watchdog` as backups.
- `fuzz_ops.py status`: no queued/running cycle after a continuous cycle is
  BROKEN at once (the successor is submitted before the cycle ends).
- Tests: `scripts/test_fuzz_compute.py`, `scripts/test_fuzz_ops.py` pass (38).

## Evidence

- Runs dispatched by `ctoth` (42750434626, 42790517087 ... 42871469786) each
  produced a controller `workflow_run`. Runs dispatched by the controller
  (`github-actions[bot]`: 42766490058 ended 08:43Z, 42897395293 ended 23:13Z)
  produced none; the chain stopped both times.
- No source found that states this directly; GitHub's GITHUB_TOKEN page says
  GITHUB_TOKEN-triggered events create no runs except workflow_dispatch and
  repository_dispatch, and does not mention workflow_run. The finding is the
  observed 2 of 2 against 6 of 6.
- Scheduled watchdog: 2 runs (16:59Z, 22:20Z) between cron add at 08:27Z on
  2026-11-10 and 01:00Z next day; about 34 due. Community discussion 206019:
  schedules created late or never since 2026-08-26, no GitHub answer.
- Four cycles (11:30, 13:39, 15:48, 17:59) failed acceptance on
  FuzzReadDatabase infrastructure and FuzzRestoreVMSnapshotDoesNotPanic
  (OOM, separate note); 42871469786 and 42897395293 were accepted.

## Decisions and why

- Successor in the cycle itself: GITHUB_TOKEN workflow_dispatch creates runs,
  so no secret or PAT is needed, and no event GitHub may not send is relied on.
- Kept `replenish`: run 42937999782 (dispatched by the agent at 01:18Z without
  Q's agreement) runs the old fuzz.yml with no successor job; its completion
  fires `replenish`, which starts the first cycle with the new job.

## Surprises

- A `dedicated-once` dispatch queued while a continuous cycle runs makes the
  successor job skip (another dedicated cycle exists); the chain then relies on
  the watchdog. Not changed.

## Next step

After merge: confirm run 42937999782's completion produced a replenish
dispatch, and that the next cycle's `Submit continuous successor` job
dispatched its successor while the cycle was still running.
