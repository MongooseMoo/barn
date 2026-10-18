# Originating tasks and typed terminal errors, October 17, 2026

Master `c840f0eb` adds the maintenance pause/input repair and WAIF isolation
coverage. Local merge `f214d4bf` joins it with the gameplay diagnostics branch
without conflicts. Existing checkpoints, profiles and failed cohorts were
preserved. This work adds diagnostic evidence; it does not establish a speed or
memory improvement, diagnose current production lag, or change the live server.

## Finalized error records

With `BARN_RUNTIME_DIAGNOSTICS=1` and debug logging, `terminal task exception`
records identify the original task before its database error handler runs.
Fields are originating task ID, root receiver/verb, native error enum, typed
`reason`, `intrinsic_eval` and `committed`. Messages, exception values, arguments
and source are excluded. Existing runtime phase records and profiles can be
joined using that ID within the same process.

The record is emitted after retry, cancellation and suspension decisions.
Caught errors and discarded attempts do not produce records. A terminal commit
failure remains visible with `committed=false`; an application exception whose
writes published has `committed=true`. Intrinsic eval failures are included,
although they bypass the database error handler.

The reason belongs to the error/result, with no reusable VM state:

| Reason | Evidence |
|---|---|
| `frame_depth` | Actual VM frame admission guard |
| `tick_budget` | Actual VM tick guard |
| `seconds_budget` | Actual VM deadline guard |
| `unknown` | No typed guard origin, including explicit `raise(E_MAXREC)` |

Builtin and lifecycle bridges, finally, retry snapshots and checkpoint restore
carry this metadata. Catching/replacing an error does not transfer its reason
to a new exception. A backward-compatible optional field in Barn-private frame
metadata preserves pending errors across checkpoint restart; legacy frames use
`unknown`. Malformed reason/code pairs fail validation. MOO-visible exception
tuples and native error codes are unchanged.

Cleanup can replace an exception while Barn retains a different outward error
code. That preexisting behavior is unchanged: a guard reason that cannot
describe the outward code becomes `unknown`. Real recycle-continuation tests
exercise both ordinary execution and resume, first reproducing the mismatched
metadata and then verifying normalization without changing code/value/flow.

`barn_logs -runtime [-task ID] [-json]` reports a bounded census by enum, reason,
intrinsic eval and commit outcome. All nineteen native error enums are accepted,
including explicitly raised `E_NONE`. Missing records are not evidence of no
exceptions: collection may be disabled, old or outside the capture interval.

## Evidence collected before the final combined checks

The unchanged task identity witness SHA-256 is
`4e17856aa1f2f6ac6e69e009c4f05d6fa8fb0b11bf64796039acd6c98c415fc5`.
Fresh canonical Debian Toast and same-session capability admission:

```text
2 passed, 1 warning in 8.85s
ORACLE_EXIT=0
```

Source inspection additionally shows native task ID and task-local state
persist through handler suspension, queueing, resume and checkpointing. Those
extended paths were inspected, not newly exercised by a managed witness.
Barn's separate handler-task identity mismatch remains open. No builtin ID alias
or lifecycle handoff is introduced here.

The clean merged baseline passed full Go tests and eleven admission race cases.
Its separately preserved executable SHA-256 is
`daea4b23b6d67f64ba5139b9ee3250c2248b8611c8e200684c620fff8691852d`.
A managed run on that binary stopped before capability admission:

```text
did not accept connections on port 43259 within 30.0s
1 error in 37.22s
CANDIDATE_EXIT=1
```

The repaired harness retained the failed directory and both startup logs.
Its initial failed working DB had already been removed by rollback; independently
sealed input copies remain available. This failure supplies no task-ID verdict.

Read-only probes of the same startup Git command produced a five-second timeout,
then a 14.67-second dirty result, then a 1.35-second result. The variable timings
and absent profile manifest do not prove where the original server stopped.
They expose an independent problem: optional provenance used unbounded Git calls
and treated operational diff failures as a dirty checkout.

The helper now shares a two-second budget across HEAD and diff, bounds inherited
output-pipe waits, validates HEAD format and reports dirty state `unknown` on
timeout or operational failure. Exit one alone means dirty. Focused tests
failed against the old helper and passed against the repair:

```text
internal/app: PASS; GREEN_TEST_EXIT=0
shared deadline: 2.003s
inherited output pipe: 109.6ms
```

Focused diagnostic tests also pass, including actual guards, manual spoof text,
catch/replacement, finally, nested builtin propagation, retry and actual
checkpoint write/load/restore/resume. The terminal producer regression covers
handled/caught errors, explicit `E_NONE`, intrinsic eval, resumed tasks, failed
commit and two attempted exceptions with only one terminal record.

```text
terminal producer: PASS; TEST_EXIT=0
recycle replacement execute/resume: PASS; TEST_EXIT=0
```

The first combined seven-package race run passed engine, VM, task, format,
metrics and the log consumer, but failed the Git helper fixtures. Copied race
test executables inherited the race detector's one-second exit delay, exceeding
the helper tests' deadlines. The test fixture now preserves existing `GORACE`
options and appends `atexit_sleep_ms=0` for its child processes; the already
initialized parent race detector is unaffected. Focused race validation:

```text
internal/app: PASS; FOCUSED_RACE_TEST_EXIT=0
```

Raw evidence lives under `.tmp/master-refresh-20261017/`: `native-identity`,
`native-semantics`, `merge-checks`, `barn-identity`, `startup-provenance`,
`typed-reason` and `terminal-checks`. Failed fixture/probe attempts remain beside
their corrected runs. Combined verification and a fresh managed diagnostic
capture are recorded separately below when available.

## Fresh managed diagnostic

After the corrective source freeze, full Go tests passed again (`TEST_EXIT=0`).
The ordinary Go 1.27.1 Linux/amd64 build uses the existing default PGO profile,
CGO and GOAMD64=v1, without the Elle build tag. Binary SHA-256:
`a33d5fe47578056b7e9ed01f84706a49f617af87a9247028896389a69ce8c863`.
All thirty frozen inputs, including twenty-five changed Go files, matched before
and after build and managed execution. Explicit source seals bind the build;
embedded VCS fields are absent in `go version -m`.

The unchanged witness and same-session admission reached the known semantic
mismatch, with no startup error:

```text
Admission: 4/4 probes passed
1 failed, 1 passed, 1 warning in 11.02s
Expected: [1,1,1,1,1,1,1,1,1]
Actual:   [1,1,0,1,1,1,1,1,1]
```

The durable server log contains exactly one new terminal record:

```text
task_id=29 this=9 verb=launch error=E_INVARG
reason=unknown intrinsic_eval=false committed=true
malformed log records=0
```

This is the runtime's originating task identity before database handler
dispatch, collected without exception payloads. The MOO witness still says the
handler's task ID differs from the fork's ID. Separate handler runtime phase
records have ID 30, but the unchanged witness exports equality predicates, not
numeric IDs; the safe summary does not fabricate an order-based numeric join.
Successful readiness in this session does not prove the cause of the previous
startup failure. Logs and target profile were written outside the managed
temporary directory and survived ordinary assertion-failure cleanup.

Evidence: `.tmp/master-refresh-20261017/final-managed/`, especially the build
manifest and `run-01/safe-origin-summary.json`; original failures are retained
separately. Race/Elle and the offline workload capture are additional gates.

## Combined verification

Final frozen source is recorded by `final-checks/source-before-02.json` (thirty
inputs, twenty-five changed Go files). Full Go and the seven-package race rerun:

```text
go test ./... -count=1: TEST_EXIT=0
race engine/vm/task/db-format/metrics/barn_logs/internal-app: TEST_EXIT=0
```

The updated pinned Elle driver ran seeds 412, 395 and 396 for forty rounds,
then all six deliberate faults. Actual saved verdicts and independent audit:

```text
6 healthy histories: true, exit 0
6 fault append histories: false, exit 1, each with a nonempty anomaly witness
DRIVER_EXIT=0
AUDIT_EXIT=0
```

All twenty required coverage keys are nonzero in each healthy manifest.
For seed 395, actual committed path counts are parallel 380, exclusive 948,
exclusive gate 847, hot renewal 2, verb memo 451, suspension 321, fork 14,
native exception 48 and read-only 4. All eleven operation/invariant counts are
also positive. All healthy histories have `fail=0` and `info=0`; successful
transaction outcomes in the list-append histories number 386/391/390;
the rw-register histories have 322 each.
These are isolation coverage counts, not performance measurements.

CLI pin `9c2a18b3719f573dab06ae05c955c9c9d5225673`, JAR SHA-256
`e53120b925291586d00c4b99cb1f8ac31439d372b2c932da05634d2f6ded5373`
and explicit Java 21.0.12.1 match the recorded toolchain. Raw histories,
manifests, checker output and anomaly witnesses are retained in
`/root/.cache/barn-master-refresh-final-elle-20261017-01` and described by
`final-checks/elle-audit-02.json`. After the CPU capture stopped,
`final-checks/elle-evidence-02.tar.gz` was finalized: 138,396 bytes, SHA-256
`ce9d347aee65018630cde8256ba0d193823dc11e5789eb7459cd5e7fda614373`.
All 67 regular archive members (3,904,632 uncompressed bytes) independently
match the raw tree's sizes and hashes. The separate final verification manifest
and archive audit retain this evidence; raw Linux files remain available.

## Actual offline magic data

Exactly one new cohort used the pinned Mongoose checkpoint, real sound catalog,
compatible Toast bootstrap and the standard Barn binary above. Other test and
checker processes had stopped; compression ran after capture. Runtime diagnostic
logging and a requested fifteen-second Go CPU profile were enabled. This is a
failed-domain diagnostic cohort, not a completed gameplay performance baseline.

```text
zero_focus_lumos: expected ordinary refusal was absent
productive_pairs_verified=0
owned shutdown: exit 0, forced=false
source/ownership/checkpoint gates: 10/10 true
DB/WAIF checkpoint pair: verified
```

The whole log has 52 terminal records: fifty `E_INVARG/unknown`, one
`E_INVIND/unknown` and one `E_MAXREC/seconds_budget`. The latter is originating
task **1788854378**, root **#277800:weave**, with `committed=true`. Thus this
particular task reached the VM's seconds guard. The action log identifies the
failed control as `weave lumos with 0 focus` but does not export its task ID;
there is no explicit action-ID join, so the report does not infer one from
timing or the actor's latest issue.

Offline Go tools parsed all three profiles, verified expected sample types and
nonzero samples, and produced flat/cumulative/tag reports. All eleven base and
two exact-task commands exited zero. Separate validation preserves the original
collector/final manifests byte-for-byte: their `go_profiles_validated=false`
field remains historical, with offline validation recorded in the safe summary.

```text
CPU profile: 15.02s wall, 23.01s sampled process CPU (153.15%, multiple threads)
task 1788854378: 9.86s sampled CPU
cloneVerbForReadTxn beneath that exact task: 5.88s cumulative CPU
```

The exact task filter is `moo.task=^1788854378$`. Across the entire profile,
`cloneVerbForReadTxn` has 5.90s cumulative CPU (25.64%); `VM.executeLoop` has
15.59s (67.75%) and `gcBgMarkWorker` 4.94s (21.47%). These stacks overlap and
must not be added. Largest flat rows are `runtime.memmove` 1.52s (6.61%),
`spanClass.sizeclass` 1.03s (4.48%), `scanObjectsSmall` 0.97s (4.22%),
`cloneVerbForReadTxn` 0.91s (3.95%) and `memclrNoHeapPointers` 0.80s (3.48%).
Root labels identify #277800:weave 9.87s, #1007:s_run 5.58s and unlabeled
work 7.07s; they identify task roots, not currently executing nested MOO verbs.

The separate action interval was 10.721908333s, with 18.56s of whole-Barn CPU
including background threads and in-process diagnostics, excluding the external
Python collector. RSS was 3,446,276 to 3,750,280 KiB; the observer peak across
85 samples was 3,751,152 KiB. Natural (`gc=0`) heap profiles estimated retained
bytes at 708,920,212 and 747,016,918. Cumulative sampled allocation totals were
3,604,023,534 and 11,653,641,720 bytes; those profiles reflect GC history and
include loading, so they are not exclusive spell allocations. Expvar NumGC
advanced from 5 to 8. Neither these snapshots nor this single failed cohort
establish a leak, speedup or memory reduction.

The measured next target is verb-read cloning: inspect its callers, snapshot
validation and returned-object mutation contracts before changing copying.
The large cloning subtree and allocation activity justify an experiment; they
do not justify exposing mutable shared definitions or removing validation.
An explicit action/task join and a completed magic cohort remain necessary
for a gameplay performance comparison.

Evidence is `.tmp/barn-magic-terminal-20261017-01/analysis/safe-summary.json`
(SHA-256 `be880e8544a7811adeb06eb52f05ad020fc5f0d8196b6cb1816a26c9df68ebd2`),
with raw commands/exits and profiles beside it. Published DB SHA-256:
`e084045ce89a7bcc772c64359f63c55657c67466b2f08ee9d4fdcc2cbd800fd8`;
WAIF sidecar: `dd880411b3e229d8c08552f8ca0d014a2f3cc607a69f1780573c1a24ee79b5c3`.
