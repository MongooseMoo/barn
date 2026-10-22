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
$env:BARN_RUNTIME_DIAGNOSTICS = '1'
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

## Runtime phases and retry costs

`/debug/vars` exposes `barn.runtime_phases` even with ordinary logging: a fixed
set of process-wide counters with count, total/max nanoseconds, MOO ticks and
disjoint duration buckets (upper bounds in `upper_ns`; the final bucket is
unbounded). Scrapes are approximate during concurrent updates. Compare snapshots
from the same process; totals include startup and all work since process start.
`barn.async_started`, `barn.async_completed` and `barn.async_completion_rejected`
count threaded operations and completions rejected by the suspension generation
or task state. They count executed workers, not failed transaction attempts.

For task-level evidence, start the process with `BARN_RUNTIME_DIAGNOSTICS=1` and
debug logging. The command harness enables debug logging when its log directory
is set. These opt-in records omit SQL text, parameters, result values, source and
arguments. Labels identify the root task receiver/verb, not its nested activation.
Detailed logging can be expensive; do not compare an instrumented candidate with
an uninstrumented control. Disable the environment setting for performance runs.

```powershell
go run ./cmd/barn_logs -dir C:\new-evidence\logs -runtime
go run ./cmd/barn_logs -dir C:\new-evidence\logs -runtime -task 7290
go run ./cmd/barn_logs -dir C:\new-evidence\logs -runtime -task 7290 -json
```

The summary streams the file into bounded buckets, ignores `-level` and `-n`,
retains malformed-record counts, and reports missing measurements explicitly.
`-json` in runtime mode emits a single JSON document. Errors anywhere in the
original log retain exit 1, including when a task filter excludes their records.
Histogram percentile columns are upper bounds, not exact quantiles. A capture
can start or end between a threaded operation's start and completion; mismatched
counts alone do not establish a lost worker.
The completion record is logged after publishing readiness and can appear after
a resume record from another goroutine. Log-line order is not a scheduling or
isolation witness; use the measured intervals and suspension generations.

| Phase | Interval |
| --- | --- |
| `slice` | Admitted invocation through cleanup; includes retries and waits. Ticks sum all VM attempts. |
| `vm_attempt` | One call to the VM, including nested calls, lock waits, inline external work and admission preemption. Ticks are instruction-budget charges, not CPU instructions. |
| `retry_waste` | The VM interval and ticks of an unpublished attempt actually discarded for retry. Subset of `vm_attempt`; excludes commit/restore overhead. |
| `input_admission` | Input waiting for admission, before a command task exists; task ID is zero. |
| `background_admission` | A selected continuation waiting for admission, including canceled requests. |
| `readmission` | A running VM lends its execution reservation and waits to reacquire it; this does not commit its transaction. |
| `commit_gate` | Acquisition of the exclusive commit gate for escalation or an irreversible boundary. |
| `sqlite_handle_wait` | Acquiring the per-handle operation slot for query/execute, including rejected closed handles. |
| `sqlite_work` | Query/execute and row conversion while owning the slot, excluding slot wait. Close and last-insert-ID are not included. |
| `async_work` | A committed threaded worker's operation, including its handle wait if any. |
| `resume_queue` | Accepted external completion becoming queued through scheduler selection and admission to its next slice. Covers threaded SQLite/sort/all_members/curl and exec. |
| `resume_to_vm` | Same readiness event through entry to the saved-VM launch path, once per resumed slice; adds lease/setup/checkpoint costs. |

These intervals overlap. Do not add them to obtain command latency, or describe
wall time as CPU time. Use Go CPU profiles for CPU work and execution traces for
off-CPU attribution. `retry_waste` measures VM work retried, not necessarily all
the cost caused by contention. Record `go test -trace=C:\new-evidence\go.trace`
to retain Go goroutine states with `moo.slice task=...` regions; use `go tool
trace -pprof=sync` or `-pprof=sched` rather than expanding a huge trace dump.

Timeout failures now include a locked snapshot of admission reservation,
physical execution, input fence, external readiness time and suspension
generation, plus aggregate admission stats. Other fields are sampled separately;
the whole timeout report is not one atomic global snapshot. A saved stack at
`sqlite_execute()` does not establish that SQL is still running. Pair readiness
with that task's threaded start/completion records and phase report.

The command harness no longer prints per-shape attempt/retry counts derived from
process-wide deltas: concurrent command windows counted the same other work
multiple times, and background work could contaminate even a one-player sample.
Use the cohort commit census or task-filtered records. `max` now means the true
maximum successful-command latency observed, including outside the percentile
reservoir; `p999` is printed separately. Failed/unsettled commands remain separate.

With diagnostics and debug logging enabled, `terminal task exception` records
identify the originating task before the database's error handler runs. They
contain its task ID, root receiver/verb, native error enum, `intrinsic_eval` and
`committed`. A false `committed` identifies a terminal commit failure rather than
an application exception whose writes published. Discarded retries, caught errors,
suspensions and canceled tasks do not produce terminal exception records.
Intrinsic eval exceptions do produce records, even though they bypass the
database handler. The handler currently runs as a separate Barn task; its
`task_id()` must not be used as the originating ID.

`reason` is typed metadata from the actual VM guard: `frame_depth`, `tick_budget`
or `seconds_budget`. `unknown` includes explicit `raise(E_MAXREC)`, unclassified
errors and old checkpoints without this metadata. Error messages cannot supply
a reason. Finally/suspension carries the pending error's reason; catching or
replacing that error does not transfer it to a new exception. This metadata is
not added to MOO-visible error tuples.

`barn_logs -runtime -task ID` and its JSON output include a fixed-vocabulary
terminal exception census, split by enum, reason, intrinsic eval and commit
outcome. Detailed records exclude messages, exception values, arguments and
source. A capture with no records cannot prove there were no exceptions: it may
predate this collector, start late or have diagnostics disabled. Correlate the
origin ID with task-filtered runtime records and profiles from the same process;
an actor's latest issue can belong to unrelated work.
