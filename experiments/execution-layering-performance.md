# Execution ownership measurements

These measurements are incomplete acceptance evidence. They compare merge base
`11a17d8cf5a0e8aa497c377933689a8e70778a4e` with `93a6ec61` and its subsequent
VM flag-packing patch. Both worktrees use Windows/amd64, Go 1.27.1,
Ryzen 5950X, GOMAXPROCS=4. The shared host is not isolated or frequency-pinned.
Keep the earlier context-only measurements separate from these ownership results.

## Interpreter

Command, sequentially in the two worktrees:

```text
go test ./vm -run '^$' -bench '^BenchmarkVM/(int_arith_1M|builtin_abs_200k)$' -benchmem -benchtime=300ms -count=10
```

The first base/head pair (`execution-layering-ownership-{base,head}-vm.txt`)
showed +64 B/op in both cases, with unchanged allocation counts (19 and 26).
Reflection measured VM/profile/shared state/executable task sizes:

```text
base:   632 / 64 / 832 / 832 bytes
head:   640 / 72 / 824 / 840 bytes
packed: 624 / 72 / 824 / 840 bytes
```

Packing existing VM flags removes padding without changing the typed profiling
interface. The packed-head/repeated-base pair restores median 16344 and 16552
B/op. Timing is indistinguishable (p=.481/.684, n=10); see
`execution-layering-packed-vm-benchstat.txt`. This does not establish a speedup.

## Continuations

Command:

```text
go test ./engine -run '^$' -bench '^BenchmarkThreadedContinuations$' -benchmem -benchtime=300ms -count=10
```

The first base/head pair is retained in ownership-{base,head}-continuations.
Two later captures were truncated by the tool output limit; their filenames
end in `-incomplete.txt`, and they are excluded from comparison. The repeated
complete pair is `ownership-base-repeat-continuations` versus
`packed-continuations`. Its benchstat file reports 1p/writes +6.70% (p=.003),
with other timing comparisons p=.052/.143/.631. The 1p allocation counts
also differed slightly. This remains a concern, not performance acceptance.

Existing CPU and heap profiling then measured only the 1p write cohort:

```text
go test ./engine -run '^$' -bench '^BenchmarkThreadedContinuations$/^players=1$/^writes=true$' -benchtime=5s -count=1 -cpuprofile=<absolute path> -memprofile=<absolute path> -o <absolute path>
packed: 26146 iterations, 234450 ns/op, 72485 B/op, 550 allocs/op
base:   25422 iterations, 239658 ns/op, 72489 B/op, 551 allocs/op
```

Profiles and matching binaries are retained locally under `.tmp/`, named
`{base,packed}-single-{cpu,alloc}.pprof` and `{base,packed}-single.test.exe`.
These instrumented single runs do not replace repeated timing measurements.
CPU samples concentrate in executeLoop (40.11% base / 41.60% packed cumulative).
Allocation profiles include the existing scheduler scans and unfinished-list
copies. The benchmark's ready loop calls ProcessReadyTasks then Gosched when
idle (engine/continuation_bench_test.go), so polling work is not fixed per command.
No source change is justified by these profiles alone.

An unprofiled follow-up reused those same binaries in base/head/head/base order,
five 1s samples per batch (ten per revision). All four batches are retained as
`execution-layering-single-write-{base,packed}-{a,b}.txt`; the combined summary
is `execution-layering-single-write-benchstat.txt`. The command used quoted
`-test.run=^$`, `-test.bench=^BenchmarkThreadedContinuations$/^players=1$/^writes=true$`,
`-test.benchtime=1s`, `-test.count=5`, and `-test.benchmem` arguments.
The first unquoted PowerShell invocation rejected `-test`; it ran no benchmark.
Across the complete follow-up, timing p=.165, bytes p=.306 and allocations p=.252
are inconclusive. The earlier short-run slowdown is not reproduced consistently;
host/scheduling variability remains material. This bounds the performance claim
to the measured VM allocation correction, not general nonregression or speedup.

An earlier profile selector matched both 1p and 16p because `players=1` was
not end-anchored. Its timing is not the single-cohort comparison above. The first
packed relative-path profile outputs could not be located; no conclusion uses
them. Absolute-path single-cohort outputs above were read successfully.

## Real Mongoose workload

Same source inputs on both runs:

```text
checkpoint: C:/Users/Q/code/barn/mongoose.db.new
SHA256: EE84F1E848A4A2D22C2BCCE1340D19F9CEC7BDB752B416355D6AD4831E8F4F79
catalog: C:/Users/Q/code/barn-266-sound/sound.sqlite
SHA256: 5AC3896689041E2AB4EF1C176E61941F83793444D8A2AFB25DF454E38AD91034
catalog bytes: 256438272; read-only quick_check: ok; AssetFiles rows: 128552
```

The main checkout's sound catalog was locked; it was not copied or modified.
The existing offline catalog above is older, not a claim of production currency.
The harness copies it into each isolated worktree and loads the checkpoint
without writing that source. Both runs use BARN_MONGOOSE_BENCH=1, PLAYERS=1,4,16,
GOGC=400, WARMUP=2s, MEASURE=8s, COMPLETION_TIMEOUT=10s, REPAIR=1, PROMOTE=1.
No CPU/MOO profiling or debug log collection is enabled for these initial runs.

```text
go test ./engine -run '^TestMongooseRealWorkload$' -count=1 -v -timeout=5m -o <absolute path>
```

Base output is retained in `execution-layering-base-real-workload.txt`.
It fails: 4p has two home E_NACC results; 16p has E_NACC and one unsettled
home deadline with movement/physics frames and family blockers. Throughput
from failed cohorts is not valid acceptance evidence. The packed head run is in
`execution-layering-packed-real-workload.txt`: 1p completed 416/416, 4p completed
357/359 (two E_NACC failures with the same player/verb frames), and 16p completed
524/524 with no unsettled commands. Its test process exits successfully despite
the 4p native failures. This is not a clean all-command acceptance result or a
speedup comparison. Both source hashes above were unchanged after both runs.
Binary SHA256 values, retained under `.tmp/{base,packed}-real.test.exe`:

```text
base:   ABB13419C65AA01010FB9E92F5920A768DDD612B3DD2A30891E0C02E68007C3E
packed: 242E95F4497F014416CAB8188451CE718ABC5EC1E02FB15D35E62223D18F5CF8
```

The failures are preserved, not suppressed or repaired in this refactor.

## Isolation

`execution-layering-ownership-elle-summary.txt` records inspected CI artifact
verdicts and coverage for head 93a6ec61 (before flag packing). Three seeds have
true append/register verdicts; ten fault cases have false verdicts; required
paths have nonzero counts. Raw histories, manifests and anomaly witnesses remain
in the downloaded archive identified there. This does not cover later commits.
