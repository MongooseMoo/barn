# Experiment: escalate at attempt 2 instead of 63 (2026-09-01)

Question: the `escalateAfterAttempts = 63` setting was tuned on the 07-27
harness before its three fidelity fixes (protected builtins, huh fallback,
PROMOTE_NUMBERS). Is a low threshold still a server-serializing disaster on
the faithful harness, or does it already help?

Setup: `engine/mongoose_real_bench_test.go`, `mongoose.db.new`, default mix
(look 35 / say 30 / i 10 / @who 10 / home 15), warmup 2 s, measure 8 s,
GOMAXPROCS=32, Ryzen 9 5950X, single run per side (noise ±10%). Branch side is
master `32664a2` with one constant changed in `engine/task_runtime.go`.

## Results

| side | players | goodput | abort | p50 | p99 | max | allocs/op | bytes/op |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| k=63 (master) | 1 | 129/s | 0% | 8.1 ms | 22.7 ms | 70.9 ms | 27,946 | 6.96 MB |
| k=63 (master) | 16 | 87/s | 65.8% | 9.3 ms | 3,670 ms | 4,150 ms | 233,058 | 60.6 MB |
| k=2 | 1 | 129/s | 0% | 8.2 ms | 23.3 ms | 73.8 ms | 27,882 | 6.95 MB |
| k=2 | 16 | 109/s | 42.9% | 157 ms | 363 ms | 393 ms | 78,053 | 20.7 MB |

Per shape at 16 players (ok / attempts / avg):

| shape | k=63 | k=2 |
|---|---|---|
| look | 247 / 28,487 / 192 ms | 316 / 22,612 / 163 ms |
| say | 226 / 14,923 / 107 ms | 263 / 24,507 / 208 ms |
| inventory | 77 / 451 / 5 ms | 95 / 1,319 / 27 ms |
| @who | 71 / 33,647 / 812 ms | 89 / 6,743 / 178 ms |
| home | 109 / 684 / 8 ms | 134 / 2,220 / 37 ms |

## Reading

- Tail collapses 10x (p99 3.7 s → 363 ms) and allocation per command drops
  3x, because tasks stop burning 60 doomed re-executions.
- p50 rises 8 → 157 ms: the escalated attempt holds the GLOBAL commit gate
  exclusively for its whole re-execution, so every task queues behind every
  escalated task. Goodput at 16 players (109/s) is still BELOW 1 player
  (129/s). The server is serialized, just less wastefully.
- Abort stays ~43% because a task may lose twice before escalating and almost
  every task loses at least once: the write set of Mongoose commands overlaps
  on a handful of global objects.

Conclusion: the retry policy is the multicore bottleneck, and the fix is
per-object write-intent locks on retry (plus early write-time abort), not a
lower threshold on the global gate. Do not ship k=2 as is. See
`plans/barn-beat-toast-2026-09-01.md` §3 A1.

Raw output: scratchpad `base.txt` / `k2.txt` (not committed; reproduce with
`BARN_MONGOOSE_BENCH=1 BARN_MONGOOSE_PLAYERS=1,16 go test ./engine -run
TestMongooseRealWorkload -count=1 -v`).
