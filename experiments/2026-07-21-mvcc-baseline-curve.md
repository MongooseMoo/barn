# Experiment: MVCC Redesign — Phase 0 Master Baseline Curve (2026-07-21)

Plan: `plans/mvcc-concurrency-redesign-2026-07-21.md` Phase 0.
Branch: `mvcc-concurrency-redesign` (off master @ cf3687d).
Harness: `scheduler/mvcc_workload_bench_test.go` (`TestMVCCBaselineCurve`).
Machine: GOMAXPROCS=32 (16C/32T). Go build, non-race.

## What this measures (and why it can't lie)

A mongoose-shaped workload of simulated players driven through the **real** VM /
txn / commit / retry machinery. One goroutine per active player calls
`s.runTask` synchronously — matching production's per-connection hot path
(`input_processor.go:669`), NOT the batch scheduler. Reports **absolute goodput
(committed commands/sec)**, abort rate (`CommitRetries/CommitAttempts`), p50/p99/max
latency, and allocs+bytes/op + GC count (MemStats delta). It is deliberately NOT
the serial/pool ratio (the June metric trap).

Command shapes exercise the exact machinery each later phase changes:
- `look` (40%): wide+deep read — iterate room `.contents`, read each occupant's
  `name` + inherited `desc` (falls through to shared root). **Phase 2** (alias reads).
- `say` (30%): read-only contents+names, no write.
- `move` (20% / 15%): the **real `move()` builtin** → `markLiveStoreMutated` →
  coarse EXCLUSIVE commit today. **Phase 3** (move off stop-the-world).
- `stamp` (7% / 5%): per-player `last_activity` scalar write.
- `build` (3%): `create()`+`recycle()` (stop-the-world topology op).
- `churn` (0% realistic / 10% stress): write a property on the shared **root**
  generic — conflicts with every in-flight `look`/`say` scan dep. **Phase 4**
  (precise ancestry deps).

Fixture: root generic (owns `desc`,`churn`), room generic (`look`,`announce`,`tick`),
player generic (`last_activity`), R rooms, P players (Zipfian placement),
`objsPerRoom=6` inert objects Zipfian across rooms. Warm-up 500ms + timed 2s,
**median of 5 repeats**.

## Reproduce

```bash
BARN_MVCC_BENCH=1 BARN_MVCC_WARMUP=500ms BARN_MVCC_MEASURE=2s BARN_MVCC_REPEATS=5 \
  go test ./scheduler/ -run TestMVCCBaselineCurve -v -timeout 20m
```
Knobs: `BARN_MVCC_PLAYERS`, `BARN_MVCC_ROOMS` (comma lists), `_WARMUP`, `_MEASURE`,
`_REPEATS`. Raw run saved alongside this file's commit.

## Results — realistic mix (look40/say30/move20/stamp7/build3), 0 churn

| players | rooms | goodput/s | abort% |    p50   |   p99   |   max   | allocs/op | bytes/op | GCs |
|--------:|------:|----------:|-------:|---------:|--------:|--------:|----------:|---------:|----:|
| 1  | 4  | 33,597 | 0.00 | 0.00us | 537.5us |  3.96ms | 160.4 | 28,033 | 279 |
| 4  | 4  | 40,500 | 0.00 | 0.00us |  1.01ms | 14.20ms | 165.2 | 28,706 | 174 |
| 16 | 4  | 37,155 | 0.00 | 0.00us |  2.67ms | 16.85ms | 191.7 | 32,680 | 195 |
| 32 | 4  | 32,361 | 0.00 | 546.7us | 4.84ms | 14.98ms | 220.6 | 37,448 | 128 |
| 1  | 16 | 28,975 | 0.00 | 0.00us | 545.0us |  1.89ms | 188.5 | 33,530 | 151 |
| 4  | 16 | 40,476 | 0.00 | 0.00us |  1.01ms | 15.75ms | 192.6 | 34,183 | 252 |
| 16 | 16 | **41,920** | 0.00 | 0.00us | 2.69ms | 7.42ms | 210.3 | 36,747 | 140 |
| 32 | 16 | 30,746 | 0.00 | 540.5us | 5.75ms | 17.18ms | 230.9 | 39,791 | 198 |
| 1  | 64 | 13,689 | 0.00 | 0.00us | 710.9us |  5.03ms | 285.3 | 51,464 | 151 |
| 4  | 64 | 23,521 | 0.00 | 0.00us |  1.51ms | 14.58ms | 286.6 | 51,656 | 196 |
| 16 | 64 | 23,188 | 0.00 | 528.7us | 4.22ms |  8.27ms | 304.1 | 54,460 | 275 |
| 32 | 64 | 26,835 | 0.00 | 538.9us | 6.40ms | 17.74ms | 309.4 | 55,166 | 275 |

## Results — churn-stress mix (look40/say30/move15/stamp5/churn10)

| players | rooms | goodput/s | abort% |    p50   |   p99   |   max   | allocs/op | bytes/op | GCs |
|--------:|------:|----------:|-------:|---------:|--------:|--------:|----------:|---------:|----:|
| 1  | 4  | 32,552 |  0.00 | 0.00us | 538.4us |  7.40ms | 156.1 | 27,635 | 649 |
| 4  | 4  | 40,849 |  5.58 | 0.00us | 635.3us | 15.79ms | 162.0 | 28,595 | 818 |
| 16 | 4  | 34,783 | 19.99 | 519.1us | 2.16ms | 11.17ms | 187.9 | 32,707 | 917 |
| 32 | 4  | 28,083 | 25.68 | 583.4us | 5.38ms | 14.12ms | 219.4 | 37,955 | 664 |
| 1  | 16 | 26,743 |  0.00 | 0.00us | 543.9us |  2.23ms | 185.4 | 33,335 | 613 |
| 4  | 16 | 36,217 |  5.23 | 0.00us | 1.00ms | 13.69ms | 188.3 | 33,874 | 820 |
| 16 | 16 | 33,846 | 18.63 | 505.7us | 2.16ms | 16.61ms | 207.7 | 36,918 | 801 |
| 32 | 16 | 31,672 | 25.10 | 542.7us | 5.38ms | 17.24ms | 234.1 | 41,002 | 671 |
| 1  | 64 | 16,415 |  0.00 | 0.00us | 935.1us |  7.38ms | 276.5 | 50,221 | 476 |
| 4  | 64 | 25,322 |  3.41 | 0.00us | 1.14ms | 16.88ms | 288.2 | 52,174 | 619 |
| 16 | 64 | 27,929 | 15.08 | 0.00us | 3.09ms | 17.07ms | 299.9 | 54,161 | 634 |
| 32 | 64 | 28,147 | 23.53 | 539.1us | 6.46ms | 22.09ms | 302.9 | 54,638 | 568 |

## Interpretation — the baseline confirms all three target diseases

1. **No parallel scaling (Phase 3 target).** Realistic goodput peaks ~42k/s at
   16 players and **degrades to ~31k at 32 players** — the extra 16 hardware threads
   buy nothing, and abort is **0%**, so the ceiling is not data conflict. It is the
   global EXCLUSIVE lock the 20% `move()` + 3% `build` commands take
   (`markLiveStoreMutated`). Single-thread realistic cost ≈ 34.5 us/command
   (1p/16r = 28,975/s); peak/serial scaling is only ~1.45x on a 32-thread box.

2. **Allocation wall (Phase 2 target).** ~160–310 allocs and **28–55 KB allocated
   per command**, scaling with room occupancy (more occupants = more objects
   deep-cloned on read). GC runs hundreds of times per 2s window; the latency tail
   (p99 up to 6.5ms, max to 22ms) tracks GC pauses, not compute.

3. **False-conflict abort storm (Phase 4 target).** Adding 10% ancestor-property
   writes drives abort rate to **25%+ at 32 players**, and goodput falls as players
   rise (e.g. 4r: 40.8k@4p → 28.1k@32p). Realistic mix at 0% churn stays 0% abort —
   proving the aborts are specifically the shared-ancestor scan-dep false conflicts.

## Success criteria for later phases (measured against THIS curve)

- **Phase 2**: allocs/op and bytes/op fall sharply (esp. the 64-room rows); GC count
  drops; serial (1p) latency improves; goodput improves-or-neutral.
- **Phase 3**: realistic goodput scales PAST 16 players toward 32; the 32p rows stop
  regressing below the 16p peak.
- **Phase 4**: churn-stress abort% collapses toward the realistic 0%, and its 32p
  goodput stops falling below its 4p value — while a define/delete/chparent STILL
  conflicts (anomaly guard).

Predicted post-Phase-2..4 ceiling: GC no longer the wall at default GOGC, no global
exclusive lock on any common command, conflicts only on true contention.
