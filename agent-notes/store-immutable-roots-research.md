# Immutable world roots: research, not an implementation proposal

## Goal

Explore candidate E from `store-ownership-exploration.md`: replace timestamped
object histories with persistent world roots without sacrificing fine-grained
serializable commits, current MOO behavior, diagnostics, or lifecycle handling.
Q requested forked Astra research before choosing. This agent owns only this note.

## State

Base convention: all line citations and measured file lengths in the original
research below refer to implementation base `cae320f3`. They are historical, not
current-line claims. The dated delta section immediately below refreshes the
conclusions against `b91e36f5`; its citations refer to that newer base.

Source inspected on 2026-10-26. `git rev-parse HEAD` returned
`206482cd2ee5a3715037893151333a5d6fb6fa23`; parent identifies implementation base
`cae320f3` and HEAD's extra commit as the earlier note. Before this note was added,
`git diff --name-only` returned only `agent-notes/store-ownership-exploration.md`.
No implementation, tests, benchmark, CI, or runtime experiments were performed.
Tests below are requirements read from source, not fresh passing evidence.

Read the task note, current AGENTS.md, `docs/elle-isolation.md`, and
`docs/workload-diagnostics.md`. Source reading covered directory/history,
snapshot acquisition and renewal, publication/validation, anonymous lifecycle,
renumbering, WAIF representation and pruning, reachability, checkpoint capture,
VM roots, irreversible boundaries, and their production call sites. This is an
architecture study, not a complete correctness audit or Toast conformance claim.

## Base refresh: 2026-10-26, cae320f3 to b91e36f5

Parent fetched and rebased the note-only work. This agent read
`git diff cae320f3 b91e36f5 -- db/store vm/op_property.go` and the affected current
publication functions. Raw revision probes during refresh:

    HEAD:          40cb17900379c30d26b3014e41395bc2a5a87f81
    origin/master: b91e36f5fe98aa665da9e4ae3ee2bb1f4e847c18

This refresh changes only this report. No code, tests, benchmarks or commits were
performed by this agent. The original gross/net estimates remain historical
planning ranges; the new WAIF work is additional migration surface, not additional
guaranteed deletions. The readTS change adds a benchmark and corrects padding
commentary; it does not replace the registry or establish root performance.

The meaningful architectural delta is class-property rename/delete propagation
to WAIF values, including live values absent from the transaction's earlier walk:

- `store_core.go:213-216` now stores weak WAIF handles by class and identity;
  `store_lifecycle.go:530` assigns `waif.WeakWaif()`. The class registry now permits
  traversal of live values rather than only tracking their identities/counts.
  Keep it weak: a root map of all registered strong WAIF values would alter liveness.
- `store_txn_properties.go:540` and
  `store_txn_property_inheritance.go:446` call `moveWaifValues` for rename/delete.
  At `store_txn_waif.go:165-196`, it walks descendant classes, resolves registry
  weak handles, and stages held values. The substitution at :146-157 first removes
  source keys, then installs destinations, preserving swaps rather than applying
  arbitrary map-key overwrites in sequence.
- `store_txn_waif.go:203-257`, `moveUnstagedWaifValuesLocked`, walks the newly
  published class graph and current class registry during commit. It skips
  identities already in tx.waifs and publishes moved images for other live WAIFs.
  Raw excerpts: `if _, read := tx.waifs[identity]; read { continue }` and
  `s.publishWaifLocked(&waifTxnImage{value: value, base: live, staged: properties}, ts)`.
  These are not limited to WAIFs reachable from persistent object-property values.
- `store_cow.go:466-475` calls that live propagation, publishes staged WAIF images,
  and only then advances the clock. The new test
  `waif_property_move_test.go:107-125` creates/registers a descendant-class WAIF
  after the renaming transaction's walk, then expects its a=40 to become c=40
  when that transaction commits. Test body read, not executed here.
- `vm/op_property.go:374-403` first tries `ReplaceWaifProperty` for a held value
  when containment permits. For an absent name it reads class existence and the
  class's colon-prefixed definition before reporting containment failure or writing.
  A root migration must preserve these dependencies and ordering, not infer that
  every WAIF write requires the same class lookup or that a WAIF image alone is
  its entire permission/definition context.

**Root scope and atomicity consequence.** The complete-world commit footprint is
larger than the original transaction's staged object and WAIF records. Rename/delete
has a commit-time extension over current live WAIFs of the resulting class subtree.
A full-root builder must include those images in the same final root as the class
definition changes, resolving the post-change graph through its private candidate
view. Calling the current helper unchanged while object changes live only in a
private root would make its `s.liveObjectLocked` walk use the wrong graph. This is
an adaptation of the existing operation, not permission for a new framework.

For the bounded numbered-only slice, keep WAIF timestamp histories and their
reader floor. Its shared-lock commit also needs the helper's class walk to use
the private candidate numbered graph before that root is exposed. Publish both
unstaged and staged WAIF images before exposing the
final numbered root at ts; snapshot acquisition must register that root's timestamp
under the existing floor gate. A numbered root swap alone is not whole-world
atomicity. The retained acquisition RLock still makes the coarse numbered path
coherent, but does not remove this shared-lock publication obligation.

Verdict is unchanged and sharpened: numbered roots remain a possible bounded
experiment; full roots still require WAIF identity/domain/lifecycle integration.
The new weak registry is an existing discovery mechanism to reuse, not a reason
to build a strong permanent WAIF directory. Pending-change ownership and roots
remain compatible, and both must preserve this commit-time propagation phase.

## Findings and recommendation

**Persistent roots can delete numbered-object history mechanics. They do not
eliminate change descriptions, validation, object ownership, or semantic GC.**
The strongest simplification is selecting a numbered snapshot by one retained
root and publishing several numbered images with one pointer store.

The complete-world variant is not ready for production selection. The most
difficult remaining design issue is not choosing a tree: WAIF values have mutable
reference identity, detached construction, store-domain binding, and lifecycle
semantics outside object storage. A root-owned WAIF map changes those ownership
relationships. Keeping the old WAIF mechanism avoids that change but also keeps
reader registration, reader floors, release handling, and WAIF history pruning.
Calling that partial result a world-root simplification would overstate it.

For code-size reduction, prefer the separately researched object-owned pending
changes first. These alternatives are compatible: pending changes describe D;
roots represent B and L. Root publication still needs rebased D. Neither depends
on implementing the other, and neither makes whole-image replacement correct.

## Evidence: what a root would actually replace

1. `object_dir.go:27-37,48-62` is a segmented dense-ID directory with 4096 slots
   per segment. Reads select a segment then atomically load an objectSlot pointer.
   It does not hash IDs. Slots remain allocated across recycling.
   Raw excerpt: `return seg.slots[int(id)&dirSegMask].Load()`.
2. `store_txn_snapshot.go:122-164` first tests the live image's version, otherwise
   captures a history slice under history.mu and scans backward for its readTS.
   Raw excerpt: `if history[i].ts <= tx.readTS { return history[i].obj }`.
   A root-based numbered read becomes a lookup in the retained root; the private
   `objects` cache and facet ownership still implement read-your-writes.
3. `store_txn_commit.go:291-350` holds publication exclusion through validation,
   preflight, map-key rebasing and publication. `store_cow.go:468-487` publishes
   WAIF images, then makes the commit timestamp visible with clock CAS.
   Raw excerpt: `if !s.clock.CompareAndSwap(ts-1, ts)`.
   A root must contain its publication timestamp; reading a root and a separate
   clock independently would reintroduce a torn acquisition window.
4. `store_cow.go:190-207,240-245` builds from live images and applies room deltas.
   Raw excerpts: `live := s.liveObjectLocked(id)`;
   `img.contents = applyContentsDeltas(img.contents, rel.contentsDeltas)`.
   Keep this rebase. Starting a persistent builder from B and publishing that
   builder after another writer commits loses unrelated changes.
5. `store_txn_mapkeys_test.go:82-99` requires different-entry writers to both
   commit and retain a=2, b=11. A whole-root equality check would reject the
   second writer; swapping its B-derived root would lose the first writer.
   Existing fine read sets and `rebaseMapKeyWritesLocked` remain necessary.

Root commit model, as a design inference:

    acquire existing publication exclusion
    L = current root
    validate recorded selectors against L
    preflight resolved edits against L
    materialize affected immutable images by applying D to L
    install those images in a persistent builder derived from L
    freeze one root containing the new timestamp and all included domains
    publish root, then release exclusion

Do not add a root-CAS retry protocol: writers are already excluded during this
window. Do not serialize complete MOO task execution. Do not replay callbacks,
permissions, time acquisition, SQL, or network effects during materialization.

## Retention: precise comparison with today's implementation

`store_history_gc.go:228-255` already keeps images whose validity interval
contains a live reader timestamp, plus the newest entry and entries above the
sampled clock. The test at `history_reader_prune_test.go:74-85` explicitly keeps
only timestamps 12 and 109 from 100 versions for readers 12 and 200:

    if got := entryTimes(pruned); !slices.Equal(got, []uint64{12, 109})

Therefore an old root does NOT uniquely solve "one old reader pins all subsequent
numbered versions". With retained roots at the same timestamps, the object images
needed by those roots approximate the union already selected by reader-aware
pruning. Both preserve images the reader has not yet consulted. Unchanged objects
share pointers; roots do not copy the entire world per transaction.

Differences that need measurement:

- Current object pruning is intermittent (`readerPruneDue`, power-of-two lengths
  starting at eight), and floor pruning can retain obsolete backing arrays via
  slices. Roots naturally discard unreferenced intermediate paths at Go GC time.
- Roots add tree path nodes and pointer scanning. Distinct retained roots with
  one updated object per root can each keep a copied path even when most images
  are shared. If R roots reference a world of N objects, storage is the shared
  world plus unique changed images and retained paths, not R full copies of N.
- A many-write commit can copy common paths once only if its builder tracks
  exclusive node ownership correctly. Repeated immutable Set calls multiply
  path copying. A full 4096-pointer leaf copy costs 32 KiB on a 64-bit machine
  before headers or higher-level copies; this is arithmetic, not a measurement.
- Dropping a root permits reclamation, not immediate reclamation. A reachable
  released transaction retaining its root can pin a world version. Release should
  clear that reference when its read contract ends; terminal error handling must
  retain whatever private view it is still allowed to use.
- Current WAIF pruning is floor-only (`types/waif_image.go:64-95`), unlike numbered
  reader-aware pruning. A properly integrated root map could remove intermediate
  WAIF images too. A numbered-only migration cannot claim that benefit.

**Diagnostics are a separate retention requirement.**
`store_history_gc.go:278-280` deliberately skips reader-aware pruning while the
conflict census is tracking. `store_conflict_census.go:322-364` walks intermediate
object images to date the first rewrite, because an inherited property's slot
version is not necessarily its publication time. Raw excerpt:
`return at <= sampled` after `rewrittenAt(history[i].obj)`.
Retaining only roots held by tasks loses this evidence. Preserve the existing
opt-in image journal while tracking, or introduce equivalent per-slot rewrite
provenance; neither is free. The smallest compatible root migration keeps the
diagnostic journal and its bounded lifetime, without using it for ordinary reads.
Deleting this diagnostic is not an acceptable line saving.

## Coarse mutation: keep the existing acquisition lock

Design rule: persistent roots do not require lock-free root capture. Retain the
existing s.mu.RLock around capture. The first draft unnecessarily required a
private coarse builder; parent identified this smaller option, and source reading
of BeginSnapshot and live adoption supports it.

`store_core.go:497-516` does not first finish a private image and then publish it:

    fresh := cloneObjectForReadTxn(old)
    s.publishLocked(old.id, fresh)
    ... retain old ...
    return fresh

Its callers then mutate fresh while holding store.mu exclusively. Existing
snapshot acquisition/first loads take that lock shared. `ChangeParents`,
`store_relationships.go:83-97`, republishes a parent, then
`attachChildToParentsLocked` loads it again and mutates its children list.
`setVerbArgs`, `store_verbs.go:369-387`, has its own publish-before-edit path.

Simply replacing publishLocked with atomic root publication AND allowing readers
to capture roots without s.mu exposes an unfinished image. That second change is
unnecessary. Coarse methods can continue path-copying the current directory as
they install fresh images under s.mu.Lock, then mutate those fresh images before
unlocking. Previous captured roots still point at the old immutable images; no
new snapshot can capture an intermediate root while the exclusive lock is held.
The guarantee is immutability of every image reachable from a captured root,
not immutability of inaccessible coarse working roots. `AdoptLiveObject`,
`AdoptLiveVerbs` and `AdoptLiveRelationships` also take s.mu.RLock
(`store_txn_live.go:37,75,127`), so retain their current live-read coordination.

Ordinary shared-lock commit is different: readers can acquire RLock beside it.
It must accumulate directory changes privately and install only the completed
root, with the matching timestamp. This can use the existing commit image set;
coarse helper rewrites are not a prerequisite. A builder may later improve wide
coarse update allocation, but that is a measured optimization, not a new design
requirement. Anonymous objects and WAIFs remain the separate caveats below.

Keep root timestamp coherent even for a coarse operation that only bumps the
clock or a verb-shape epoch. `store_core.go:387-406` has those clock paths, including
one post-publication bump under the shared commit lock. Root/clock publication
must account for them: update the root descriptor's timestamp with the unchanged
directory when needed. Do not independently sample root and clock under RLock
and assume it excludes the ordinary shared-lock committer. Register the root's
timestamp before WAIF pruning can pass it, preserving the current floor gate.

The mutation inventory includes createAnonymous, Recycle, Recreate, Renumber,
ChangeParents/reseeding, verb add/delete/info/args, Add/AddAnonymous loading,
ordinary `publishStagedLocked`, and direct WAIF writes. A method-level root swap
must respect existing prefix boundaries, not merge all builtins in a task.

`engine/task_runtime.go:296-337` takes the exclusive gate and calls
`CommitAndRenewCarryingReads` before irreversible effects. It publishes pending
forks/effects after successful boundary publication. Roots do not make those
effects rollbackable and do not remove live-adoption or carried-read logic.

## Root ownership inventory

| State | Required treatment |
|---|---|
| Numbered objects, tombstones, relationship slices, property tables, verb nodes | Immutable images under the root; existing facet copies and version stamps stay. |
| Anonymous objects | Full world roots require immutable images keyed by permanent runtime identity, separate from above-max dump IDs. A numbered-only root retains the current separate behavior. |
| WAIF properties | Full roots require immutable images resolved through the selected root/domain; retaining a WAIF pointer alone does not freeze its properties. |
| Publication timestamp and semantic epochs | Pair coherently with the root; derived caches may remain outside with explicit root/version validity. |
| Allocation reservations/high-water | Do not roll reservations back with a root. Keep the monotonic allocator outside; preserve the existing captured watermark contract. |
| max_object and recycled-ID metadata | Capture coherent published metadata or retain the existing locking/watermark contract; do not infer all counters are snapshot data. |
| Pending finalizations | A full checkpoint view must freeze this queue along with its graph, or keep the capture barrier. Current setters mutate it under s.mu without transaction commits. |
| Task stacks, suspended/queued snapshots, retry roots, active connections | External to an object root; retain runtime capture coordination and value rewriting. |
| Locks, metrics, conflict counters, weak WAIF registration, memo structures | Not world state; preserve their capabilities rather than putting arbitrary operational state in a persistent map. |

Creation/recycling/renumbering source constraints:

- `store_lifecycle.go:10-56` gives anonymous creation a permanent allocator ID,
  installs it in anonObjects and updates numbered parents. `store_core.go:638-660`
  raises highWater for loaded anonymous IDs but not maxObject. Runtime identity
  and dump ID must never become interchangeable keys.
- `store_lifecycle.go:67-175` recycles by updating children, contents, parents and
  tombstone. `Recreate:179-217` can reuse a numbered slot. Preserve the old root's
  image at that ID while the new root points to a new incarnation/tombstone.
- `Renumber:305-511` publishes an old-ID tombstone and new-ID clone, rewrites
  structural references and owners, and directly edits anonymous objects. The
  owner rule clears owners equal to destination and rewrites source to destination.
  This is not moving one persistent-map entry. It remains a potentially wide scan.
- `store_txn.go:135-175` supports explicit readTS; production callers found by
  search all pass 0. Tests use explicit timestamps, including future ones in
  `waif_prune_watermark_test.go:53-56`. Retained root handles are not an arbitrary
  historical-time API. Do not silently emulate explicit timestamps with current
  root or add an indefinitely retained root index; resolve the exported contract
  explicitly if removing it. Existing history is itself pruned, not a permanent
  historical database.

## Why complete WAIF integration is the hard part

`types/waif.go:34-43` places class, owner, properties, domain and image chain in
waifRep. Identity equality is a 128-bit token, not a property-map pointer.
`SetProperty:105-115` permits construction only while domain is nil.
`types/waif_image.go:28-45` binds a detached value to a store's domain on first
image access, creating a timestamp-zero image. `PublishWaifImage:49-61` also
updates the shared latest properties view.

In particular:

    w.domain = domain
    w.images = []*WaifImage{{properties: maps.Clone(w.properties)}}

A WAIF can exist solely in VM state before any store access. A world root captured
before its first binding has no entry for it, yet the current API can obtain its
detached baseline. Full roots therefore need a precise fallback for immutable
detached baseline and domain ownership, not "missing entry means nonexistent".
An old root must use that baseline when it predates first publication, including
first access after another transaction bound and edited the value.

A strong root map of every WAIF ever touched creates a further problem: its values
retain property graphs independently of whether the identity is live. Today's
history index deliberately uses `types.WeakWaif`, and RegisterWaif uses cleanup
registration (`store_lifecycle.go:515-543`). Keeping all entries permanently would
retain dead property graphs; automatic Go GC cannot remove strong map entries.
Root entries therefore need semantic retirement or carefully designed weak
identity membership. Weakness alone must not make an image disappear while a
snapshot can first dereference an identity it holds.

`VisitRetainedWaifValues`, `types/waif_image.go:115-134`, walks every retained image
for semantic GC. `store_reachability.go:12-31,274-290` explicitly chooses retained
versus current traversal. `engine/waif_lifecycle.go:31-40` combines persistent
roots, siblings and VM roots. `vm/value_roots.go:113-168` includes retry state,
frames, stack, pending roots, fork data and `StoreTxn.VisitWaifValues`.

With root-owned images, those walks require a selected-root resolver and must
account for all roots that can still expose old edges. Go heap reachability is
not an enumeration API for semantic liveness. An active-root inventory, retained
edge summary, or integration with all runtime-owned snapshot handles is still
required. That may replace the timestamp registry with a different registry,
not eliminate lifecycle bookkeeping. Retaining every old-root edge conservatively
can delay finalization; changing that delay is not automatically behavior parity.

This is a specific incompleteness in the full-root design: a small map API cannot
simultaneously specify detached first access, domain rejection, identity aliasing,
snapshot-visible images, retirement, and :recycle eligibility. The research does
not claim such a design is impossible; it declines to hide these contracts behind
an unimplemented "resolve WAIF" abstraction.

## Checkpoint/export is not deleted

`SnapshotWithRoots`, `store_snapshot.go:68-155`, takes the gate, captures pending
finalizations, chooses anonymous serialization IDs, and rewrites values. Its
rewriter at :308-331 creates WAIFs with preserved identity and recursively captures
current properties while the barrier is held. `db/format/checkpoint_stage.go:63-96`
collects external queued/suspended task values, rewrites them with the same plan,
then serializes. These operations remain even if a root replaces the object walk's
barrier. `writer.go:339` still consumes `waif.GetProperty`, so root-derived output
must provide frozen WAIF images to that interface or change its resolver.

A numbered-root capture can shorten neither the external-task coordination nor
the WAIF freeze merely by reading one pointer. A full root could freeze store
images cheaply, but queued/suspended task ownership, pending-finalization overlap,
anonymous identity remapping, file publication and recovery remain separate work.
Do not count all 451 lines of store_snapshot.go as removable snapshot machinery.

## Persistent directory choices checked against primary source

Sources were read as code, not papers. URLs below are mutable upstream branches,
inspected 2026-10-26; any implementation would pin a revision before depending on it.

1. **Small radix vector indexed by numeric ID.** Best conceptual fit for Barn's
   current dense numbering. A 32-way trie needs about ceil(log32(N)) routing levels;
   a batch builder can own/copy each changed path once. This complexity statement
   is analytical; no Barn speed claim follows. Holes from anonymous allocation and
   aborted creates must remain sparse or be explicitly represented. A 4096-entry
   leaf preserves fewer indirections but pays the 32 KiB copy noted above.
2. **benbjohnson/immutable List/Map.** [Source](https://github.com/benbjohnson/immutable/blob/master/immutable.go)
   :276-334 uses five-bit List nodes and copies branches for immutable Set; builders
   mutate owned nodes. Its public ListBuilder starts empty (:222-224), not from an
   existing List. Per-write Set on a retained list is usable but repeatedly copies
   paths; do not assume the builder is a ready-made batched update of old roots.
   Lists also require bounds, whereas Barn can allocate sparse IDs. Map avoids
   dense fill but adds hashing; map iteration order is not ascending ID. Importing
   the package moves implementation lines into a dependency, not out of existence.
3. **Hashicorp immutable radix tree.** [Source](https://github.com/hashicorp/go-immutable-radix/blob/master/iradix.go)
   :74-93 derives transactions from roots; :135-182 caches owned nodes and copies
   prefixes/edges. This fits batch updates but carries byte-key encoding, copied
   variable edge arrays, LRU ownership tracking and mutation channels. Barn's
   integer-indexed lookup does not need most of those features. A typed bespoke
   vector may have lower overhead; that remains an experiment, not a conclusion.
4. **go-memdb is a publication example, not a drop-in transaction model.**
   [memdb.go](https://github.com/hashicorp/go-memdb/blob/master/memdb.go):69-78 locks
   the writer when its transaction begins; [txn.go](https://github.com/hashicorp/go-memdb/blob/master/txn.go):135-159
   publishes a root then unlocks. Barn must retain its optimistic task execution
   and take exclusion only around validate/rebase/publish. Both sources warn by
   implementation/contract that values themselves must remain immutable.

No library handles Barn's anonymous identity or WAIF semantic GC for it.

## Smallest bounded migration and complete-world extension

**Bounded numbered-root slice**, if selected after separate authorization:

1. Introduce only a persistent numbered directory and a root {directory,timestamp};
   keep current private images, deltas, validation, anonymous state and WAIF domains.
2. Keep root capture under s.mu.RLock. Coarse publishers retain their current
   clone/install/mutate pattern under s.mu.Lock, replacing directory entries by
   path copying. Ordinary shared-lock commits collect all directory changes
   privately and publish one final root. Pair timestamps coherently, including
   clock-only/shape-epoch changes. No extra coarse builder is required initially.
3. Capture the root in BeginSnapshot; read numbered objects from it. Keep floor
   registration for WAIF history. Replace ordinary numbered history lookup/pruning
   with root retention; preserve the conflict-census journal when tracking.
4. Update hot/boundary renewal and release to carry/drop the correct root without
   discarding old private error views. Keep DirectTxn a live operation interface.
5. Preserve checkpoint barriers and existing anonymous behavior in this slice.
   Do not claim a complete immutable-world snapshot or remove GC machinery.

This is coherent but modest in mechanism deletion. It can be built independently
of object-owned writes and can be rejected after measurement without adopting the
full architecture. No prototype is authorized by this note.

**Complete-world extension** requires, in addition, immutable anonymous images;
root-resolved WAIF images and detached fallback; no mutable referent escapes in
committed views; root-aware semantic-liveness and retirement; frozen pending
finalizations; checkpoint/external-root coordination. Those are substantive design
work, not cleanup after the numbered slice. Until a liveness ownership decision
is selected, there is no honest smallest *complete-world* production change.

## Deletion ledger and size estimates

Measured physical file lengths (comments/blanks included): object_dir.go 135;
store_history_gc.go 368; store_txn_waif.go 250; store_txn_snapshot.go 365;
store_core.go 728; store_cow.go 532; store_snapshot.go 451.
These are affected surfaces, not deletions.

| Scope | Gross removal estimate | Replacement/addition estimate | Net estimate |
|---|---:|---:|---:|
| Numbered-root slice | 250-450 lines | 200-400 lines | 150 added to 250 removed |
| Full roots, including WAIF/lifecycle integration | 650-950 lines | 850-1600 lines | 950 added to 100 removed |

All ranges are unmeasured planning estimates, not a diff, quota, promise, or
independent per-file budgets. Full scope includes the numbered slice. Wider
call-site churn is likely larger than the net count; no defensible exact figure
exists before selecting the lifecycle design. A library can make local LOC lower
by moving tree code outside Barn; it does not make that algorithm free.

Gross-removal candidates: numbered history reads and inserts, object history
pruning, slot atomics/directory implementation; in the full variant reader-floor
registration and WAIF version-chain/heap pruning. Retained replacements: root
builders for ordinary shared-lock commits, root acquisition/release, optional diagnostic
journal, WAIF identity/resolution/retirement and semantic retained-root accounting.
Never count validation, resolved changes, MOO operations, serialization rewriting,
tests or diagnostics as deleted merely because they need new owners.

## Proof obligations and existing-workflow experiments

Before any correctness/performance claim, use existing PR CI, not a new local
pre-push gate. `docs/elle-isolation.md` pins elle-cli
`9c2a18b3719f573dab06ae05c955c9c9d5225673` and requires raw histories, manifests,
nonzero selected path counts and explicit healthy/fault verdicts. Root publication
still must expose timestamped/validated pause points meaningfully and preserve
the commit-exclusion and map-rebase fault witnesses.

Existing source cases that a migration must continue to satisfy:

- Different-key writes both commit; same-key writes conflict; captured-map
  observations retain the original dependency (`store_txn_mapkeys_test.go`).
- Multi-object snapshots and checkpoints never tear (`store_snapshot_atomic_test.go`,
  `commit_atomicity_test.go`, Elle torn-snapshot pair).
- A released/dropped task stops retaining snapshots; a live old reader still sees
  its selected images (`history_reader_prune_test.go`, `cow_history_gc_test.go`).
- WAIF references hidden in old images remain semantically live while readable;
  current-only checkpoint ownership excludes historical edges (`store_txn_waif_test.go`,
  WAIF prune tests, `engine/run_gc_task_roots_test.go`).
- Anonymous creation, recycle, renumber and external task serialization keep
  identity distinct from numbering (`anon_concurrent_race_test.go`, renumber tests,
  checkpoint-stage tests). These test names are corpus pointers; only cases quoted
  earlier were read in detail here, and none was run.
- Direct readers, shared facets, coarse publication and renewal retain their
  existing contracts; error prefixes and irreversible effects retain boundaries.
- Conflict-census first-rewrite classification remains available when enabled.

Measurement plan, after Q selects/authorizes an experimental implementation:
compare the same base and candidate using existing Direct, CommitScratch,
TxnPropertyWriteCommit, TxnReadOnlyCommit, ancestry, VerbArgsPublication and
ChparentReseed benchmarks. Record ns/op, B/op and allocs/op for narrow and wide
commits; inspect retained heap separately from allocation churn. Existing
`BenchmarkPruneWaifHistoryLargeRetainedSmallPrunable` describes the old WAIF
mechanism's cost, not a fair standalone full-world comparison. Reuse the documented
Mongoose workload with fixed fixture/binary hashes and valid command completion;
inspect saved exception frames, Go/MOO profiles, scheduler diagnostics and census
before attributing a change to the directory. Historical no-twins measurements are
not current-head performance proof.

Missing empirical evidence: root lookup cost, path-copy churn, retained-root heap,
GC pause effects, wide-commit builder behavior, checkpoint duration, semantic-GC
cost and real-workload throughput. Existing microbenchmarks do not fully isolate
the retained-root matrix; report that limitation rather than quietly adding new
guards or declaring parity from a narrow benchmark.

## Decisions, surprises, and next step

Decision: recommend neither a dependency import nor a full rewrite for code
shrinkage on present evidence. The bounded numbered slice is a possible experiment,
not the most promising immediate simplification. Root ownership and pending-change
ownership solve different problems and can coexist.

Surprises: current numbered history already removes versions between readers;
coarse code publishes then mutates under its lock; diagnostics intentionally need
intermediate versions; full root snapshots require semantic-liveness accounting
even though Go automatically reclaims unreachable allocations.

Substantive unresolved choice for Q, if pursuing E: keep the bounded numbered-only
architecture with existing WAIF floors, or undertake a larger lifecycle/resolver
change whose net code may grow. That choice cannot be settled by tree benchmarking
alone. Explicit historical readTS compatibility also needs a decision if the API
changes; production callers found here use current snapshots.

Next step: parent checks the concrete source claims, compares with the pending-write
report, and presents the tradeoff. No code until Q says "make it so". No tests,
commits, publication, or external messages were made by this research agent.
