# Semantic persistence round-trip oracle

## Goal
Strengthen the existing CompareRoundTripStores owner so automated compute detects
supported persistent-state corruption, retaining the actual serialized input and
normalizing only documented format transformations.

## State
PR 655 follow-up fixes the exact red diagnostic supplied by root:
`db/format/roundtrip_test.go:41:3 QF1003 could use tagged switch on i`.
The fixture setup now uses a tagged switch. The private review's invalid-class
WAIF/anonymous-override edge was proved with accepted raw input produced by the
real writer and a class-flag-only mutation. Before the comparator correction,
the focused fixture printed `accepted/reloaded snapshot anonymous objects: 1/0`,
then `round-trip verification failed: reachable anonymous objects 1 != 0` and
`focused_test_exit=1`. This established a false positive in the new comparator,
not a defect in the reader/writer's documented normalization.

Anonymous objects are now compared as the persisted-field value walk reaches
them, with forward/reverse identity maps that preserve aliases and cycles while
allowing serialization IDs to shift. This uses the existing semantic traversal,
not another copy of the snapshot allocation/reachability algorithm. A stronger
fixture additionally retains a directly rooted anonymous object whose ID shifts
when the invalid-class override disappears; it checks the first and second dump.
Missing objects, anonymous content changes, alias splits and alias merges remain
deliberate failing controls. No store, reader or writer production code changed.
The task-retention research below is preserved.

Implementation authorized by Q's make-it-so instruction. This isolated branch is
owned by the persistence worker; root owns publication, CI and merge.
The existing comparator now checks serialized values and metadata, relationships,
verb descriptors/source/program presence, player identities, reachable anonymous
objects and references, WAIF graph aliases and overrides, and pending finalizations.
The production reader and writer are unchanged. The fuzzer and barn -dump already
call this one comparator, so they acquire these assertions without a new target.

Focused tests from the new roundtrip_test.go file were run with this exact selector:
`go test ./db/format -run '^TestCompareRoundTripStores(WriterReader|InvalidWaifClass|TrackedDatabases|DetectsMutation)$' -count=1`.
Raw final output: `ok github.com/MongooseMoo/barn/db/format 2.116s`,
`focused_test_exit=0`. `git diff --check` produced no diagnostics and
`diff_check_exit=0`. Broad local tests were not run; PR CI remains root-owned.

## Decisions and why
- Extend the existing comparator, not a competing oracle or production algorithm.
- Read reader/writer/store contracts before selecting fields or normalization.
- Deliberate fixture mutations must prove each added field family is detected.
- Keep queued/suspended task preservation separate if it needs an independent API.
- Preserve all other work; use paper-reader only for any new literature work.
- Use Store.Snapshot for the existing anonymous reachability/serialization-ID
  normalization; do not reimplement allocation or root-collection algorithms.
  Snapshot includes references inside WAIF overrides that the writer omits.
  Only anonymous nodes reached by the comparator's persisted-field walk belong
  to the semantic graph being checked. Bind their identities bijectively; do
  not require the original and reloaded snapshot anonymous counts/IDs to match.
- Compare strings case-sensitively and scalar tags exactly. Preserve signed zero;
  the native textual float representation has no NaN payload identity.
- Mirror only actual format transformations: recycled/missing placeholders;
  argument/preposition code normalization; alias join/trim/split; inherited slot
  layouts; omitted CLEAR payloads and synthesized missing CLEAR slots; anonymous
  serialization identities; WAIF overrides limited to a valid class's colon
  definitions. WAIF durable identities require a graph bijection because portable
  dumps do not carry the optional Barn identity sidecar.
- Derived cache fields, property Defined flags, and unreachable anonymous objects
  are not serialized fields. Definition names/order/count establish local slots.
- Keep the existing fuzzer's first-dump slot-layout normalization. Real tracked
  Test_fresh2.db, Anon6.db and toastcore.db pass semantic second-dump idempotence.
- The constructed fixture also passes its first write/reload, including nested
  maps/lists, BOOL, signed zero, NaN, CLEAR/NONE sentinels, inherited CLEAR slots,
  programmed/empty/unprogrammed verbs, two anonymous self-referential objects,
  distinct and shared WAIFs, WAIF cycles, undeclared WAIF overrides, and a prep
  alias. Per-family mutations fail the existing comparison with field witnesses.

## Surprises
The portable reader's SuspendedTask already embeds task.Snapshot, while its
QueuedTask has separate fields (reader.go). Writer.SetTaskSnapshots accepts
scheduler task.Snapshot records. Checkpoint.Capture additionally owns roots and
TransformPersistenceValues remapping. Queued restoration is owned by
engine/task_load.go. Task retention needs its own focused change sharing those
actual owners, rather than a test-only copy of scheduler restore and checkpoint
logic. Accepted queued/suspended tasks are still omitted by the
current FuzzReadDatabase dump helper; this PR does not claim continuation coverage.
Connections and sidecar durable WAIF identity persistence also remain outside the
store comparator.

One read-only rg invocation mistakenly put a wildcard in its Windows path; another
named an unverified runtime directory (the actual owner is engine). Both reads
failed without changing files. The worker notified root to coordinate the existing
rule's requested mechanical guard. One apply_patch delete/add attempt was rejected
before modification; the verified update patch then applied. The task-boundary
report was corrected after reading SuspendedTask's embedded Snapshot field.

## Next step
Root reviews the scoped diff, publishes an independent PR, runs PR CI, and handles
normal merge. Follow with a separate task-record retention oracle using the actual
checkpoint and restore contracts. No new Barn defect is claimed from these
deliberate mutation controls. Native fuzzing compute is needed to seek real cases.
The follow-up commit contains the lint repair, proved comparator-normalization
repair and this preserved research note; root owns PR 655 publication and CI.

## Next focused PR: retain parsed tasks in the database fuzz round trip

Research only; no task-retention implementation is included in the comparator
commit. The source read for this plan is reader.go, reader_task.go,
reader_fuzz_test.go, writer_task.go, writer_vm.go, checkpoint_stage.go,
task/snapshot.go, engine/task_load.go and engine/restored_fork_environment_test.go.

### Actual contract and dependency boundary

- Database.QueuedTasks contains the retained native fields ID, StartTime, This,
  Player, Programmer, VerbLoc, Verb, Variables and Code. readQueuedTask currently
  discards the header's firstLine and runtime-environment declaration order;
  readQueuedTaskActivation also discards native placeholders and typed temporary
  fields before retaining its scalar activation descriptor. The oracle must not
  claim preservation of those already-discarded raw fields.
- Database.SuspendedTasks contains task.Snapshot directly. That includes ordinary
  suspended, ready-to-resume queued VM and interrupted tasks. SetTaskSnapshots
  routes interrupted snapshots using IsInterruptedForPersistence; preserve that
  categorization rather than counting only ordinary suspended output.
- engine.loadQueuedTask is the execution-restoration owner: it compiles saved.Code,
  rejects compiler diagnostics and ambiguous case-folded saved locals, restores
  only the recompiled program environment plus command variables, substitutes
  time.Now for nonpositive StartTime and queues a running task. Calling it from
  the raw-reader fuzzer would filter accepted input and introduce runtime work.
  It also cannot be imported by package format: engine already imports format.
- Writer.SetTaskSnapshots accepts []task.Snapshot. ForkSnapshot contains Variables,
  VariableNames, SourceLines and FirstLine. writeRtEnv requires every variable
  name exactly once with matching map membership. writeQueuedTask rejects empty
  source or nonpositive FirstLine. It rounds StartTime by adding 500ms before Unix;
  reader timestamps are already integral seconds, so use time.Unix without the
  runtime loader's wall-clock substitution.
- Checkpoint.Capture already owns task root collection and rewriting through
  Snapshot.TransformPersistenceValues and Store.SnapshotWithRoots. This is the
  algorithm to share, including anonymous objects reachable only through task
  locals, constants, stacks, native continuation metadata and activation values.
  Snapshot is a value containing owned maps/slices; TransformPersistenceValues
  modifies those aggregates. Any consuming helper must explicitly own its parsed
  database/task records so an expected value cannot be silently overwritten.

### Smallest implementation boundary

1. In checkpoint_stage.go, extract the existing root-collection/snapshot/rewrite
   body into one package-private function taking the store and the two task
   snapshot slices and returning store.Snapshot. Capture calls that function,
   then retains its current writer setup. The fuzz helper calls the same function
   and writes to bytes.Buffer. This avoids a second root walker, filesystem I/O
   per fuzz iteration, a new exported checkpoint API, or a new package dependency.
2. In a directly related format task owner, add a package-private parsed-queued
   record adapter to task.Snapshot. Transfer all retained descriptor fields and
   source without compilation; sort the existing variable-map keys to provide
   deterministic VariableNames, use FirstLine 1 because the current reader has
   discarded that header field, and construct the activation from the retained
   scalar descriptor. Do not copy engine's compiler/name-matching/queueing logic.
   This adapter has a different contract from runtime execution restoration;
   engine/task_load.go remains unchanged. Canonical FirstLine 1 is a limitation
   of the retained reader representation, not evidence of native equivalence.
3. In reader_fuzz_test.go, carry *Database through both dump/reload passes rather
   than converting it permanently to *Store. Retain queued records and embedded
   suspended snapshots, feed them through the shared checkpoint snapshot helper,
   SetTaskSnapshots and SetActiveConnections, then parse the actual output.
   Empty queued bodies that the parser accepts but the existing writer rejects
   must remain an explicit serialization error; do not skip them or infer a
   native execution result. Keep raw corpus bytes as the minimized reproducer.
4. In roundtrip.go, reuse roundTripComparison's one identity context for the
   captured store snapshot AND task payloads. Include task-only anonymous roots
   when constructing both snapshots; plain Store.Snapshot would omit them.
   Compare queued records by ID, retained activation descriptor, integral time,
   source, exact variable-name set and case-sensitive typed values. Start the VM
   task family with category, ID/time, wake/task-local values, activation count,
   frame source/IP, local names/values, operand stack and call descriptors. Extend
   remaining native continuation/VM metadata only with a mutation fixture for
   each added field family; do not label the first family complete VM coverage.
   Use direct typed accessors and the existing value comparer, not re-encoded
   bytes or reflection over process identities. One context must span store and
   task values to detect WAIF alias splitting between those surfaces.
5. Add task-oracle tests in one owned test file and update this living note.
   Source changes are limited to checkpoint_stage.go, reader_fuzz_test.go,
   roundtrip.go and one directly related task-record adapter/test owner. No new
   module, dependency, package cycle, runtime, scheduler or store algorithm is
   required. The comparator PR must be reviewed first; the next PR starts from
   the default branch after integration, rather than stacking this branch.

### Positive and regression design

- Feed a real writer-produced database containing both a queued fork and a
  suspended VM through the raw fuzz path; require nonzero task counts after
  both passes. Include an interrupted snapshot to prove section routing.
- Put an anonymous self-referential object ONLY in queued locals and another
  ONLY in suspended locals/stack. Assert objects and references survive both
  passes. The old store-only helper loses them: that is the first regression.
- Share a WAIF between an object property and task locals and between two tasks;
  prove the common identity context rejects deliberate alias split and merge.
- Mutate one retained descriptor, source line, variable type/value/case, task ID,
  timestamp, suspended category, wake/task-local value, frame IP/local/stack value
  in the reloaded result. Each mutation needs a specific nonempty witness.
- Cover zero/negative integral timestamps without wall-clock dependence,
  noncanonical saved variable spellings without engine recompilation, empty
  bodies that the writer currently rejects, and first-dump normalization followed
  by semantic idempotence. Preserve native parse/write errors; never suppress
  a source exception to manufacture a passing corpus.
- The first PR's fixture checks portable persistence, not execution or a Toast
  behavioral claim. If implementation reveals uncertain MOO behavior, the
  pinned managed WSL Toast command plus capability_admission must establish the
  expected behavior before changing Barn's runtime. No such runtime change is
  part of this plan.

No tests were run for this research-only plan. The earlier comparator's focused
test output above remains the only local test evidence.
