# Diagnose real-workload failures before optimizing

Use the existing collectors together: structured exception frames explain which
operation failed, MOO profiles identify expensive verbs, Go profiles identify
implementation work, and scheduling diagnostics explain waits. Preserve failed
runs. A faster cohort with unsettled commands is invalid performance evidence.

## Establish the error and fixture

Record source revision, binary hash, MOO and sound-catalog hashes, selected
players, repair settings, command mix, worker count, GC settings and deadlines.
Use a real sound catalog through `BARN_MONGOOSE_SOUND_DB`; an empty catalog makes
`say` exercise the error handler instead of sound playback.

Read the saved exception frames before assuming a permission mismatch. The
workload harness retains the raised error's stack after the live task stack has
unwound: verb location, receiver, player, effective programmer and line. Timeout
frames are captured before cancellation, with task state, execution lease and
family blockers. These summaries omit source, arguments and runtime values.
Resolve the indicated code from the same immutable dump using `moodb code`.

`E_PERM` and `E_NACC` are different contracts. For example, movement rejected by
a missing/falsy destination `accept` hook can raise `E_NACC` for a non-wizard.
Determine the actual hook and effective programmer before changing property or
builtin checks. Confirm uncertain behavior through managed WSL Toast conformance
with capability admission; see [mongoose-conformance.md](mongoose-conformance.md).

## In-process command harness

The harness drives normal command dispatch and waits for terminal output
acknowledgement. Run Go tests from the repository root with `./engine`; a prebuilt
test executable must run from `engine/`. Its file sandbox switches to the repository
root. Use an isolated worktree: setup copies the catalog into `files/sqlite/`.
No source checkpoint is opened for writing.

For a diagnostic run, set these alongside the ordinary workload knobs:

```powershell
$env:BARN_MONGOOSE_BENCH = '1'
$env:BARN_MONGOOSE_DB = 'C:\absolute\checkpoint.db'
$env:BARN_MONGOOSE_SOUND_DB = 'C:\absolute\sound.sqlite'
$env:BARN_MONGOOSE_PLAYERS = '1,4,16'
$env:BARN_MONGOOSE_GOGC = '400'
$env:BARN_MONGOOSE_LOG_DIR = 'C:\new-evidence\logs'
$env:BARN_MONGOOSE_MOOPROFILE = 'C:\new-evidence\moo'
go test ./engine -run '^TestMongooseRealWorkload$' -count=1 -v
```

Create the evidence directory first and use a new prefix for each run. The MOO
collector is the same as `/debug/pprof/moo`; it writes `moo-players1.pprof`,
`moo-players4.pprof`, etc. Each profile covers its measurement window and command
drain, excluding loading, startup and warmup. Profile output refuses to overwrite
an existing file. An invalid cohort still retains its profile and failures.

```powershell
go tool pprof -top -cum -sample_index=ticks C:\new-evidence\moo-players16.pprof
go tool pprof -top -cum -sample_index=wall C:\new-evidence\moo-players16.pprof
go tool pprof -tags C:\new-evidence\moo-players16.pprof
go tool pprof -top -tagfocus='moo.verb=home' C:\new-evidence\moo-players16.pprof
go tool pprof -top -tagfocus='moo.task=^123$' C:\new-evidence\moo-players16.pprof
```

For a separate Go CPU run, set `BARN_MONGOOSE_CPUPROFILE` to an output path.
Its interval excludes database loading but includes runtime startup, warmup,
measurement, drain, idle and teardown. Inspect flat and cumulative samples and
`moo.verb`/`moo.task` labels; root labels are not the currently executing nested
verb. `BARN_MONGOOSE_MEMPROFILE` is an end-of-run heap snapshot. Use
`-inuse_space` for retained memory and `-alloc_space` for churn; the latter also
includes loading. Keep collection settings explicit and don't present an
instrumented diagnostic run as an uninstrumented performance comparison.
Anchor numeric task IDs with `^` and `$` when filtering labels; a bare numeric
filter can be interpreted as a numeric range instead of an exact label value.

## Running server

Find the actual private debug listener in the run log. The existing
`scripts/capture-mongoose-debug.ps1` saves expvar, goroutine stacks, recent logs,
Go CPU flat/cumulative reports and labels, and MOO ticks/wall profiles and labels.
It defaults to `-Profile Both`; choose `Go` or `MOO` for a single collector. Go
and MOO are captured sequentially, so the two profiles cover different windows.

```powershell
./scripts/capture-mongoose-debug.ps1 -RunDir <run> -DebugUrl http://127.0.0.1:<port> -Seconds 10
go run ./cmd/barn_logs -dir <log-directory> -level warn
```

`barn_logs` exit 1 means errors were found, not that capture failed. Its original
structured tracebacks can include source lines; keep raw logs local and inspect
only relevant records before sharing. The metadata tracer documented in
[TRACING.md](TRACING.md) omits payloads and supports verb filters.

Read `slow task slice` ticks and `gate_held`, external completion-to-dispatch
`queue_wait` and `ready_to_vm`, conflict census, GC/finalization statistics, and
goroutine stacks before adding instrumentation. CPU profiles omit off-CPU waits.
MOO wall samples describe active execution intervals, not suspension/admission
latency or pure CPU time. A leased running timeout needs execution/lock evidence;
a queued or suspended timeout needs readiness/completion evidence.

Before proposing a fix, report the exact failure frames and relevant profile
rows, identify the evidence still missing, and choose a deterministic regression
for the suspected cause. Keep native application errors, benchmark fixture faults,
engine faults and terminal-completion failures distinguishable.
