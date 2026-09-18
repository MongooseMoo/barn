# Mongoose follow-up to Claude Fable

Branch: `fix/mongoose-workload`. Review: [Fable recommendations](fable-mongoose-next-20260918.md).

Implemented the first recommended semantic repair: selecting a ready task no
longer marks it running. Each batch claims its tasks immediately before dispatch;
tasks killed or claimed elsewhere are skipped. This keeps unstarted siblings
visible to `queued_tasks()` and killable. The commit gate, task budgets, startup
ordering, and SQL initialization were not changed.

The new managed regression uses its own object/driver to observe and kill a ready
sibling. WSL Mongoose Toast passed both assertions with capability admission.
Before the fix Barn returned `[0, 1]` instead of `[1, 1]`; afterwards both servers
passed. The regression is committed separately in the conformance worktree as
`4b6ac54`. Barn's dispatch repair and tick/gate telemetry are `573c2c7`.

Validation:

- Full `go test ./... -count=1` passed.
- `go test -race ./engine/... -count=1` passed; a subsequent focused race run
  covered the telemetry capture before releasing the VM execution lease.
- Managed local regressions: 7 passed. Packaged fork/queue selection: 3 passed,
  9 skipped by existing requirements; the skipped cases are not evidence.
- `TestMongooseRealWorkload`, players 1 and 16, passed with no failed commands or
  uncaught exceptions. This mixes commands and is not an idle CPU comparison.

Fresh Barn workload evidence is under `.tmp/mongoose-fable-20260918` (private,
untracked). The sample `dispatch-fix-summary.json` covers 595.45 seconds:

| Metric | Fable baseline | After dispatch repair |
| --- | ---: | ---: |
| schedule-rooted boundary records/second | 5.15 | 0 |
| median s_run-rooted slow slice | 3.69 s | 0.722 s |
| slow-slice duty | 99.4% | 98.5% |
| maximum slow slice | 18.98 s | 15.90 s |

This removes the scheduler-reset traffic but does not remove the continuous
simulation workload. The new interval includes startup, probes, and concurrent
test activity, unlike the 21-hour baseline; these are observations rather than a
controlled end-to-end speedup. GC sweep frequency increased as anticipated;
the sampled last sweep was 0 ms, with no evidence yet of the feared 200 ms
per-slice cost.

Added repeatable workload summaries, per-object cycle probes, and clean-login
retry timing. The latter requires the complete MOTD marker and no connection-hook
error, records each attempt, and measures from process launch after build/copy.

Sequential fresh port-7777 probes with the same client settings (3s banner wait,
2.5s command spacing, 60s idle timeout, 90s attempt limit) recorded:

| Engine | First clean attempt | Observed clean welcome from launch |
| --- | ---: | ---: |
| WSL Mongoose Toast | 1 | 20.512 s |
| Barn | 2 | 137.041 s |

Barn's first attempt reported the SQL connection-hook error. Its elapsed result
includes that failed attempt's receive wait before retrying; this measures the
probe policy, not the earliest instant SQL became ready. The second Barn attempt
saw the MOTD at 35.530 s after connecting. The rebuilt Barn is left on port 7777,
PID 88896; the temporary port-7777 Toast process was stopped after verifying its
working directory. These are single observations, not latency distributions.

For context only, the short Toast observation advanced from 52 CPU seconds at
79 seconds uptime to 91 CPU seconds at 137 seconds uptime (39/58, about 67% of
one core). It also runs substantial background work. This includes the startup
probe and is not the requested ten-minute idle parity comparison.

The first three object probes produced different tick counts on the two live
worlds (Barn 71/201/15, Toast 382/1036/70). These were different server ages and
ports, so this is inconclusive. Following Fable's conditional recommendation, no
protected-builtin optimization was added on that evidence. A controlled same-state
per-job comparison and the remaining high CPU/latency are outstanding. Restoring
Toast suspended-task bytecode remains outside this change.
