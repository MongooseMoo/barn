# unique() scaling test flake

## Goal
Make `TestBuiltinUniqueDistinctScalarsDoesNotScaleQuadratically` (builtins/lists_unique_test.go) stop failing under machine load while still failing on a quadratic unique().

## Evidence
CI job 136970430910 (PR 1087), step `go test ./... -count=1`:
`lists_unique_test.go:83: unique() scales quadratically: 2,500 values took 348.259µs, 10,000 took 18.202111ms`; test wall time 0.38s, builtins package 86.7s. All 12 runners busy (load ~20 on 8 cores).

## Cause (from the code)
- builtins/lists.go:448 `builtinUnique` buckets by `uniqueKey` and only calls `Equal` inside a bucket; distinct ints each get their own bucket, so the work is linear (map insert per element).
- Old test (lists_unique_test.go:53-85) measures 3 samples of size 2,500, THEN 3 samples of size 10,000, keeps each min, and fails if large > 8*small + 10ms.
- The two sizes are measured in separate time windows. 348µs at 2,500 means ~1.4ms is expected at 10,000; all three 10,000 samples took >=18ms, i.e. the process was starved (descheduled / GC assist) for the whole 10,000 window but not the 2,500 window. Min-of-3 only removes a disturbance that hits some samples; it cannot remove one that spans the whole block, and the ratio then compares a quiet window to a loaded one.
- 4x sizes give only 4x (linear) vs 16x (quadratic) separation, so the threshold (8x) sits 2x from each side; a 10ms allowance is the only slack.

## New check
- Windows `time.Now` granularity on the workstation measured ~0.5ms: 1,000-value calls timed as `0s` in 7 of 10 samples. A per-call min at small sizes can be 0, so a ratio check must time enough work per sample.
- New test: sample A = 16 calls on 1,000 distinct ints, sample B = 1 call on 16,000 (equal element count). Alternate A and B for up to 20 rounds with `runtime.GC()` before each, keep the fastest of each, pass as soon as bestB <= 4 * bestA. Linear gives ~1x (measured 0.9-2.0x per sample, fastest ~1.0x); quadratic gives ~16x. No fixed time allowance.
- Alternating puts both sizes under the same load; a starved window stretches both, and the min needs only one quiet sample of each.

## Quadratic mutation (temporary, reverted)
`key := uniqueKey(elem, i)` -> `key := uniqueBucketKey{}` in builtins/lists.go (every value in one bucket = all-pairs scan).
Result: FAIL `fastest of 20 rounds, 16 calls on 1000 values took 36.7477ms, one call on 16000 values took 573.3611ms (limit 4x)` (15.6x), 14.2s.
Unmutated: PASS 5/5, 0.02-0.03s each.

## First rewrite failed idle on Windows (3/150)
Equal-work version (16 x 1,000 vs 1 x 16,000, per-sample ~2-4ms) failed 3 of 150 idle runs: `16 calls on 1000 values took 0s` / `514µs`. Cause: Windows `time.Now` ticks coarsely (seen 0.5ms here, and 0s for ~2ms of work, consistent with the 15.6ms default timer). Samples of a few ms are below the clock's resolution.
Per-element cost with long batches (512,000 elements per sample, best of 7): n=250 130ns, 1,000 135, 4,000 142, 16,000 155, 64,000 279 (64,000 exceeds the largest uniqueScratch pool class, 16,384, so it allocates a fresh map). So 1,000 vs 16,000 is ~1.15x when linear; keep both sizes inside the pool classes.

## Final check (builtins/lists_unique_test.go)
- Each sample = 256,000 elements of work: 256 calls on 1,000 distinct ints, or 16 calls on 16,000 (~35-40ms idle on the workstation; long enough for a coarse clock).
- Alternate small/large for up to 10 rounds, `runtime.GC()` before each, keep the fastest of each; pass once bestLarge <= 4 * bestSmall. A large sample stops early once it passes 4 * bestSmall (bounds the failing path).
- Quadratic mutation (same `uniqueBucketKey{}` change, reverted): FAIL `fastest of 10 rounds over 256000 elements, 1000-value calls took 630.8496ms, 16000-value calls took 2.531827s (limit 4x)` (large sample stopped at the limit), 36.6s.

## Windows runs (32 logical CPUs; precompiled test binaries, -test.count=50 each)
- Idle (CPU 60%): new 150/150 pass; old 50/50 pass.
- Loaded (3 burners x 96 allocating goroutines, CPU 99%): old 100/100 pass, new 100/100 pass. Windows load did not reproduce the old failure.

## WSL Debian runs (the CI runners' distro, 8 CPUs; linux test binaries, CGO_ENABLED=0)
- Ambient (load avg 18.7-22.5 from the runners): old 100/100 pass, new 100/100 pass.
- Plus 2 burners (load 19.7-24.9): old 99/100 (1 fail: `2,500 values took 375.035µs, 10,000 took 25.253475ms`), new 100/100.
- Plus 3 burners (load 28-35): old 274/300 (26 fail, e.g. `2,500 values took 393.024µs, 10,000 took 15.487212ms`, same shape as CI), new 300/300.
Totals under load in WSL: old 26 fails / 400, new 0 / 400.

## Gates
gofmt clean, `go vet ./builtins` ok, `go test ./builtins -count=1` ok (48.4s, Windows).

## State
Committed on fix/flake-unique-scaling-test (local only; lead publishes).

## Next step
Push and PR (lead). Results reported to team-lead; nothing left on this branch.
