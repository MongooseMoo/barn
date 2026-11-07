# Semantic persistence round-trip oracle

## Goal
Strengthen the existing CompareRoundTripStores owner so automated compute detects
supported persistent-state corruption, retaining the actual serialized input and
normalizing only documented format transformations.

## State
Implementation authorized by Q's make-it-so instruction. This isolated branch is
owned by the persistence worker; root owns publication, CI and merge.
The existing comparator now checks serialized values and metadata, relationships,
verb descriptors/source/program presence, player identities, reachable anonymous
objects and references, WAIF graph aliases and overrides, and pending finalizations.
The production reader and writer are unchanged. The fuzzer and barn -dump already
call this one comparator, so they acquire these assertions without a new target.

Focused tests from the new roundtrip_test.go file were run with this exact selector:
`go test ./db/format -run '^TestCompareRoundTripStores(WriterReader|TrackedDatabases|DetectsMutation)$' -count=1`.
Raw final output: `ok github.com/MongooseMoo/barn/db/format 2.207s`,
`focused_test_exit=0`. `git diff --check` produced no diagnostics and
`diff_check_exit=0`. Broad local tests were not run; PR CI remains root-owned.

## Decisions and why
- Extend the existing comparator, not a competing oracle or production algorithm.
- Read reader/writer/store contracts before selecting fields or normalization.
- Deliberate fixture mutations must prove each added field family is detected.
- Keep queued/suspended task preservation separate if it needs an independent API.
- Preserve all other work; use paper-reader only for any new literature work.
- Use Store.Snapshot for the existing anonymous reachability/serialization-ID
  normalization; do not reimplement allocation or graph traversal.
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
