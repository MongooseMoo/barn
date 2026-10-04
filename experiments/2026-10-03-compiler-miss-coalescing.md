# Compiler cold-miss coordination (#363)

## Registered comparison

Base: `9414dbbb2f21bc03096e494da25c9ab38208522a` (normal merge of #367).

The issue is duplicate parsing/lowering when callers simultaneously request one
uncached source key. This experiment evaluates that compiler workload; it does
not measure server throughput or claim a deployment-wide speedup.

Before editing production code, build the baseline compiler test binary with the
new benchmark workloads. Use the same benchmark source for the candidate binary.
Keep Go 1.24.6, Linux amd64, `GOMAXPROCS=4`, and CPU affinity 8-11 fixed. Serialize
measurements, alternate baseline/candidate order across 12 paired samples, use
`-test.benchmem -test.benchtime=200ms -test.count=1`, and compare with benchstat.
WSL does not expose CPU frequency controls; timing conclusions are limited to
these paired runs, and allocation/work counts are the primary cost evidence.

Registered workloads:

- Shared cold burst: 16 callers, 256 increment statements plus initialization
  and return, one compiler, one precomputed source key.
- Independent cold keys: the same 16-call burst with distinct initial values and
  source keys, to detect serialization across unrelated compilations.
- Single cold caller: the same 258-line source, to measure coordination overhead.
- Sequential and parallel warm hits: prepopulate the compiler cache and reuse
  its source key; both paths must retain zero allocations.
- Holdouts: shared bursts of 8 callers/512 statements and 32 callers/128
  statements, withheld from the primary keep decision.

The keep decision requires fewer allocations/bytes and a significant timing
improvement in the shared cold burst, zero warm allocations, and no material
regression in warm or independent-key work. Treat small timing differences on
this unpinned-frequency host as uncertain. Reject coordination if its measured
cost outweighs savings. Capture separate CPU profiles before/after for attribution,
and compare a digest of compiled corpus output to guard the workload.

Correctness coverage must include a shared-key burst, independent blocked loaders,
diagnostic completion/retry and caller-owned diagnostic slices, registry isolation,
bounded LRU eviction, and cleanup after panic. No conformance baseline rerun is
needed; require the complete repository CI gate on the published candidate.

## Results

Keep the implementation. The shared-key primary workload and both holdouts save
substantial time, bytes, and allocations. Distinct-key time is statistically
unchanged. Warm hits remain allocation-free; their small timing increases are
reported below rather than treated as a speedup or hidden by a mixed-workload
aggregate. These are compiler microbenchmarks on the registered WSL host.

Medians from 12 samples per binary (benchstat, time comparisons):

| Workload | Baseline | Candidate | Change | p |
| --- | ---: | ---: | ---: | ---: |
| Shared16 | 3.126 ms | 1.158 ms | -62.96% | 0.000 |
| Distinct16 | 5.462 ms | 5.479 ms | +0.31% | 0.932 |
| Single | 1.102 ms | 1.077 ms | -2.27% | 0.000 |
| Warm sequential | 35.56 ns | 36.14 ns | +1.60% | 0.000 |
| Warm parallel | 51.41 ns | 52.02 ns | +1.19% | 0.045 |
| HoldoutShared8 | 5.869 ms | 2.172 ms | -63.00% | 0.000 |
| HoldoutShared32 | 1.841 ms | 0.633 ms | -65.61% | 0.000 |

The warm differences are under 1 ns per lookup. The warm lookup body is unchanged;
the compiler entry point now contains a cold-path branch. Without frequency
controls, attribution of these small timing changes remains uncertain. They do
not outweigh the registered cold-burst savings.

| Workload | Baseline bytes/op | Candidate bytes/op | Baseline allocs/op | Candidate allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Shared16 | 1681.6 KiB | 198.2 KiB | 29,427 | 3,427 |
| Distinct16 | 3.031 MiB | 3.035 MiB | 54,340 | 54,370 |
| Single | 194.6 KiB | 195.3 KiB | 3,407 | 3,411 |
| Warm sequential | 0 | 0 | 0 | 0 |
| Warm parallel | 0 | 0 | 0 | 0 |
| HoldoutShared8 | 2869.4 KiB | 384.4 KiB | 50,586 | 6,752 |
| HoldoutShared32 | 983.0 KiB | 104.3 KiB | 17,416 | 1,773 |

Coordination adds four allocations to a single cold request. Shared16 saves
88.22% of bytes and 88.35% of allocations. Holdout byte savings are 86.60% and
89.39%; allocation savings are 86.65% and 89.82%. All byte/allocation differences
have p=0.000 except identical zero-allocation warm samples (p=1.000).

The baseline shared-burst CPU profile attributes 90.68% cumulative CPU time to
`CompileMOOWithKey`, with parsing and lowering prominent (56.22% and 32.54%
cumulative). The candidate profile still attributes its work to parsing/lowering,
but has 3.88 CPU-sample seconds over 3.62 wall seconds versus the baseline's
10.94 over 3.32, while completing 3,024 versus 762 profiled benchmark iterations.
Profiles are separate attribution runs, not substitutes for the paired timings;
cumulative percentages overlap and must not be added. The controlled cache tests
prove that joined callers receive one owner's completed program, while unrelated
keys can own unfinished compilations concurrently.

Baseline and candidate corpus digests (bytecode, constants, locals, line info,
builtin layout/slots, and source) both equal:

```text
31bc9af117211dde83c2fab75450a5305c00ae7a63afccde66769da1ed1895f3
```

The shared-key public API regression failed in all eight baseline runs. The
candidate passed `go test -race ./compiler -count=10 -timeout=240s`, including
diagnostic retry/storage isolation, independent keys, registry isolation, LRU
eviction, and real owner/waiter panic cleanup.

## Reproduction

Build baseline and candidate test binaries with identical
`compile_miss_bench_test.go` and `compile_miss_test.go` files (copy those two files
to the baseline checkout). The new private cache-state tests apply only to the
candidate. Then run each sample with the registered alternating order:

```sh
go test -c ./compiler -o compiler.test
taskset -c 8-11 env GOMAXPROCS=4 ./compiler.test -test.run '^$' \
  -test.bench '^BenchmarkCompileCache(Cold|Warm)/(Shared16|Distinct16|Single|Parallel=.*)$' \
  -test.benchmem -test.benchtime=200ms -test.count=1
taskset -c 8-11 env GOMAXPROCS=4 ./compiler.test -test.run '^$' \
  -test.bench '^BenchmarkCompileCacheCold/HoldoutShared(8|32)$' \
  -test.benchmem -test.benchtime=200ms -test.count=1
benchstat baseline.txt candidate.txt
```

For separate profiles, use `-test.bench '^BenchmarkCompileCacheCold/Shared16$'`,
`-test.benchtime=3s`, and `-test.cpuprofile=compiler.cpu`, then
`go tool pprof -top -cum -nodecount=12 compiler.test compiler.cpu`.
Benchstat version: `golang.org/x/perf v0.0.0-20260825160852-19be9d8e6c70`.
Host: AMD Ryzen 9 5950X, Go 1.24.6 linux/amd64, WSL Debian.
Raw paired outputs, profiles, and digests are retained in the local delivery
artifacts directory `C:/Users/Q/AppData/Local/Temp/barn-363-tools`.
