# Application profile diagnostics

These profiles identify allocation and CPU costs in the real Mongoose workload.
They do not establish application speedup or a normalized allocation reduction.
The separately recorded paired application measurements remain authoritative for
throughput; their initial result is inconclusive with concerning losses.

## Profile scope and provenance

The archived profiles use `engine/TestMongooseRealWorkload`, GOMAXPROCS=4,
one player, 1s warmup, 5s measurement, default command mix, repair and number
promotion enabled. The disposable database SHA256 is
`489FF8D14884392DCFBA6CD88D407E53A2140C03D4F0CC4B1B19CF2AFD8A031E`.
Baseline production source is `3c0f510`; candidate includes unique `7e0363d`,
file-read `0e5fdba`, and finalization `6b72d0a`. Neither rejected conversion
preallocation experiment is included. The unchanged workload evaluator blob is
`3ac942cb3d5589b7a588dfb354ec0d0ac0627045`.

Profile build IDs identify baseline binary built at 22:18:29 and candidate at
22:33:17 on September 26, 2026. Saved executable SHA256 values:

- Baseline: `5C61B43174045818574EF37E031166122D22CA0449B4469FE6C67A78D3A45F11`
- Candidate: `2D9E7AD676284C3A417861D195D65A0419EC7BABA029137EDA173EA9F3CA48F1`

Memory profiles are sampled cumulative allocation and retained heap snapshots
after an explicit `runtime.GC()` near the end of the test body. They include
database loading, runtime startup, warmup, measured commands, and the idle probe;
deferred teardown has not finished at the heap snapshot. They are not profiles
of only the five-second measurement window. The loader's database structures
remain visible. CPU profiling starts after database loading and player selection,
but before runtime construction; deferred StopCPUProfile means startup, warmup,
measurement, idle work, and runtime shutdown are included. The existing harness
comment describing only measurement windows is not an accurate scope statement.

All four raw logs end in PASS and report `failed=0 uncaught=0`. The instrumented
logs show different command counts and timings, and startup repair returned
different handle sets. Memory-run goodput was 184/s baseline and 84/s candidate;
CPU-run goodput was 70/s and 73/s. These figures are recorded to expose the
measurement mismatch, not to estimate regression or improvement. Background
startup/shutdown work also differs. No profile totals are normalized or treated
as comparable units of completed application work.

## What the profiles show

The retained heap is dominated by loaded database data. In the raw inuse tables,
`Database.resolvePropertyNames` accounts for 66.01% of baseline retained bytes
and 68.64% of candidate retained bytes. Total sampled retained heap is 1593.93MB
and 1498.74MB respectively; those totals are separate process snapshots, not a
measured pool memory saving. The same loader path and ObjectBuilder.SetProperty
also dominate cumulative allocated bytes. Cumulative totals include the loader
and must not be divided by only the timed command counts.

The finalization allocation mechanism is visible in application execution.
Baseline `collectPendingFinalizationsFromFrame.func1` has 248.01MB flat sampled
allocation (2.14% of the full allocation profile), and
`collectDirectFinalizationRoots` has 126.57MB (1.09%). The corresponding candidate
frame callback no longer appears as an allocating node in the focused table;
direct root collection has 2.65MB flat (0.028%). This is consistent with removing
those temporary allocations, while the unequal work and startup counts prevent
deriving an application-wide percentage improvement from these amounts.

CPU profiles still show execution, lookups, and memory management as substantial
costs. VM.executeLoop cumulatively accounts for 79.14% of baseline samples and
80.07% of candidate samples. The finalization frame collector has cumulative
3.61s (5.26%) baseline and 1.74s (2.98%) candidate. These describe each sample's
composition, not a per-command speedup. Candidate scratch reset and release
symbols are sampled, demonstrating that reuse/cleanup itself has a cost.

Neither profile's complete pool-symbol CPU table contains unique() or file_read()
symbols. Absence from sampled data does not prove zero calls or zero cost; this
application mix does not provide useful CPU attribution for those two pools.
Their focused experiments provide the relevant ownership, allocation, and
retention evidence.

The pool-related focused inuse snapshot contains 3.01MB of baseline samples and
no candidate samples. Sampling, explicit GC, runtime state, and different work
counts mean this is not proof that the candidate pools retain nothing. Per-item
limits and the controlled microbenchmark retention snapshots are documented in
the individual experiment records; there is no global retained-memory bound or
guaranteed pool lifetime.

## Artifacts and reproduction

`2026-09-26-pool-application-profile-evidence/` contains all four raw profiles and
logs, the exact parent profiling script, symbol-based CPU top/focus tables, and
alloc_space/inuse_space top/focus tables. Profiles contain embedded symbols;
these reports deliberately avoid source listings because the baseline debug
paths can resolve to subsequently modified source. MB/GB units above follow
pprof's raw display.

From the repository root, set `$evidence` to that directory. Offline analysis
requires only Go's pprof tool and the archived profiles, for example:

```powershell
go tool pprof -top -nodecount=25 -alloc_space "$evidence/mongoose-baseline.mem"
go tool pprof -top -nodecount=25 -inuse_space "$evidence/mongoose-candidate-mem.pprof"
go tool pprof -top -nodecount=30 "$evidence/mongoose-candidate-cpu.pprof"
go tool pprof -top -nodecount=30 '-focus=Unique|FileRead|Finalization|Finalizer|Pending.*Root|pending.*Root' "$evidence/mongoose-candidate-cpu.pprof"
go tool pprof -top -nodecount=0 -nodefraction=0 -edgefraction=0 '-show=Unique|FileRead|Finalization|Finalizer|Pending.*Root|pending.*Root|finalizationScratch' "$evidence/mongoose-candidate-cpu.pprof"
```

To regenerate profiles, compile each named source's `./engine` tests into
`.tmp/engine-baseline.exe` and `.tmp/engine-candidate.exe`, supply the same
disposable `.tmp/mongoose.db`, and run the archived `profile-application.ps1`
from a fresh campaign working directory. Its three cases generate candidate
memory plus baseline/candidate CPU profiles in private runtime directories.
It intentionally preserves the executed script, which expects fresh profile
directories and does not generate the earlier baseline memory profile.
For the baseline memory case use the same environment and private-directory
procedure, select `engine-baseline.exe`, set BARN_MONGOOSE_CPUPROFILE to empty,
and set BARN_MONGOOSE_MEMPROFILE to the desired absolute baseline output path;
invoke `-test.run=^TestMongooseRealWorkload$ -test.v -test.timeout=180s` and capture
the native exit code immediately. Executables and the database are not archived.
Do not run profiling concurrently with benchmark confirmation.
