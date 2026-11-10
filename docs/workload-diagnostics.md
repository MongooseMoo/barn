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

## TCP player-command harness

Use `scripts/bench_players.py` when measuring decoded input to first socket output.
It logs in real roster players and runs the native look/say/inventory/who/home
mix against a disposable world. Supply the real sound catalog; its source and
copy hashes are saved with the checkpoint hashes in `run.json`.

```powershell
$env:BARN_RUNTIME_DIAGNOSTICS = '1'
uv run --project C:\Users\Q\src\lambdamoo-db-py python scripts/bench_players.py --engine barn --players 1,4,16 --db C:\fixtures\mongoose.db --sound C:\fixtures\sound.sqlite --barn-exe C:\build\barn.exe --out C:\new-evidence --keep-db
```

The diagnostic opt-in also selects debug logging for Barn. Read the command
records in `logs/` with `barn_logs -runtime -command ID`. The harness's terminal
latencies include an out-of-band completion probe and its scheduling delay;
use the command's `first_output` stage for time to first response. Inspect native
tracebacks and broken connections before accepting any throughput comparison.
The harness creates private accounts offline using the documented Mongoose
Toast build; it does not need live player credentials.

That bootstrap loads and saves the whole checkpoint through Toast. It now
requires current `lambdamoo-db-py` in the Python environment and uses its reader
to compare saved task identities, deadlines and native VM function IDs before replacing the
input copy. A mismatch aborts preparation and retains the original copy, rejected
`.boot` output and `bootstrap-schedule.json`. Supplying an existing login avoids
this bootstrap. An old installed reader without `read_v17_header` is rejected.

Toast is not schedule preserving for far-future queued tasks: the Mongoose task
1787475465 changed from Unix seconds 109290153959993 to 416145977. A reduced
stock-Toast checkpoint/restart probe also ran a future fork immediately. Do not
use bootstrap-based runs as evidence for the original checkpoint's scheduling
or as a clean Barn/Toast comparison. Preserve both checkpoint hashes and task
headers when investigating this discrepancy.

## In-process command harness

The harness drives normal command dispatch and waits for terminal output
acknowledgement. Run Go tests from the repository root with `./engine`; a prebuilt
test executable must run from `engine/`. Its file sandbox switches to the repository
root. Use an isolated worktree: setup copies the catalog into `files/sqlite/`.
No source checkpoint is opened for writing.

The harness restores queued and suspended tasks after preparing its host and
boot caches, before starting dispatch, using the same runtime loaders as the
server. It reports restored/input counts. Native restore warnings remain in
the JSON log. Unsupported or invalid suspended continuations now fail startup
and the harness before dispatch, rather than accepting a partial world. The old
loader rejected all 49 suspended/interrupted tasks in the November 5 checkpoint
with empty bytecode frames. The native importer now translates saved positions,
locals and control state without replaying source effects; focused verification
restored all 49 and preserved all 1080 queued deadlines. Native builtin
continuations, pending native-finally reasons and incompatible byte-string
iteration remain explicit unsupported errors. Detailed diagnostics go to JSON without
duplicating every record on stderr, leaving cohort results readable.

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

`shared_commit_gate` records successful ordinary and task-row-only shared
acquisitions for runtime tasks and standalone hooks. Records carry the task,
hook and command identities available at that entry; standalone system hooks
can have task ID zero and no command ID. `commit_gate` records exclusive
acquisitions. Admitted work charges both waits to admission gate-wait accounting
rather than execution service; sweep hooks have no admission scope. Canceled
acquisitions are not included in these shared records.
Earlier captures omitted task-row-only shared waits from counters and admission
accounting; their absence is not proof that a yielded command did not wait.
For example, a restored Mongoose `say` traversed 174 slices and the Go trace
attributed about 321ms to task-row commit blocking while the old task records
showed only 5.5ms of exclusive acquisition. Keep trace-derived blocking separate
from active VM wall durations and sampled CPU.

On Windows with Go 1.27.1, do not enable mutex profiling for a capture. The Go
runtime's asynchronous preemption can deadlock during GC while mutex profiling
is enabled ([upstream issue](https://github.com/golang/go/issues/80982)). Use
Go 1.27.2 or newer for mutex collection, or execution traces and block profiles
on this toolchain. The real-world harness rejects enabled mutex profiling on
Windows Go 1.27.0/1. Preserve stalled captures;
their missing terminal results are failures, not latency samples. For an external
Delve attachment, use a persistent interactive terminal or an explicit command
file that captures stacks and detaches; an attachment with immediate EOF cannot
establish the requested stacks.

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

## Player input and first response

With `BARN_RUNTIME_DIAGNOSTICS=1` and debug logging, physical transport inputs
receive a process-local `command_id` when decoding completes, before input-buffer
backpressure. `player command` records contain IDs, classification and elapsed
nanoseconds from that receipt. They exclude input lines and output payloads.
Initial banners and forced input do not create physical-input measurements.
The player field is the identity observed at receipt; queued login transitions
can change which player eventually executes that input.

The reader continues accepting input while its existing per-connection worker
executes events in order. At most 16 transport events are outstanding with a
5 MiB aggregate payload budget; one additional decoded input can wait at that
budget. A transport message already larger than the byte budget enters alone,
preserving existing framing policy. Forced input does not wait on this budget.
The shared dispatcher is never blocked waiting for a connection worker.

| Record stage | Meaning |
| --- | --- |
| `received` | One physical input has been decoded. |
| `dequeued` | Its connection worker begins processing; elapsed time includes shared queue, connection queue and input-buffer waits. |
| `classified` | Identifies a command, login, held input, out-of-band input or delivery to `read()`. |
| `task_started` | A root command task is registered, before execution. Its elapsed time also includes admission, parsing/lookup and compilation; it is not pure admission wait. |
| `first_committed_output` | The first published notification/direct response for the initiating connection reaches the output path. This is not a successful write. |
| `first_output` | The first attributed response successfully completes its transport write. `write_ns` records time in the output path, including output serialization when measured there. |
| `output_failed` | An attributed output attempt failed; no payload or error message is retained. |
| `pending` | No successful response write or completion after 100 ms. Includes admission state and up to 64 unfinished task summaries, plus an omitted count. Emitted once while still pending. |
| `input_finished` | The first input-processing invocation returned or the input was discarded; root tasks can remain suspended. |
| `task_finished` | A correlated root task/error handler reached terminal state. |
| `finished` | Input processing and its correlated tasks ended, with explicit committed/written output flags. |

Command IDs follow `do_command`, the selected command verb, intrinsic eval,
their suspensions/retries, and database error/timeout handlers. Runtime phase,
threaded-lifecycle and terminal-exception records from those tasks include the
same ID. Discarded attempts cannot satisfy first output. Buffered notifications
retain their identity until a successful flush, which can happen after `finished`.
Prefixes/suffixes, unrelated room output, and output to a replacement connection
cannot satisfy the response measurement. Forked background tasks have independent
contexts; output produced only by a fork is not attributed to its parent's input.
Login, held input and delivery to `read()` are classified separately and do not
imply command-response coverage. Programming input is also classified separately;
its direct compile/error replies carry the input identity.

```powershell
go run ./cmd/barn_logs -dir <logs> -runtime
go run ./cmd/barn_logs -dir <logs> -runtime -command <command_id> -json
```

The summary aggregates fixed stage buckets and preserves errors from the whole
log. It distinguishes other input from classified commands. A finished-without-
write count describes the state at completion; a buffered response can be written
later. Missing output, unfinished input and absent diagnostics are not zero
latency. A task filter excludes input records without a task ID; prefer a command
filter for the full path. Stage timestamps share one receipt origin and must not
be added together. They are wall durations, not CPU time.

The pending task/admission fields are independent context snapshots, not an atomic
snapshot or evidence that a listed task caused delay. Pair them with task-filtered
runtime records, gate evidence, conflict census and profiles. Decoded-input timing
excludes network/client delays and bytes waiting before the next transport read.
A successful server write establishes neither client rendering nor audible receipt.
Detailed collection adds work; do not use differently instrumented runs as a speed
comparison.

## Contention diagnostics in the binary

The running server and the in-process harness report the same contention
data from one implementation (`engine/diag_report.go`). Two levels:

- Always on: exclusive commit gate holds per holder and shared gate waits.
  `GET /debug/commit-gate` serves them as JSON; each hold row is the task's
  root verb, the cause (the frame that called an irreversible builtin, or an
  escalation reason), the builtin (`exec`, `sqlite_execute`, ...), count,
  total/max hold, VM ticks run under the gate, acquire wait (total, max,
  histogram), the queue depth ahead at each acquire and who was ahead (the
  exclusive owner and queued exclusive requests by label, `shared` for shared
  owners or requests). `Shared` counts commits that took the shared gate and
  the subset that queued behind an exclusive holder or request, with their
  waits. Hot-read renewals made, refused and declined are counted per
  property row of the conflict census.
- With `CONFLICT_CENSUS = 1` (harness: `BARN_MONGOOSE_STALE_READS=1`), the
  conflict census adds three tables and each task keeps a timeline:
  - `Paths`: losses per (key, loser root verb by its defining object, read
    site): attempt number, where validation was lost (`final`, `suspend`,
    `boundary`, `coarse`), for boundary losses the builtin and calling
    `verb@line` with its slice tick, keyed map reads made whole and by which
    path (`boundary`, `live-mutation`, `flush`, `rename`), losses after a
    hot-read renewal of the slot, the winning writers with how they published,
    and the wall time from the read to the loss (histogram) split into
    exclusive gate wait, shared gate wait and the rest.
  - `Verbs`: per root verb, slices, attempts, retries, committed vs wasted
    ticks, retries per finished task (histogram, max) and the first
    irreversible boundary of each attempt (mean tick, builtin and site).
  - `BuiltinCalls`: calls of boundary-checking builtins (the Irreversible
    descriptors, inline SQLite, create/recycle/chparent and the other coarse
    builtins) by calling `verb@line`. Threaded SQLite calls are not counted.
  - Task timelines: per task, each slice's start, background admission wait,
    external completion queue time and wall time, and each attempt's VM time,
    ticks, outcome (`commit`, `suspend`, `exception`, `lost`,
    `lost-at-boundary`, `lost-final`, `error`), first stale key, exclusive gate
    hold and wait, and shared gate wait (and the part behind an exclusive).
    `GET /debug/task-timelines` lists running tasks' timelines and the latest
    32 finished ones that took a second or more; such a task that is a player's
    command also logs one INFO `slow task timeline` record (background loops
    that suspend between slices are kept but not logged). Bounds: 64 slices (first 4 and latest),
    8 attempts per slice.

When the server stops it logs one INFO `diagnostics summary` record: headline
attributes and the text report in `report`. The harness prints the same
report per player level and, for every failed or unsettled command, its MOO
error, traceback and (with tracking) timeline. All tables are bounded and
overflow into counted rows. Timeline and table recording cost time on every
transaction; keep tracking off for performance comparisons.
