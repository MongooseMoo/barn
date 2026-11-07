# Barn system understanding and refactor execution plan

## Goal

Understand Barn across its layers, looking especially for resources whose
lifetime exceeds the work that needs them. Q's example is socket reading tied
to task execution. Correctness and beauty are independent reasons to refactor:
clear ownership, direct control flow, fewer special cases, less duplication and
representations that express their invariants. A speedup is optional; negative
performance impact is unacceptable. Performance measurement constrains a
change, but a measured bottleneck is not a prerequisite for structural work.
Q approved execution with "Make it so!" on November 7. The whole-system plan
below is active. Work is isolated on refactor/system-layering; preserve unrelated
work and report each bounded slice with its actual evidence.

## Owner cleanup analyst checkpoint

State: actual PR #638 published source review is complete; no actionable
defect found. The completed report is
reports/system-layering-owner-cleanup-analyst.md. Decisions and why: inspect
existing runtime/session and transaction owners directly, preserve the future
file-grep edits, and use PR CI rather than local broad tests. Surprises: the
standalone GC path replaces the transaction, making the late-bound release
necessary. Next step: parent checks the delivered report and remaining coverage
limits; conformance and isolation CI were pending at last inspection.

## Execution plan

### Outcome and authority

Leave Barn with direct, understandable ownership from input through execution,
publication, external effects and cleanup. Correctness repairs, simpler code
and better resource lifetimes are each valid outcomes. A structural improvement
does not have to produce a speedup. Every retained change must preserve or
improve performance on the affected paths; there is no accepted slowdown budget.

Q's "Make it so!" authorizes execution of this plan. Publication, reviews,
merge and deployment holds remain governed by Q's explicit instructions.
No deployment or live-world restart is implied.

The plan covers the whole production system. It does not prescribe rewriting
every package. Each bounded slice ends in delete, move, consolidate, rewrite
or keep, with a reason grounded in its callers and contracts. Keeping an already
clear owner is a completed disposition, not a reason to manufacture changes.

### Target ownership

| Existing owner | Responsibility that remains there |
| --- | --- |
| cmd and internal/app | Process composition, options, service startup and shutdown, tool entrypoints |
| server transport and Connection | Protocol I/O, accepted output order, connection identity, buffering/draining and close lifecycle |
| server input processor | Input classification, per-connection ordering, routing and command boundaries |
| command | Command parsing, matching and dispatch selection |
| internal/admission | Execution reservations and service accounting |
| engine scheduler | Readiness, scheduling lanes, dispatch slots and wakeups |
| engine runtime | Attempts, physical VM ownership, retry/publication orchestration and lifecycle settlement |
| task | MOO-visible task state, suspension generations, task-owned callbacks and observable completion state |
| vm | Frames, instruction execution, native continuations and retry execution snapshots |
| builtins | Builtin semantics, effect declarations, operation handles and host calls |
| db/store | World/task-row transactions, dependencies, validation, publication and version history |
| parser, compiler and bytecode | Source syntax, compilation, instruction format and compiled-program identity |
| ir, sourcekey and preposition | Compiler representation, source identity and shared language vocabulary |
| types | Value semantics, sharing, immutable containers and container metadata |
| engine finalization coordinator | Root ownership, sweep coordination and finalization lifecycle |
| db/format | Portable decoding/encoding and checkpoint artifact publication |
| config, profile and internal/listener | Validated options, managed profile evidence and listener descriptions |
| logging, metrics, trace and internal/mooprof | Observability formats and collection with accurate ownership/attribution |

Use these owners directly. Do not add an umbrella runtime facade, a parallel
task model, a general effect scheduler, duplicate DTOs, compatibility aliases
or a second implementation hidden behind a cleaner API. Internal functions or
files are created only for a demonstrated responsibility that lacks an owner.

### P0. Establish the execution basis

Start from refreshed remote master in an isolated worktree. Preserve this
checkout's unrelated dirt, experiments and other worktrees. Re-read applicable
AGENTS.md files and compare relevant changes since the research baseline.
Inventory the companion conformance repository and any already-open overlapping
work before choosing a branch or making a duplicate fix.

Freeze source, executable, configuration, database, sound-catalog and workload
identities. Establish usable private fixtures, runner access and credentials
without putting secrets in notes. Identify anything only Q can supply before
implementation depends on it; nothing may be left for Q after a merge.

Use existing workload diagnostics for one initial capture and existing native
workload programs for an uninstrumented baseline. Preserve failed/unsettled
cohorts. Separate application errors, fixture faults and engine faults before
interpreting measurements. Read docs/workload-diagnostics.md,
docs/mongoose-performance.md and docs/elle-isolation.md as the current runbooks.

Deliverable: the exact baseline/evidence locations, resource-lifetime table and
phase status in this living note. Baseline collection must not block an
otherwise understood mechanical cleanup that does not touch the measured path.

### P1. Close observable contracts and repair demonstrated defects

Resolve source-discovered uncertainty before changing the corresponding
behavior. Start with notify/no_flush, buffered-output bytes and return values,
overflow, boot/switch ordering and the meaning of command completion. Then
cover thread_mode, suspension/resume/kill, inline versus threaded external work,
exceptions with committed writes, checkpoint/restore and recycle hooks as the
later slices reach them.

For each uncertain MOO behavior, use the documented managed stock WSL Toast
oracle first, including capability_admission in focused sessions. Preserve
raw reported input, exact result/transcript, native errors and fixtures. Use a
test-owned driver when caller identity matters. Do not infer expected behavior
from Barn's existing unit tests or the currently inconsistent prose spec.

Create a minimal failing regression from each confirmed defect before repair.
Generic MOO behavioral tests belong to the companion conformance owner; keep
Barn-specific ownership, races and deterministic fault cases in Barn. Deliver
dependency changes first, with independent PRs against each default branch.
Update incorrect spec text with the established behavior. Keep semantic repairs
separate from subsequent structural motion so their effect remains reviewable.

Deliverable: settled expected results and regression locations for the active
slice, plus repaired defects in their present owner. An unknown is closed by
evidence, not by picking the implementation that looks most convenient.

### P2. Make task and execution lifetimes explicit

Active slice: engine/task_runtime.go, task_factory.go, call_verb.go, eval.go,
runtime.go, task rows and the task state/context/snapshot surfaces they use.
Follow all production callers, including CLI/database tools and host hooks.

Separate the existing responsibilities of attempt execution, attempt
resolution, ordered committed effects and task settlement. Keep one owner for
each cleanup action and remove duplicated handling once callers use it.
Represent the transitions through the existing typed task/result machinery;
do not invent another task-state enum or parallel outcome protocol.

Preserve the distinction between admission, scheduler slots, physical leases,
logical status, input receipts and terminal Done. Preserve necessary differences
between a resumable task, nested frames, synchronous host VMs and sweep-owned
hooks. Share genuinely identical mechanics; retain different semantics explicitly.

Cover successful return, native exception, failed commit, retry, cancellation,
panic, suspension, stale completion and callback re-entry. Completion callbacks
must follow physical/admission release and keep their results rooted. A saved
continuation must not run before its former owner has published/released it.
Use the production frame-installation helper in manually built VM fixtures.

Deliverable: an execution path readable as acquire, execute, resolve, hand off,
release and settle, with obsolete lifecycle paths removed. Validate with the
existing continuation-retry, effect-boundary, completion, provenance, task-row
and lifecycle regressions, extending them only for missing observable cases.

### P3. Give connection output its complete ownership boundary

Active slice: server/connection.go, transport.go, websocket_transport.go,
connection_manager.go, input processor/buffer, builtins/network.go,
connection.go and pending_effects.go, plus command diagnostics.

Preserve serial connection input processing and bounded reader read-ahead.
Make accepted output, pending byte ownership, transport draining and connection
closure distinct from VM execution and task completion. Connection is the
existing owner of the ordered output stream; physical draining belongs to its
transport lifecycle. Replace the present mixed buffering/write responsibility
directly rather than layering another unrelated output queue over it.

The output contract established in P1 determines acceptance, capacity, return
values, overflow and close behavior. Preserve ordered notification/boot/switch
effects, framing captured at call time, recipient resolution rules and partial
write handling. Do not silently turn a rejectable notify into an always-successful
enqueue. Resolve any transactional acceptance/result race before selecting the
final implementation; do not compensate with a new global effect sequencer.

Keep the store's exclusive gate and task's execution ownership out of waits for
physical socket draining once the corresponding final publication/effect
acceptance contract permits release. Treat mid-slice irreversible boundaries
separately: subsequent computation may still need their protection.

Use deterministic blocked/slow-reader tests for TCP and WebSocket, concurrent
send/boot/switch/close, NoFlush/NoNewline, overflow, partial failure, cancellation,
shutdown and diagnostics. Assert unrelated tasks can publish while a peer is
slow, along with ordered accepted output and correct terminal acknowledgement.
Measure connection/idle overhead and throughput, not only blocked-write recovery.

Deliverable: connection-owned accepted output with correct backpressure and
draining, no duplicate buffering policy and no socket wait hidden inside an
unnecessarily retained execution/publication resource.

### P4. Clarify scheduling and admission capacity

Active slice: engine/scheduler.go, admission.go, TaskManager readiness/index
operations, task scheduling state and internal/admission and commitgate.

Give every capacity an explicit acquisition and release point. Separate
eligibility, dispatch-slot occupancy, admission waiting, active execution and
callback work in both code and existing diagnostic names. Review leaf-lock
ordering and work performed while scheduler/runtime/task locks are held.

Keep asynchronous DispatchReady and synchronous draining where their real
callers require those semantics; Eval is a real synchronous-drain caller.
Remove obsolete selection/batch paths only after their callers are gone.
Preserve wakeup ordering, input receipts, fork-family barriers, resume generations,
fairness identity and cancellation. A ready task is not automatically runnable.

Change slot lifetimes or fairness policy only for a shown ownership defect or
reproduced wait. Do not retune worker counts, weights or quantum as a cosmetic
refactor. Verify queue progress, no duplicate execution, no lost wakeups and no
starvation using deterministic synchronization and existing fairness tests.

Deliverable: one understandable capacity model with direct owners and no
redundant bookkeeping. Compare scheduling latency and both single-player and
concurrent throughput for any hot-path or lifetime change.

### P5. Make builtin effects and external-resource lifetimes consistent

Active slice: builtin descriptors/registry, threaded work, SQLite, file I/O,
curl, exec and other native builtins, session resources and their real callers.

Keep the descriptor catalog as the effect-policy source of truth. Distinguish
transactional work, commit-deferred launches, inline irreversible work and
operations that own their own boundary. Remove duplicated policy switches or
metadata only after proving they represent the same rule.

Give each operation a clear owner for start, result, cancellation and cleanup.
Distinguish canceling its consumer from interrupting a shared handle operation.
Use the P1 contract before changing kill/thread_mode behavior. Do not attach
every external operation to task cancellation merely because that is simpler.
Capture immutable worker inputs and generation identity; avoid reading a
mutable execution context after its task resumes.

Preserve at-most-once external work per published slice, per-handle sequencing,
SQL row-conversion ownership, file position behavior, exec cancellation and
session teardown. Cover error/close/kill/completion races and rejection of stale
results. Audit long native calls for existing work limits and cooperation points;
resolve actual monopolization/cancellation defects without blanket threading,
new pools or unrequested flags.

Deliverable: a complete operation-lifetime table and direct implementations in
their existing owners, with confirmed defects repaired and duplicate launch or
cleanup paths removed. Run affected native-operation regressions and measure
SQL/file/native throughput, queue delay and resource retention where changed.

### P6. Simplify store ownership without weakening isolation

Active slice: db/store transactions, objects/properties/relationships, keyed
maps, task rows, publication, hot renewal, dispatch dependencies and history.

Separate snapshot reads, staged mutation, dependency validation and publication
in the existing Store/StoreTxn APIs. Review direct/live paths against real
callers. Keep transactional task rows distinct from task execution ownership;
remove a representation only when it duplicates the same fact and lifetime.

Preserve granular keyed dependencies, absent-key reads, whole-map observations,
permission/owner/quota validation, reciprocal relationships and blind inverse
edge deltas. Preserve snapshot registration and explicit release. Consolidate
duplicated validation/publication mechanics at their store owner; do not replace
them with broad object conflicts or make errors disappear through retry.

Retain the shared publication gate versus exclusive slice grant distinction,
native errors and unknown outcomes. Every isolation-sensitive change uses the
pinned Elle workflow and its negative controls, with raw histories, manifests,
anomaly witnesses and nonzero committed path coverage retained.

Deliverable: clearer transaction ownership and fewer exceptional paths with the
same or stronger established behavior. Validate conformance, store races and
Elle, then compare conflict/retry costs, throughput and retained history.

### P7. Clarify language execution, compilation and value ownership

Active slice: parser, ir, compiler/cache, bytecode, sourcekey, preposition,
VM frame/opcode/continuation paths, types containers and the builtin interfaces
that consume them.

Keep syntax and diagnostics in parser/compiler; instruction layout in bytecode;
frames and execution state in VM; value semantics in types. Remove accidental
server/session policy from those owners where a concrete dependency audit finds
it. Preserve callable lookup, effective permissions, protected builtin frames,
caller identity, budgets and exception/suspension behavior.

Keep immutable program identity separate from transaction lookup identity.
Preserve compiler-key validity, cold-miss coalescing and per-transaction
dependency replay on memo hits. Consolidate duplicates without adding another
cache. Document borrowed container storage; preserve immutable aliases and the
distinction between VM retry copies, persisted snapshots and mutable WAIF state.

Use parser/formatter fuzz regressions, VM snapshot/continuation tests, bytecode
validation and affected type races. Run meaningful compiler/dispatch/container
benchmarks in CI for changed hot paths. Do not revive an unmeasured representation
experiment or rewrite the interpreter merely to reduce a file's line count.

Deliverable: direct language-layer boundaries, exact source/program identity,
explicit storage ownership and no duplicated semantic implementation.

### P8. Separate persistence, reclamation and finalizer responsibilities

Active slice: checkpoint server orchestration, db/format, store snapshots,
task persistence, VM retry/persistence code and engine finalization coordinator.

Keep consistency/drain decisions in the runtime/server and portable encoding
and durable artifact publication in db/format. Keep retry snapshots and disk
snapshots distinct. Remove duplicated serializers or snapshot adapters only
after the actual format/caller contract is accounted for.

Preserve one task/store checkpoint boundary, WAIF/anonymous identity and restart
behavior. Keep checkpoint capture, encoding and durable publication costs
separate. Encoding may move outside a world barrier only if the captured payload
is demonstrably independent of all reachable mutable state; otherwise retain
the barrier and improve the surrounding structure without pretending it is free.

Keep root capture, parked VM ownership and recycle-hook execution explicit.
Preserve the sweep-owned entry path, start barrier and re-entry behavior.
Refactor finalization bookkeeping into its existing coordinator, deleting
duplicate lifetime state. Cover nested hooks, shutdown during finalization,
unparkable VMs, snapshot histories, native errors and restart failures.

Deliverable: visibly correct checkpoint/finalizer ownership, shorter justified
holds where possible and no premature reclamation or repeated continuation.
Validate restart/conformance/Elle as affected; compare pause, serialization,
publication, retained memory and finalizer throughput separately.

### P9. Finish process composition, tools and diagnostics

Active slice: cmd, internal/app, internal/dbtool/buildinfo/listener, server
lifecycle, config/profile wiring, session host construction, logging, metrics,
trace, internal/mooprof and existing profiling/reporting/workload scripts.

Make startup, readiness, shutdown and resource teardown follow their actual
owners. Preserve the order of manifests, listener reservation, startup hooks,
accepting clients, checkpointing and session closure. Audit every partial-start
and failure exit for owned resources and error preservation.

Keep common entrypoints shared by server and tools while retaining their
different capabilities and suspension contracts. Delete copied behavior rather
than adding facades. Keep diagnostics attributable to actual task/command and
phase; do not turn process-wide concurrent counters into per-command costs.
Use existing collectors and add only a measurement demonstrated to be missing.

Deliverable: straightforward composition and teardown, consistent tools and
documentation that describe the resulting code. Validate startup failure,
disconnect/shutdown, tool execution and log/report contracts.

### Validation and non-regression policy

Use existing checks, not a new review bureaucracy. Every retained code slice
has source-backed ownership evidence, appropriate behavioral regressions and
the repository's required PR checks. Broad Go tests, race checks, static
analysis, builds and full conformance run on PR CI. Only the explicitly named
test file currently being written may be run locally under Q's rule.

Resolve uncertain MOO expectations with the canonical managed stock Toast
workflow; preserve native outcomes and full failure inventories. Do not manually
launch conformance servers or run against tracked databases. Elle is mandatory
for the changes named in docs/elle-isolation.md, with explicit verdicts and
coverage evidence; ordinary Go tests are not isolation proof.

For performance, use the documented native Linux baseline/candidate matrix and
the real-workload harness through a PR CI task. The inspected default CI does
not currently invoke TestMongooseRealWorkload: do not claim it does. Where that
collector is needed, add only the invocation of the existing harness to the
existing workflow on the fixture-capable runner, with fixture access solved in
P0 and runner selection still using CI_RUNNER_LABELS and its hosted fallback.
Do not build a new benchmark framework or silently skip unavailable fixtures.
The native matrix is a standalone workload program, not a local full-CI rerun.
Freeze both binaries, source database, sound catalog, repair settings, player roster, command
mix, worker/admission/GC settings and deadlines. Alternate fresh-world engine
order and retain all raw results. Use instrumented runs for attribution and
matched uninstrumented runs for comparisons. Inspect throughput, latency tails,
allocation/retention, startup, idle and shutdown costs relevant to the change.

Performance-affecting ownership/algorithm changes get paired evidence before
acceptance. Mechanical changes use checks proportionate to their actual risk;
do not repeat the full workload matrix for every textual move. Compare the
integrated result against the original baseline at the end as well. A
reproducible slowdown is repaired or the candidate change is not retained.
Inconclusive evidence stays inconclusive; do not label it neutral. A correctness
repair with an unavoidable slowdown is an explicit conflict for Q before merge,
not permission to silently trade away the performance constraint.

### Delivery loop and completion

Work sequentially through bounded slices. For each one: read its complete
production/caller boundary; record why its surfaces exist; choose the owner and
delete/move/consolidate/rewrite/keep disposition; remove the wrong surface and
repair callers directly; finish its existing checks and relevant measurements;
then deliver before starting the next overlapping slice.

Use independent branches/PRs from current default, with no stacked PRs. Keep
companion-repository dependencies first. Once publication and merge are
authorized, commit only the slice and its living note, push/open the requested
PR, watch its exact-head checks, repair failures, resolve actual review blockers
and perform the repository's normal two-parent merge/merge-queue flow. Re-query
remote merge state; queued, green, merged and deployed remain different states.
Do not post reviews/comments or change labels without the required authority.

After an authorized verified merge, integrate locally without disturbing dirty
work, then remove only the owned merged branch/worktree. Keep raw evidence and
local paper PDFs/images. Notes and commit messages contain no commit hashes.
Carry this living record into substantive work, updating dispositions and the
next slice; do not create per-session handoff files or copy source code.

Complete the whole-system pass by accounting for every production package and
its relevant symbols, including the keep decisions. Verify old surfaces have
no production callers and no aliases, wrappers or fallbacks preserving them.
Run final integration checks and matched original-versus-final measurements.
Close documentation/spec drift and list any remaining problem with its evidence
and state. Completion means the chosen ownership is present in merged code,
the replaced implementation is absent, required checks are satisfied and no
unreported dependency, setup or acceptance work remains.

### Planning evidence and limits

Planning source refresh: remote master with subject `Merge pull request #629
from MongooseMoo/test/elle-maps-permissions`. Compared with the preceding
research refresh, changes in the examined runtime scope were limited to map
dependency fault probes and Elle cases; execution/output owners were unchanged.
Recheck before each future slice rather than applying stale line-number edits.

Literature prerequisite: applied paper-process notes-only flow to the existing
canonical Automatic Mutual Exclusion paper by Isard and Birrell (HotOS 2007).
Reused its PDF and complete reader artifacts, read all six page images, checked
existing tags and collection cross-references, and left them unchanged. Sections
3-4 distinguish explicit atomic-fragment boundaries from non-abortable I/O and
deferred work. That supports making Barn's boundaries explicit; it does not
authorize inserting new MOO yield points or prove Barn's implementation correct.
Primary paper: https://www.usenix.org/legacy/event/hotos07/tech/full_papers/isard/isard.pdf

The whole execution sequence is specified here; later slice edits are
conditional on the contract/caller evidence gathered in that slice. In
particular, P3's output acceptance/results design is not yet settled. Do not
describe it as implementation-ready before P1 resolves those discoverable
contracts and their concurrent implications. Structural work whose own
contracts are settled need not wait for a global bottleneck ranking.

## State

PR #638 final-verifier checkpoint: published eval-resource and standalone-hook
cleanup inspected independently of future uncommitted slices. All seven PR
checks are terminal passes, including conformance. The pinned Elle artifact is
preserved in .tmp/system-layering/elle-owner-pr/elle-evidence.tar.gz; direct
parsing reports six healthy true and twenty fault-control false verdicts, with
nonzero parallel, exclusive, renewal, fork, suspend and exception paths. Final
bounded verdict is being recorded in the owner-cleanup verifier report.

The bounded manifest-checksum slice is implemented with a fixed local 32 KiB
read buffer. Exact digests and native open/read errors pass. The first io.Copy
candidate was rejected for a measured small-config regression. The retained
loop reports 328 B/op for both a 21-byte config and 32 MiB database fixture;
benchstat against original reports -6.87% and -21.97% time respectively with ten
samples. PR CI/review remain pending; no all-input/application-wide claim is
made. Its full report is reports/system-layering-manifest-hash-coder.md.

Q authorized execution with "Make it so!". The isolated refactor/system-layering
worktree is based on remote master with subject Merge pull request #628 from
MongooseMoo/test/elle-definitions. The other checkout's dirty files and worktrees
remain outside this slice.

P0 is active. The current private fixture candidates are
C:/Users/Q/code/barn/mongoose.db.new (260031588 bytes, November 5) and
C:/Users/Q/code/barn/files/sqlite/sound.sqlite (481218560 bytes, November 5).
The mongoose_db checkout's 61494805-byte August 2025 database is older and is
not selected as the current workload. Neither benchmark credential environment
variable is present; the native matrix requires both, while the in-process
workload has a separate credential-free path. Runner selection is already
["self-hosted","Linux","X64","barn"].
Frozen binary/fixture/config identities are recorded in
.tmp/system-layering/baseline-manifest.json. These are identities, not measured
baseline results. The benchmark credential-source question remains pending.

P1's managed stock WSL Toast session completed: 72 passed for admission,
notify semantics/call shapes and buffered output length. Four additional
byte-accounting cases plus admission passed on stock Toast with no skips, then
failed all four behavior cases on Barn: expected byte deltas 5, 3, 3/7 and 2;
Barn returned notification counts 1, 1, 1/2 and 1. Raw XML and admission evidence
are in .tmp/system-layering/toast-byte-v2.xml and barn-byte-before.xml with
matching admission JSON files. The initial incorrect singular getter produced
four skips; toast-byte.xml preserves that non-verdict. The corrected generic
suite uses canonical connection_options without an optional prerequisite.
Two further stock-oracle acceptance cases plus admission passed. Reloading
max_queued_output changes the no-argument length result within the same task.
With a 2048-byte capacity, a small queued notification followed by a 4096-byte
payload with no_flush returns {1, 0}, even when the earlier bytes can drain.
Raw evidence is toast-acceptance.xml and its admission JSON. The frozen Barn
comparison passed admission and failed both cases: 65536 instead of 4096 and
{1, 1} instead of {1, 0}. barn-acceptance-before.xml preserves both failures.
This establishes real
rejectable notify results, so deferred always-success acceptance cannot be the
P3 design. Full slow-peer/discard/boot ordering contracts remain unresolved.

The byte-accounting coder has captured focused red tests for queued bytes,
partial flush suffixes, pending framing, UTF-8 and binary output. It is applying
the shared PendingNotification calculation and a cached connection total;
physical transport/framing and effect ordering stay outside this slice.

The bounded byte-accounting implementation now shares PendingNotification.ByteLength
between deferred effects and connection queue accounting. Connection maintains
its total at existing enqueue and successful-delivery sites, preserving O(1)
length queries and failed suffix ownership. Edited focused tests captured the
original count failures and now report:

```text
ok github.com/MongooseMoo/barn/server 0.006s
ok github.com/MongooseMoo/barn/builtins 0.100s
diff_check_exit=0
```

The managed post-fix byte suite then reported five passes with no skipped cases
in 7.29 seconds, including admission. barn-byte-after.xml and its admission JSON
preserve the result. PR #636 passed build/unit, managed conformance and pinned
Elle CI, received analyst and final verifier clearance, and merged through the
merge queue on November 7. Local integration fast-forwarded to that merge.
The implementation report records exact focused selectors and red/green evidence.

P2's first bounded slice is implemented in PR #634: consolidate duplicated
command-output settlement in the existing Runtime.settleTaskResult owner,
using Task/error inputs rather than the scheduler's result carrier. The three
entrypoints are ExecuteVerbTaskSyncWithStart, successful runServerVerbTask and
ResumeReadingTask. Preserve their error policies, all lease/admission/callback
ordering, and distinct Eval/login/sweep lifetimes. Reports are
reports/system-layering-execution-scout.md and
reports/system-layering-settlement-coder.md. The source diff is 8 insertions and
27 deletions across three engine files. It leaves one production
flushTaskOutput(t) call in the existing settlement owner. No local test result
is claimed. PR CI completed successfully: build/unit, managed conformance and
Elle workflow. The analyst found no concrete defect; final verifier approved
the exact published diff. PR #634 merged through the required merge queue on
November 7. All four review/implementation agents delivered and stopped.
No throughput improvement or measured performance neutrality is claimed.

The P4 readiness/admission scout completed its producer/lifetime survey. The
unused required TaskYielder host capability and two forwarding methods are now
deleted in merged PR #637, together with startup wiring and three obsolete test
assignments. The seven-Go-file source diff adds one line and removes twenty;
the now-unnecessary Yin helper local is replaced with a direct constructor return.
it preserves ProcessReadyTasks and its engine/eval.go caller. Raw checks:

```text
gofmt_exit=0
diff_check_exit=0
removed_symbol_search_exit=1
```

The last result is the repository Go search for TaskYielder/YieldReadyTasks,
with no matches. No local tests/build ran for this deletion-only slice. Build,
full conformance and Elle PR CI passed; analyst and final verifier cleared the
actual diff. The required merge queue completed and local integration fast-
forwarded on November 7.
Reservation, physical lease and dispatch slots have distinct producers and
lifetimes and retain keep dispositions. The implementation report is
reports/system-layering-task-yielder-coder.md.

P3's acceptance design needs care: the existing irreversible-effect boundary
can provide a real notify return before execution continues, but retains the
exclusive gate for the remaining computation. Simply marking every notify
irreversible may trade socket waiting for unnecessary serialization. No such
change has been selected or implemented; preserve the observed rejection result
without inventing speculative transaction machinery or claiming a speedup.

P5's builtin lifetime scout has identified a dbtool EvalSession.Close teardown
omission: it stops Runtime but does not close the builtin Session's handles.
The direct correction belongs at EvalSession.Close with cleanup errors carried
through EvalExpression, EvalFile and cmd/moo's existing error outputs.
That four-file correction is implemented locally. Its focused regression opened
real file/SQLite handles through eval, failed before cleanup delegation, then
passed after it; repeated Close is safe. PR CI and review remain pending.
file_count_lines/file_grep also change a shared cursor without an irreversible
descriptor. Existing managed suites cover only presence/signatures/invalid
handles, so valid-file cursor oracle cases are needed before that correction.
The first candidate fixture was invalid: suite setup executes physical lines
separately and ignores errors, so a multi-line program created an empty file.
Preserve toast-file-scan.xml as a fixture non-verdict. The corrected single-
program fixture passed all three scan cases plus admission on stock Toast;
toast-file-scan-v2.xml records count/all-match cursor at EOF and first-match
cursor immediately after the matching line. Barn passed admission/count/all-
match but first-match grep raised E_FILE on the next file_readline instead of
returning omega. barn-file-scan-before.xml preserves this confirmed cursor
defect. Focused new regressions reproduced first-match cursor 19 instead of 13
in text and binary modes. Registered file_count_lines/file_grep calls also ran
with zero boundary crossings and moved to 19 despite a refusing callback:

```text
fileio_readline_buffer_test.go:60: result = {19 0 E_NONE unknown  <nil> []}, want 13
file_count_lines: crossings=0 irreversible=false, want aborted before effect
file_grep: crossings=0 irreversible=false, want aborted before effect
FAIL github.com/MongooseMoo/barn/builtins 0.133s
```

The coder is holding production edits while the parent verifies long-line
behavior, because replacing Scanner with Reader also removes Scanner's token
limit. No additional stream machinery has been added. Fix read-ahead
consumption and the two missing effect policies after that evidence arrives.
SQL slot/row
conversion, immutable worker inputs and generation-based resume have keep
dispositions. Subprocess shutdown cancellation remains an unverified lifetime
question. Findings and their limits live in
reports/system-layering-builtins-scout.md.

P6's completed store survey found a prompt snapshot-release omission in
standalone callVerbWithArgstr: ordinary login hooks lack explicit Release;
sweep hooks capture the original receiver even when GC renewal replaces it.
The existing owner's correction is unconditional late-bound cleanup, now
implemented locally. With Go GC disabled, its real ordinary-hook
regressions reproduced one active snapshot after read/write/error/run_gc
returns where zero are required; actual run_gc replacement was asserted.
The same focused selector then reported:

```text
gofmt_exit=0
diff_check_exit=0
ok github.com/MongooseMoo/barn/engine 0.116s
```

It also covers read/write/error hooks under the actual held sweep barriers and
reentrant run_gc's no-op snapshot behavior. The source change adds one
late-bound defer and removes the sweep-only captured defer. No broad local
tests or Elle ran; independent PR CI and explicit isolation evidence remain
pending. Implementation details are recorded in
reports/system-layering-call-verb-snapshot-coder.md.
Preserve raw source findings in
reports/system-layering-store-scout.md. Granular map dependencies, publication
locks, task-row atomicity, renewal distinctions and pruning retain keep decisions.

P7's language/value survey completed. Keep decisions cover the bounded compiler
cache, immutable builtin IDs, shared sourcekey owner, compilation outside locks,
retry versus persistence snapshots and immutable values/WAIF images. Two direct
beauty candidates are consolidating optional activation-list fields in
ActivationFrame.ToList and deleting the literal error forwarding wrapper.
The six-file cleanup is implemented locally: exact-size frame construction
preserves lazy variables and caller redaction. The wrapper is deleted.
Formatting/diff checks passed; no mirror tests or local test run were added.
reports/system-layering-language-scout.md preserves the handoff.

P8's persistence/finalization survey completed. The writer's emitted surfaces
are detached after store/task capture and value rewriting; use the existing
Checkpoint and Writer owners to split Capture from Serialize. Only running
checkpoints may encode/publish after the capture barrier. Terminal checkpoints
retain the whole barrier. Direct stage interface and WriteCheckpoint callers
are inspected. Use Capture(current inputs), Serialize() and the retained
existing Writer, clearing captured state on Serialize/Close. Write the
deterministic blocked-encoding/frozen-graph regressions before the phase split.
Preserve finalization roots, parking, path locks and recovery ownership. The
handoff is reports/system-layering-persistence-scout.md. The phase split is
implemented locally. Focused tests reproduce blocked Eval before the change,
then allow commit during encoding while reload preserves captured scalar/WAIF
values, identity and task aliases. Stage abort/state tests also pass.

P9 survey completed: move startup-script input reads before server construction;
stream database/config manifest hashes in their existing owner; close builtin
session resources on listener-start failure after runtime shutdown. Keep the
distinct panic-to-abort contract. Config, listener and diagnostic ownership
received source-backed keep dispositions in
reports/system-layering-composition-scout.md. Manifest hashing is in a measured
baseline/candidate implementation slice. Focused startup RED regressions
reproduced a missing script creating the profile manifest before failure and
listener reservation leaving real file/SQLite handles valid after Start
returned. Preserved pre-correction excerpts:

```text
missing script created profile manifest: <nil>
file_name after failed Start: Error:E_NONE, want E_INVARG
sqlite_info after failed Start: Error:E_NONE, want E_INVARG
FAIL github.com/MongooseMoo/barn/internal/app 0.042s
FAIL github.com/MongooseMoo/barn/server 0.030s
```

The bounded startup correction then passed its focused app/server regressions;
exact selectors and evidence are in reports/system-layering-startup-coder.md. No
blanket graceful/panic teardown change or Close facade is selected.

The file-scan correction is implemented locally and its managed post-fix suite
passed five cases including admission in 6.92 seconds. It repairs first-match
read-ahead, supports stock-confirmed long lines and puts cursor mutation behind
the existing irreversible boundary. Raw evidence: barn-file-scan-after.xml.
PR #638's separate eval/session and standalone snapshot repairs passed full
PR/queue CI and explicit Elle verdict/path checks, then merged and integrated
locally on November 7. Companion byte suite PR #177 also merged after all Toast
profiles and final review; its isolated checkout fast-forwarded to main.

The dynamic capacity correction is implemented in the existing server-options
snapshot/cache, with two focused regressions passing. It preserves task-private
reloads and commit publication, signed-32 cache conversion and positive minimum
clamping. A stock 64-bit research probe confirmed wrap-before-clamp; keep that
wide-literal probe local rather than publishing it into 32-bit profiles.

Manifest hashing's retained bounded-loop candidate measured 328 B/call for both
21-byte config and 32 MiB inputs, versus 1016 B and about 33.56 MB originally.
Ten-sample times improved on both measured sizes; rejected io.Copy had regressed
the config path. No application-wide performance claim follows from this probe.
Early script validation and listener-failure Session.Close are implemented;
their real failure regressions pass. Source and timing evidence stay in their
bounded coder reports; all these future slices still require PR delivery.
Output acceptance and concurrent result semantics remain a P1 prerequisite for P3.

First pass traced input, admission, background selection, task slices,
transaction publication, external operations, output, checkpointing and GC.
Q then directed deeper understanding, taking notes along the way. The second
source pass traced resource lifetimes, effect ordering, external cancellation,
maintenance, dispatch/compilation caches and value/retry ownership. It also
compared output ownership with the installed WSL Toast source. This is not a
complete system audit. No new performance measurement supports a ranking of
current bottlenecks.

The local master checkout is behind remote master. Refreshed origin/master
with `git fetch origin master`; read updated files with `git show` and compared
them with the checkout. The checkout and existing dirty work were preserved.
The detailed second-pass references below were recorded at the refresh with
subject `Merge pull request #630 from MongooseMoo/test/elle-map-quota`.
The newer planning refresh and its limited source delta are recorded above.
Recheck the relevant source on continuation rather than treating a moving
origin/master reference or an old line number as an immutable source identity.

Read docs/workload-diagnostics.md and docs/elle-isolation.md. Historical latency
and workload notes are leads, not current measurements or proof that a defect
still exists. The initial two source passes made no literature-based design
claim; the execution plan above adds the image-read AME grounding and identifies
the concrete contracts that remain to be settled.

## Layer map

1. Process composition: cmd/barn/main.go passes flags/configuration into
   internal/app/app.go; server/server.go owns loading and server lifecycle.
2. Transport: server/transport.go and server/websocket_transport.go decode
   input and synchronously write output. Connection objects own connection
   identity, login state, output buffering and their cancellation context.
3. Input: server/input_processor.go routes events onto serial per-connection
   lanes. A connection's command processing is ordered, while separate lanes
   can proceed concurrently. The refreshed source uses
   server/input_buffer.go to bound outstanding transport events to 16 plus
   one decoded event waiting for capacity, with an additional byte budget.
   Forced input does not consume that transport budget.
4. Command dispatch: input processing classifies input using the connection's
   player when processing begins, handles login/read/OOB/intrinsics, then
   invokes do_command or command matching. command/ owns parsing/matching;
   engine task entry owns execution. The serial lane can remain occupied
   during the synchronous entry invocation; this is distinct from whether
   the transport reader can accept another line.
5. Scheduling: engine/scheduler.go selects ready continuations, enforces lane
   exclusions and fork-family yield barriers. Runtime.DispatchReady starts
   eligible work without joining every running slice. Dispatch goroutines and
   admission reservations are separate: do not infer active VM occupancy
   from goroutine count or the scheduler worker-count setting alone.
6. Admission: internal/admission/controller.go and scope.go bound execution
   occupancy and account service by principal/class. Scope.Yield lends the
   reservation but retains the VM, transaction and physical execution lease.
   This is not a MOO suspension or a transaction commit.
7. Execution: engine/task_runtime.go owns attempts, saved-VM execution,
   retry state, publication and cleanup. Its physical execution lease extends
   through state publication and cleanup; logical suspension alone does not
   make the VM safe for another goroutine to inspect. vm/ owns frames,
   opcodes, nested calls and VM-local state. builtins/ owns host operations.
8. Store: db/store owns snapshots, dependencies, staged changes and
   validation/publication. Ordinary object commits acquire the commit gate
   shared, store.mu shared and commitMu exclusively across validation and
   publication. Live/anonymous-object mutation takes store.mu exclusively.
   An escalated or irreversible slice can own the commit gate exclusively
   outside Commit itself. Task-only commits have a separate route.
9. External work: builtins/threaded.go records an async start as a deferred
   effect. Work starts after publication, then completion resumes the matching
   suspension generation. SQLite additionally serializes operations per
   handle, including row conversion; its slot is released before diagnostic
   logging. Inline SQLite enters the irreversible boundary before waiting
   for its handle or performing SQL.
10. Effects/output: builtins/pending_effects.go replays committed effects
    in call order. Notification delivery calls Connection.SendNotification
    synchronously. A preceding notification can therefore delay a later
    async-start effect in the same list. This is separate from SQL duration.
11. Persistence: Server.dumpCheckpoint captures tasks/connections and calls
    Serialize inside WithCheckpointBarrier. That barrier pauses admission
    and takes SweepMu and VMStartMu. Running checkpoints call Publish after
    releasing the barrier; terminal checkpoints publish inside it. Serialization
    pause and durable-publication time must be examined separately.
12. Reclamation: engine/gc_handshake.go asks running VMs to park at tick
    checkpoints, retaining their VM, transaction and lease until released.
    This differs from checkpoint admission pause. Deeper root-scan,
    finalizer and history-retention cost analysis remains open.

## Source-backed candidates and limits

### Output can extend exclusive publication ownership

Current origin/master engine/task_runtime.go calls FlushPendingEffects at
line 582 before gate.release at line 599. BeforeIrreversibleEffect also
flushes published effects while holding the gate (line 391). The gate is an
exclusive store commit grant; ordinary object commits need its shared side
(db/store/store_txn_commit.go:258 onward).

The real producer is a committed MOO notification: FlushPendingEffects calls
SendNotification, which calls transport.WriteOutput while holding the
connection mutex. TCP flushes synchronously; WebSocket writes synchronously.
Current source bounds each active write with outputWriteTimeout, 30 seconds
(server/transport.go:65; websocket_transport.go:89). This is bounded blocking,
not evidence of an indefinite write. Multiple effects and waiting for an
output mutex require separate accounting.

Consequences to reproduce: an exclusive slice emitting to a slow reader can
retain the gate and admission/execution ownership during output; unrelated
object commits may wait. A notification before an async-start effect can delay
launching the external operation too. These follow from call ordering, but
current incidence, latency and throughput impact are unmeasured. Do not move
effects or release the gate without understanding publication/effect ordering.

### Inline external waits can occur inside the exclusive gate

builtins/sqlite.go:277 onward enters beginIrreversible before inline SQL,
including the per-handle wait. engine/task_runtime.go acquires the exclusive
gate at that boundary and retains it through the remaining attempt.
Threaded calls use the separate commit-and-suspend path instead. Determine
whether the real MOO workload uses inline calls before assigning impact.

### Serialization holds a global checkpoint barrier

server/server.go:504 onward calls checkpoint.Serialize inside the runtime
barrier. This is a concrete scope to measure; it is not itself proof of an
avoidable defect. Mutable WAIF graphs explain why a shallow snapshot does
not automatically permit serialization after releasing the barrier.

## Decisions and why

### Second-pass findings: execution and effects

Source basis: refreshed remote master described in State. These are Barn
implementation contracts, not newly verified cross-server conformance claims.
Existing tests below were read, not executed in this research pass.

#### Ordinary command lifetime

ExecuteVerbTaskSyncWithStart (engine/task_runtime.go:1311 onward) enters input
admission before compiling the matched verb, constructs/registers the task,
then calls runTaskAdmitted with ownership of that scope. The input lane is
occupied until that synchronous entry returns. The transport reader has its
separate bounded read-ahead budget.

runTaskSliceAdmitted acquires the physical execution lease, starts admission
service accounting, attaches the execution context, and opens a fresh snapshot.
After VM return it stages its own task row and resolves commit/retry. A native
exception can still commit writes and effects; an unpublished conflict attempt
discards its effects and created forks before re-execution. Thus exception,
commit failure, explicit cancellation and successful completion cannot share a
single undifferentiated cleanup action.

The final attempt calls FlushPendingEffects before releasing the old snapshot
and exclusive gate (lines 582, 591, 599). The return defers release the physical
lease, finish admission and then attempt deferred GC (lines 197-201). Completion
callbacks run after runTaskSliceAdmitted has returned, with the result pinned
as a root across that gap. Suffix/output-buffer flushing follows the synchronous
entry; Done closes only for terminal state after that flush. A suspended task
does not mean its command is terminal.

Ownership therefore extends through effect delivery, not just through VM
instructions. Moving only transport output has implications for task completion,
the input lane and first-output/terminal-output measurements as well as the gate.

#### Threaded SQLite lifetime

runInBackground suspends the task, captures its suspension generation and queues
an async-start effect. No worker starts for a discarded attempt. After the slice
publishes, effect replay starts a goroutine. Completion can arrive before the
original slice has saved its VM and relinquished physical ownership: readiness
alone is insufficient to dispatch it. ReserveAdmission/TryClaimQueued reject
executionActive tasks; the lease release requests another scheduling scan.
ResumeGeneration checks both generation and resumability before recording
external-ready time (task/task.go).

The SQLite operation slot ends when SQL and row conversion finish, before
diagnostic logging and ResumeGeneration. Resumption obtains background admission
and a fresh snapshot/execution budget. The SQL handle slot does not need to
remain owned while the MOO continuation waits to run.

engine/sqlite_retry_test.go:104 deliberately forces a conflict before an INSERT
and checks that only the committed attempt's statement is executed. This is an
existing contract to preserve, not a fresh passing-test claim.

#### Irreversible lifetime

Registry.install wraps descriptors marked Irreversible with beginIrreversible.
The runtime boundary acquires the exclusive gate, validates/publishes prior
writes and carries dependency reads into a new snapshot. The rest of the slice
remains under that gate to prevent a conflict after an external action that
cannot be undone. Retrying that remainder would repeat the action; simply
dropping the gate immediately after the action would lose this protection.

The preempt hook explicitly skips both admission Yield and GC parking while
the gate is held (engine/task_runtime.go:435 onward). A holder waiting for
readmission could prevent admitted committers from completing; GC hooks can
also commit. Any refactor must preserve this dependency, not merely move locks
into smaller functions.

engine/irreversible_effect_boundary_test.go checks stale reads retry before
server_log, that the log happens once, and that the post-effect writes publish.
It also records exclusive hold duration through subsequent work in the slice.

#### What ordering is actually encoded

- The pending list preserves order among its own effects. Its boot-then-notify
  producer is exercised by engine/worker_pool_test.go:763: boot retires the
  connection, and the later notification must not be delivered.
- Notification framing is captured when notify executes, but the recipient
  connection is looked up again when the effect is replayed. Connection switch,
  boot and output are therefore related operations, not independent queues.
- An effect failure is logged and does not undo the committed store or replace
  the task's result. Replay continues with later effects; the existing test at
  engine/worker_pool_test.go:831 checks a committed value and return 42 survive
  a send failure.
- There is no global effect-order dispatcher in this path: each task replays its
  own list after Commit returns, outside the store's commitMu. Ordinary tasks
  hold no exclusive slice grant. Do not assume store publication order already
  establishes cross-task transport delivery order.
- An irreversible mid-slice boundary flushes queued effects only when
  CommitAndRenewCarryingReads reports publishedWrites. Queued effects alone
  are not a store write. Per-list ordering must not be described as a universal
  source-order guarantee covering all immediate external operations.
- Server output-deadline tests exercise partial TCP writes, timeout closure,
  reader wake and no replay of partial output. They do not establish that
  unrelated commits progress while a task is delivering pending output.

#### Refactor implications established so far

Keep three boundaries explicit: resolving the attempt, applying its ordered
committed effects, and settling task ownership/completion. They are currently
interleaved in the runtime. These are responsibilities to understand, not a
proposal to add new layers or queues. Moving final effect replay after releasing
the exclusive gate is a bounded candidate; it must be assessed separately from
mid-slice effect replay, which occurs while the VM will continue executing.

Unsettled: whether the chosen MOO observation contract requires stronger
cross-task effect ordering, whether a new effect delivery lifetime can safely
outlive task roots/connection changes, and which real workload exercises each
case. No code motion is authorized or justified as a completed design yet.

### Second-pass findings: capacity and cancellation

#### Scheduler capacity is not physical execution capacity

Dispatch (engine/scheduler.go:184) increments inFlight before starting runTask.
That slot remains occupied during background admission wait, VM execution,
effect replay and completion callback. It is decremented after runTask returns,
before the done callback flushes suffix/output and closes terminal Done.
heldLocked at line 256 bounds these slots by workers and enforces per-lane
exclusion. Direct input execution enters admission without this scheduler slot.

Consequently, scheduler slots, admission reservations and execution leases are
three different capacities with different lifetimes. Releasing admission does
not immediately free a scheduler slot; a callback can run after the VM lease is
released while still inside runTask. This is a source dependency to measure,
not proof that the capacity policy is wrong.

Scheduling uses TaskManager.Unfinished, which copies an index under a leaf lock
without acquiring every task lock. readyLocked and blockedReadiersLocked still
inspect the unfinished catalog under the scheduler mutex. Retained completed
tasks are not part of that main scheduling scan. Do not infer a completed-task
scan bottleneck from TaskManager.Snapshot, which is used by other paths.

#### External operations have different cancellation owners

| Operation | Where work runs | Capacity held while working | Cancellation owner |
| --- | --- | --- | --- |
| Threaded SQLite query/execute | Commit-started goroutine | Per-handle slot through SQL and row conversion | Handle operation context; sqlite_interrupt or Session.Close |
| Inline SQLite query/execute | VM goroutine | Admission, lease, exclusive gate, then handle slot | Same handle operation context |
| Threaded curl | Commit-started goroutine | HTTP request/response resources | HTTP client timeout; task kill rejects completion but is not request cancellation |
| Inline curl | VM goroutine | Admission, lease, exclusive gate | HTTP client timeout |
| exec | Goroutine launched after irreversible boundary | Child process, I/O buffers; VM later suspends | Task.ExecCancelFunc and independent 30-second timeout |
| File read/write/seek | VM goroutine | Admission, lease, per-file position lock; irreversible descriptors also hold gate | File close; no task-derived I/O context in these paths |
| open_network_connection | VM goroutine | Admission, lease, exclusive gate | One-second dial timeout, then up to two seconds waiting for accepted registration |

Sources: builtins/threaded.go, sqlite.go:91/245/277/623, curl.go:13/54,
system.go:148/223, fileio.go:17/361/1006/1036/1085,
connection.go:317 and server/connection_manager.go:773.

SQLite beginSQLiteOperation uses context.Background and a condition-variable
wait. Killing its MOO task does not call the handle's currentCancel. The work
can continue owning the slot after its consumer is killed; ResumeGeneration
then rejects a stale/killed completion. This is different from exec, whose
cancel function is explicitly called by State.killLocked. No current workload
frequency or normative change is inferred from this difference.

Session.Close removes handle maps, closes files, cancels each SQLite current
operation and waits for activeOps before closing the SQL connection/database
(builtins/session_runtime.go:84 onward). Runtime.Stop cancels runtime contexts
and joins scheduler/maintenance; it is not synonymous with Session.Close.
Server shutdown calls input.Stop, terminal checkpoint, runtime.Stop, joins
backgroundWG and then Session.Close (server/server.go:549 onward). Teardown
ordering must therefore be traced independently of a task kill.

file_count_lines and file_grep seek and scan inline under positionMu. Their
descriptors do not enter the irreversible gate, but they still occupy their VM
and admission capacity until returning and can delay other uses of that file.
Treat their position/retry semantics as a separate contract question before
changing their execution mode. No defect or workload impact is claimed here.

### Second-pass findings: maintenance and retained memory

#### Checkpoint protection covers encoding and buffered file output

The checkpoint barrier drains admission in two stages (background first,
then input/command continuations), takes SweepMu and VMStartMu, and invokes
Serialize. Serialize collects task roots, calls SnapshotWithRoots, rewrites
task values and writes the portable dump before returning. SnapshotWithRoots
itself holds the exclusive commit gate across the store walk, preventing a
multi-object snapshot from crossing a transaction's publication.

Running-mode Publish follows barrier release. Terminal-mode Publish remains
inside it. Thus there are at least four distinct costs: drain, snapshot/root
capture, encoding/output, and durable publication. The whole checkpoint's
duration cannot establish which one stalled players. Moving encoding outside
the barrier requires demonstrating that every reachable mutable value is owned
by the snapshot, including WAIF state; copying the object map is insufficient.
Sources: engine/admission.go:44, internal/admission/controller.go:143,
server/server.go:493, db/format/checkpoint_stage.go:61 and
db/store/store_snapshot.go:68.

#### GC protection covers user recycle hooks

flushDeferredGCBarrier (engine/waif_lifecycle.go:236) keeps SweepMu and
VMStartMu through root capture, batch processing and recycle hooks. Maintenance
requests cooperative VM parking; it does not use checkpoint's admission Pause.
Parked VMs retain their transaction, lease and admission reservation. Admission
may grant new work, but that work cannot obtain a physical start past VMStartMu
until the sweep completes. Sweep-owned synchronous hooks bypass ordinary
admission and must inherit the protected start boundary.

Maintenance has bounded patience (20 ms) to obtain barriers/parking and retries
later if an executing builtin or gate holder cannot park. That bound is not a
limit on a sweep's total duration or on recycle-hook work after entry. Measure
hook/root-scan duration separately before attributing a delay to Go GC or
changing the cadence.

#### Snapshot registration is another retained resource

BeginSnapshot registers its read timestamp before returning. Release
deregisters once, may trigger pending WAIF-history pruning, and clears its Go
finalizer backstop (db/store/store_history_gc.go:342/364). It does not erase
all transaction object fields; replacing/dropping the transaction also matters
for reference retention. Old readers constrain which object versions may be
pruned. Admission Yield keeps that snapshot; actual suspension returns through
cleanup that releases it before the next slice starts with a new one.

The final effect flush precedes that old-snapshot release too. A blocked output
can therefore extend a reader registration as well as admission/lease/gate
ownership. The amount of retained history and its cost require runtime evidence.

### Second-pass findings: dispatch, compilation and values

Command matching (command/matcher.go and verbs.go) reads the live store through
DirectTxn before task entry. Object matching scans inventory/room objects and
their names/aliases; command verb dispatch searches player, location, direct
object and indirect object. This is separate from transactional nested verb
dispatch inside the VM. A received command is not one snapshot from socket
receipt onward, and its pre-task matching work is not VM instruction time.

Compiler.CompileMOOWithKey uses a stored source-content key when supplied.
Its program cache has 64 mutex-protected LRU shards; parsing/lowering run after
the shard lock is released. Concurrent misses for the same key join one
compilation through a channel. That wait has no task-derived cancellation
argument. Task entry compiles after admission but before task creation/lease;
nested calls can compile while already executing. Warm static VM call sites
can reuse an atomically stored program for the same compiler and content key,
bypassing the LRU lookup (compiler/cache.go, compiler/compiler.go:112,
vm/verb_call_site.go). A claim that every call hashes/recompiles the source or
contends on one global compiler mutex would be wrong.

Store dispatch memoization has a different owner and correctness obligation.
Transaction-local records replay snapshot dependencies; a shared dispatch hit
still re-fetches the selected definer/index, validates permission/shape and
records the read marks needed for later validation. Call-site identity is shared,
while its lookup record belongs to each StoreTxn. StoreTxn assumes one executing
goroutine, so moving lookups or effect processing onto another goroutine must
not accidentally share that mutable transaction scratch.
Sources: db/store/store_resolve_cache.go and store_verb_call_site.go.

Relationship publication already distinguishes blind inverse-edge deltas from
reads followed by writes. Two additions to a room's contents can apply to the
live list at publication without overwriting one another; a task that reads
contents records the corresponding dependency. Collapsing these into generic
whole-object updates would change contention and validation, even if the data
model looked simpler (db/store/store_txn_relationships.go:12 onward).

Values and retry state also have distinct ownership:

- List Set and slice copy element storage. Append can claim an unused backing
  frontier and return a new header; aliased prior headers stay unchanged.
  Elements returns a borrowed slice for iteration, not mutable ownership.
- Maps use small flat storage or a persistent index; indexed MapSet creates a
  new map/header and path rather than copying the entire logical map. Bulk
  loaded/read-only maps can retain a flat representation until an update.
- Size/finalizable metadata can require an initial scan and is cached. Avoid
  classifying every list assignment as a deep copy or every root check as a
  full scan without following the actual operation.
- RetryCheckpoint copies mutable VM stacks, locals, handlers and native
  continuation state, shares programs/value handles, and retains additional
  roots. It is not a disk persistence snapshot and does not reacquire session
  guards on Restore. A resumed slice takes this checkpoint before consuming
  its wake value. First-run tasks/forks can instead rebuild from their program.

Sources: types/list.go:168/185/296, types/map.go:522 and vm/retry.go:21/63.

VM admission lending/GC parking occur at tick checkpoints, amortized at the
1024-tick boundary (vm/vm.go:456-491). A long native builtin need not return to
that boundary while it runs. Regex matching, crypt and file scans therefore
need their own work/timeout analysis. Existing crypt checks configured work
limits and charges work ticks before hashing; cached PCRE matching sets a
two-second match timeout. Neither is proof that the entire builtin returns
within a scheduler quantum. argon2 invokes IDKey inline. No current workload
cost or new semantic defect is asserted from these observations.

Nested MOO calls have multiple existing execution paths. Ordinary bytecode
verb calls push frames on the active VM. Host CallVerbInContext creates a
separate synchronous VM sharing its parent's transaction and borrows/refcounts
the existing lease. Standalone CallVerb and sweep-owned hooks establish their
own context and ownership, with different resumability. Registered server hook
tasks and intrinsic Eval use the resumable task path. Those paths are not
interchangeable just because all eventually execute a verb. Sources:
engine/runtime.go:107, call_verb.go:26/200, task_factory.go:93 and eval.go:121.

### Primary-source comparison: output acceptance versus socket draining

Read the canonical installed WSL Debian Toast source at
/root/src/toaststunt/src, whose inspected commit subject was
`Fix build against Nettle 4.0`. This was source reading only: no server was
launched and no managed oracle behavior test was run in this pass.

server.cc:2920 bf_notify calls network_send_line/send_bytes. network.cc:1401
network_send_line calls enqueue_output, which copies bytes into a connection's
linked output queue and returns acceptance. Socket descriptors are made
nonblocking (network.cc:586); push_output handles would-block/partial writes
(line 316), and readiness handling drains queued output (line 1471).
enqueue_output may attempt a drain under queue pressure; it does not wait for
a slow peer to consume all queued bytes. Thus output acceptance and socket
completion are separate owners in this concrete oracle implementation.

This is a more useful reference boundary than moving all effects into an
arbitrary background worker. Ordered connection effects still need a defined
acceptance point; actual transport progress can have a separate lifetime.

The source comparison also exposes contract questions that must not be buried
inside a performance refactor:

- Toast passes !no_flush as flush_ok. enqueue_output consults it on overflow
  after attempting push_output: when it cannot make room, it either refuses
  the new output or discards queued blocks under its configured policy. Barn's
  Connection.SendNotification uses NoFlush to append to outputBuffer until
  Flush, and the local spec describes that interpretation. An existing Barn
  test explicitly expects that buffer behavior. Equivalence is not established.
- Toast network_buffered_output_length returns output_length bytes. Barn's
  Connection.BufferedOutputLength returns queued line count, and its builtin
  adds one per pending notification. The zero-argument builtin returns the
  constant 65536. These are precise source differences, not a managed
  conformance verdict or permission to change the API.
- Toast close_nhandle makes a best-effort push_output and frees remaining
  queued blocks. Barn BootPlayer synchronously sends its banner before Close.
  Boot/disconnect policy must be included when defining connection acceptance
  and completion; task return cannot automatically mean peer receipt.

Sources: Toast network.cc:648/1337/1401/1413 and server.cc:2920;
Barn server/connection.go:96/114/314, connection_manager.go:639,
builtins/connection.go:112, builtins/network_test.go:276 and
spec/builtins/network.md:79. Preserve source observations and use canonical
managed Toast conformance with capability_admission before selecting or
changing the uncertain observable behavior. Do not treat unit-test agreement
with Barn's current implementation as oracle evidence.

### Evidence and refactor readiness after the second pass

The core ownership sequence is now explicit:

1. Input ordering chooses a command; admission grants execution capacity.
2. The runtime acquires physical ownership and opens the attempt snapshot.
3. The VM runs; a threaded operation records a deferred start, while an inline
   irreversible operation first promotes/validates under the exclusive gate.
4. The runtime resolves the attempt: retry discards effects; successful
   publication makes those effects irrevocable, including native-exception
   slices whose writes commit.
5. Current code applies ordered effects while retaining the snapshot and
   physical/admission ownership, and sometimes the exclusive gate.
6. It releases/replaces the snapshot, releases the gate, publishes suspended
   VM state or terminal state, and relinquishes physical/admission ownership.
7. Completion callbacks and suffix/buffer flushing have their own order;
   terminal Done acknowledges the latter, not peer rendering or audio receipt.

This identifies bounded responsibilities for a refactor: attempt resolution,
ordered effect acceptance, socket draining, and lifecycle settlement. It does
not justify adding new facades or a general effect scheduler. In particular,
moving final output acceptance/draining and shortening a mid-slice irreversible
gate hold are different changes with different correctness requirements.

Known protections that must survive: discarded attempts launch no external
work; effects retain their encoded order and framing/target rules; external
completions cannot resume stale generations or a still-owned VM; callbacks
retain roots; Done follows terminal flushing; admission lending and GC parking
do not occur under an exclusive slice grant; checkpoint task/store state share
one boundary; root barriers cover recycle hooks.

Current implementation shape is now much better understood. A complete
behavior-preserving output refactor still needs the output policy/acceptance
contract above settled, plus a real slow-reader trace that correlates gate,
admission, physical ownership and unrelated progress. The broad system has not
earned a measured bottleneck ranking.

Historical artifacts inspected: the September pool application's baseline
workload output and the October deferred-GC concurrent CPU report under
experiments/. The latter includes runtime.gcDrain, runtime.memmove and
VM.executeLoop samples, but belongs to an older instrumented executable. These
files are leads for retained-memory/allocation analysis, not evidence of current
cost percentages or a current before/after result. Fresh source/binary/fixture
identity and settled workload outcomes are still needed for measurement.

- Trace actual ownership and producers before choosing optimizations.
- Separate transport capacity, input ordering, scheduler eligibility,
  admission, physical execution, publication and external-handle capacity.
  Each can delay work independently.
- Source analysis can establish blocking relationships; only matched runtime
  evidence establishes which relationship dominates a particular workload.
- Preserve semantics: MOO suspension, admission yielding and GC parking are
  different boundaries. Isolation-sensitive changes require the documented
  Elle workflow, raw histories and explicit verdicts, not ordinary tests alone.

## Surprises

- The checkout initially showed per-line reader waits and no output deadline;
  refreshed remote source already includes read-ahead and write deadlines.
  Those older observations must not be reported as current defects.
- Historical local notes describe newer investigations and isolated changes;
  their presence does not establish that every change is on remote master.

## Next step

PR #638 is merged. Publish the verified file-scan/capacity and frame-format slices
independently against current master, with exact-head CI and review.
Finish checkpoint capture/encoding tests and implementation, manifest hash
measurement and early-start failure repairs. Complete companion PR #177 merge
and publish later oracle suites only after their Barn dependencies are fixed.
Preserve raw histories, anomalies, oracle red/green captures and fixture identities.

Dynamic output capacity and rejectable notify acceptance remain demonstrated
defects. Resolve them in their existing owners before P3 changes physical
draining; default commit-deferred notify and mid-slice irreversible boundaries
must not be conflated. Existing output deadlines do not establish unrelated
publication progress. No new effect sequencer or parallel task model is allowed.

Both baseline and candidate binaries exist under .tmp/system-layering; the native
matrix credential-source question remains pending. Continue independent work.

P4-P9 bounded surveys and keep/change dispositions are now preserved in their
reports. Exec shutdown and blocking stdin cancellation still need a real
behavior/liveness reproduction before redesign. The source survey is broad,
not a claim to exhaustively audit every builtin, opcode or possible world program.

New workload captures, current failure frames, current Go/MOO profile rows,
and measured bottleneck rankings remain absent. Existing instrumentation is
the first source for those questions; do not add speculative machinery.
