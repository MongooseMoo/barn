# Experiment: MVCC Redesign — Phase 1 Re-land (2026-07-21)

Plan: `plans/mvcc-concurrency-redesign-2026-07-21.md` Phase 1.
Branch: `mvcc-concurrency-redesign`.
Re-lands two verified June wins the 2026-06-25 master rebase dropped:
- `604dd09` sharded readTS floor registry (A1, was c09bcae).
- `2609bd8` GOGC=400 default (was 8d47c44).

Harness: `TestMVCCBaselineCurve`. The harness inherits `GOGC` from the env
(it does not call `debug.SetGCPercent`), so the server's shipping GOGC=400
default is reflected by running the harness with `GOGC=400`. All runs
`-count=1` (test result caching serves stale output when only an env var
changes — an identical-to-the-digit table is the tell).

## Single-variable GOGC result (HEAD code fixed; only GOGC env differs)

Median of 5, 2s window, rooms=16.

| scenario / cell | GOGC=100 g/s | GOGC=400 g/s | speedup | GCs 100→400 |
|-----------------|-------------:|-------------:|--------:|:-----------:|
| realistic  1p   | 22,239 | 32,237 | 1.45x | 296 → 41 |
| realistic 16p   | 33,612 | 54,401 | 1.62x | 294 → 31 |
| realistic 32p   | 30,976 | 48,998 | 1.58x | 189 → 72 |
| churn-stress  1p| 25,120 | 32,718 | 1.30x | 609 → 166 |
| churn-stress 16p| 38,126 | 63,606 | 1.67x | 958 → 243 |
| churn-stress 32p| 32,853 | 59,418 | 1.81x | 738 → 216 |

The **GC count collapse (up to ~9.5x fewer collections)** is the robust,
near-deterministic signal that GOGC=400 took effect and that GC frequency —
not compute — was the binding constraint on this alloc-heavy workload (~34KB
allocated per command; see the Phase 0 baseline). Goodput rises ~1.5–1.8x on
both scenarios, on both the serial (1p) and parallel axes. This directly
matches the plan's thesis and the original 8d47c44 sweep (100→4.56x,
400→8.50x parallel speedup on the verb-call path).

## Floor-shard (A1) contribution on this workload

The floor-shard's proven win was on the COMMIT-DOMINATED disjoint-write
microbench (`TestConcurrencyCommitDominatedDisjoint`: ~1.17x→1.98x 32-worker).
On this READ-HEAVY mongoose mix (0% abort realistic; reads dominate), the
register/floor/deregister path is a smaller fraction, and the harness's
run-to-run variance at a 2s window (~±20% cell-to-cell) is wider than the
expected floor-shard delta here. So no goodput win is claimed for A1 on this
mix — it is re-landed because it is proven on its target path (re-verified:
`db/store -race` green incl. history-GC correctness and floor-multiset tests)
and because Phases 3–4 will stress the commit path it accelerates.

## Gates

- `go build ./...` clean.
- `go test ./db/store -race` green (full package, incl. history-GC/disjoint/floor).
- Server boot logs `"GC target set (default)" percent=400`.
- Phase 1 exit: both wins on the branch; harness re-measured; recorded here.

## Caveat / instrument note

Harness variance at a 2s window is ~±20% between neighboring cells (warm-state
and thermal drift). Robust signals: GC count (near-deterministic) and large
(>1.4x) goodput deltas. Small goodput deltas (<1.2x) are noise here — widen the
window or interleave A/B on identical machine state before trusting them. This
is why Phase 1's claim rests on the GC-count collapse, not on hairline goodput.
