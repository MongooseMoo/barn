# perf/escalate-by-wasted-work

## Goal

Escalate a task slice to the exclusive commit gate by wasted work, not only by
attempt count. Motivation (from the team lead's verified census on the 1p real
Mongoose workload): about 1 run in 4 falls into an abort storm; every lost
commit is stale on `#1007.scheduled_tasks` (read at `#1007:task_valid` line 4,
written by `#1007:s_run` via `#1007:insert_task` line 20); slices took up to 28
attempts and 86.8% of ticks were wasted, because escalation only happened after
63 lost attempts.

## State

Implemented and committed on branch `perf/escalate-by-wasted-work` (not pushed,
no PR). Timing gates not run by this agent (the lead runs them).

- `engine/task_runtime.go`: new const `escalateAfterWastedTicks = 200_000`;
  `lostAttemptTicks` records the ticks of the attempt that just lost; at the top
  of the retry loop
  `t.IsForked && attempt > 0 && lostAttemptTicks >= escalateAfterWastedTicks`
  takes the gate with new cause `gateCauseWastedWork = "wasted work"`. Only
  forked tasks escalate this way; player commands do not. The attempt-63 rule
  and the `!canRetry` rule are unchanged (cause precedence: no restorable
  state, then retry budget spent, then wasted work).
- `engine/wasted_work_escalation_test.go`: three tests. A forked task's first
  run (retryable via `forkFirstRunRebuilder`) that loses after >= threshold
  ticks escalates once with the new cause; one that loses under the threshold
  retries without the gate; a plain command task that loses after >= threshold
  ticks retries without the gate. The fixture makes `#0` its own
  `$server_options` with `bg_ticks` 10M and loads it into the session the way
  server boot does (`Session().LoadServerOptionsFromStore`), since forked
  tasks take background limits (default 30,000 ticks).
- Break-checks run: dropping `t.IsForked &&` fails only the command test
  (escalations=1); dropping the whole wasted-work term fails the forked
  expensive test (64 attempts, escalation only at the retry budget);
  threshold 1 fails the forked cheap test (escalations=1).

## Decisions and why

- Forked tasks only, from the lead's real-workload gate. 16p: escalating heavy
  player commands serialized the server, goodput mean 215 -> 149/s, p999 up to
  4.7 s. 1p with the rule: no abort storm in 8 runs vs 1 of 8 on base, abort
  rate 0.01-4% vs 2.9-8.3%, p999 <= 185 ms vs up to 574 ms. The 1p storms are
  in forked background jobs, so limiting the rule to forked tasks keeps the 1p
  gain without serializing 16p commands.

- Wasted-tick source: `attemptTicks` (`bcVM.Ticks - sliceEntryTicks` after the
  attempt), the same number added to `rerunTicks`, which `NoteSliceAttempts`
  sums into the census's `WastedTicks`. No second counting scheme.
- Both loss paths set `lostAttemptTicks`: the commit-validation loss and the
  irreversible-effect boundary loss (`ConflictRetryRequested`). Both are lost
  attempts that the census counts as rerun ticks, and the attempt-63 rule
  already escalates after either, so the gate state on re-run is the same one
  that rule already produces.
- `attempt > 0` guard: without it a threshold of 0 would escalate the first
  attempt (seen in the threshold-0 break-check).
- 200_000 is a starting value. The 141k (ordinary commands) and 240k-470k (long
  scheduled jobs) per-slice figures come from the lead's brief; I found no
  record of them in this repo. Tuning belongs to the lead's real-workload gate.

## Surprises

- The first test draft derived its tick limit from the constant, so the
  threshold break-checks failed on the task's own tick limit, not on the
  escalation assertion. Fixed with a literal 10M-tick limit, and the
  escalation assertion is checked first in the cheap-loss test.

## Next step

Lead: re-run the 1p and 16p real-workload gate plus conformance with the
forked-only rule; tune `escalateAfterWastedTicks` from the result.
