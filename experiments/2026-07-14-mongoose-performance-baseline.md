# Mongoose deployment performance baseline

## Fixed workload

- Fixture SHA-256: `b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69`
- Toast executable: `/root/src/toaststunt-mongoose/build-release/moo`
- Toast executable SHA-256: `a748a93644fe2b973cc85dfed902454a0a56c8b368afdc8104161ec76154d098`
- Runner: `scripts/benchmark-mongoose.ps1`
- Client: existing `cmd/moo_client`, with optional timestamped events and a
  deterministic maximum-duration boundary
- Login input: exactly three newline-separated commands supplied through the
  uncommitted `MONGOOSE_LOGIN_SCRIPT` environment variable
- Post-login commands, identical on both engines: `look`, `west`, `@who`, task
  and connection liveness query, then `dump_database()`
- Client timing: 3000 ms banner wait, 2500 ms between commands, 15 seconds idle
  timeout, and 40 seconds maximum duration
- Resource sample: after a fixed 180-second post-workload settle period
- Checkpoint completion: observed from creation of the disposable
  `<run.db>.new`, not from the command reply

Raw run artifacts stay under `.tmp/mongoose-convergence/` and are not committed
because the client transcript is deployment-local. The summary below contains
no login commands or credentials.

## Pinned WSL Mongoose Toast baseline

Run directory: `.tmp/mongoose-convergence/perf-toast-20260714-03`

| Metric | Toast baseline |
|---|---:|
| Database load to listening | 6392 ms |
| Connect to first banner | 143 ms |
| PROXY send to first output | 3 ms |
| Complete login from PROXY send | 5015 ms |
| Startup `@who` response | 2 ms |
| Explicit `look` render | 3 ms |
| Open-exit movement response | 6 ms |
| Liveness query response | 1 ms |
| Checkpoint command reply | 2 ms |
| Checkpoint file completion | 9429 ms |
| Post-settle CPU | 3.6% |
| Post-settle RSS | 311640064 bytes |

The liveness query returned `{3, 1}`: three queued tasks and one connected
player. The Toast transcript independently reported checkpoint completion in
9.37 seconds, consistent with the runner's 9.429-second file observation.

## Barn acceptance thresholds

These thresholds were fixed from the Toast baseline before measuring Barn:

- database load to listening: at most 12784 ms (2x Toast);
- connect to first banner: at most 500 ms (Toast plus cross-OS scheduling slack);
- complete login from PROXY send: at most 10030 ms (2x Toast);
- PROXY, startup command, `look`, movement, and liveness response: at most
  100 ms each (Toast is 1-6 ms; the floor avoids making scheduler jitter the
  acceptance boundary);
- checkpoint file completion: at most 18858 ms (2x Toast);
- post-settle CPU: at most 7.2% (2x Toast);
- post-settle RSS: at most 467460096 bytes (1.5x Toast);
- liveness: one connected player, a nonnegative queued-task count, successful
  `look` and west movement, and a completed checkpoint file.

Barn must be measured with the unchanged runner, fixture, commands, client
timings, and settle period. A failed threshold names the first performance
target; it does not authorize widening to another metric.
