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

## Central ownership hypothesis, investigated 2026-10-26

Question from Q: can one ownership model cover numbered objects, anonymous
objects, WAIFs and retained snapshots, preserve behavior, and eliminate enough
machinery to make the whole store simpler?

This extension uses implementation base `b91e36f5`; all citations in this extension
refer to that base. Before reading, `git rev-parse HEAD` returned
`9f66116e1e92c4ab4deb1093baf4901aa8332aac`. The report was clean. This extension
changes only this file. No executable experiment or implementation was performed.

**Answer: one shared publication/image discipline is coherent, but one uniform
ownership or reachability relation does not preserve these contracts.** The
strongest counterexamples are actual lifecycle producers, not rare hypothetical
interleavings: a WAIF awaiting :recycle must remain allocated without counting as
a live MOO reference; an old WAIF edge can protect a value from semantic GC while
the checkpoint must exclude that edge. A root object can contain all those rules,
but containing them is not deleting them.

I also tested a different unification, versioned entity handles instead of world
roots. It avoids a strong global WAIF image map but retains history registration,
pruning, weak discovery, root classification and export freezing. It might share
small image-selection helpers; source does not establish that it simplifies the
whole store. This revises neither candidate into an implementation recommendation.
The earlier estimate that complete roots may grow code remains an estimate, not
a theorem: the concrete conclusion is that its large proposed deletions have
replacement obligations which have not been eliminated.

### Five distinct questions, with real producers

| Relation | Authority/producer in current source | Why it cannot be inferred from the others |
|---|---|---|
| Go allocation reachability | `types/waif.go:34-43,166-176`; Value holds waifRep, cleanup unregisters it | Pending finalization deliberately retains a strong Value. |
| Snapshot image visibility | `store_txn_snapshot.go:122-164`, `types/waif_image.go:28-45` | Same identity yields different payloads at different read timestamps. |
| Identity and publication domain | `types/waif.go:80-88,139-163`, `waif_image.go:32-38` | Equality of identity does not mean two detached/export payloads are the same mutable allocation or share a store domain. |
| MOO semantic liveness | `vm/value_roots.go:5-10,131-136`; `engine/waif_lifecycle.go:309-321` | Pending versus live roots have different eligibility despite both retaining memory. |
| Checkpoint ownership/capture | `db/format/checkpoint_stage.go:65-88`; `store_pending.go:79-117` | Export selects current graph and task ownership, not every historically readable edge. |

There is already useful unification: `vm/value_roots.go:14-17,113-168` gives
consumers one inventory of typed VM values, with root kinds; persistent current
versus retained traversal shares walkers in `store_reachability.go`. The next
useful abstraction must delete something beyond renaming these existing owners.

### Disproof attempt 1: let Go reachability decide everything

Real producer: assigning a local releases its previous value through
`vm/vm.go:558-562`. `collectDirectWaifsForGC`, `vm/waif_gc.go:19-36`, appends the
WAIF Value itself to PendingWaifs. The VM enumerates it as pending, not live:

    visit(value, valueRootPendingFinalization)  // value_roots.go:132
    if kind == valueRootLive { ... }           // waif_gc.go:119

At `engine/waif_lifecycle.go:309-321`, the sweep calls :recycle only when its
separately built live set lacks the queued value. At :485-500 the callback receives
that very WAIF as ThisValue and local `this`. Thus the queued allocation must be
strongly retained so its callback can run. Treating all strong references as MOO
roots makes the pending queue keep its own candidates live forever; weakening the
queue lets Go reclaim data the callback needs. This rejects the proposed collapse,
not current behavior. Go cleanup still usefully removes weak registry bookkeeping.

The analogous anonymous path is also not Go-object ownership. `types/value.go:90`
constructs TYPE_ANON with only a numeric ID. Keeping that Value does not keep a
particular *Object alive through a Go pointer. `RecycleOrphanAnonymousBatch`,
`vm/anonymous_gc.go:422-465`, explicitly resolves ID roots, freezes candidates,
and invokes recycle under the request context. A shared host-pointer rule would
require changing the Value representation and all identity/serialization users.

### Disproof attempt 2: make all retained edges semantic and export roots

The current retained graph is purposeful. `types/waif_image.go:115-134` visits
all still-readable images. `store_txn_waif_test.go:218-234` holds a snapshot that
has never read a WAIF, changes parent.child to an integer, and requires the old
child to remain a semantic root until release. The production producer is normal
WAIF assignment (`vm/op_property.go:359-403`) on a persistent WAIF value; this
test supplies a controlled instance, not a new lifecycle rule.

But `store_txn_waif_test.go:262-280` requires an old parent->child edge NOT to
remove child from checkpoint pending finalizations. Production
`store_pending.go:85-99` explicitly walks current edges for ownership, and
`store_snapshot.go:308-331` freezes current WAIF properties. Snapshot retirement
and export ownership therefore need different graph projections even if both
operate on one immutable image representation. Counting all retained images as
one global root set changes one of these requirements.

Do not infer that current semantic GC automatically walks all old numbered
images. `store_reachability.go:369-406` seeds persistent WAIF roots from current
numbered slots, then expands retained WAIF images. The source does not justify
silently broadening that policy to every edge in every old numbered root. Whether
such a change is desirable is a semantic decision, not a proven simplification.

### Disproof attempt 3: put every identity's image in one strong root map

Actual creation has no object-property prerequisite:
`builtins/objects_misc.go:128-130` constructs NewWaif, registers it weakly, and
returns it to the caller. It can live only in VM locals or task data. The loader
also constructs detached values and registers them (`reader_value.go:205-220`,
`reader.go:109-113`). First transaction access binds the physical payload to one
domain and installs its detached baseline at timestamp zero
(`types/waif_image.go:28-45`); older snapshots can first read that baseline after
another task changes the value. A missing root entry is therefore not uniformly
an invalid identity.

The strong-map candidate would retain every image's property values even after
its owner has no live handle. Keeping only a weak handle alongside the strong
image does not fix that. The loader is a real producer of graphs with back
references: it places a WAIF in savedWaifs BEFORE recursively reading properties
(`reader_value.go:205-237`). A root-held image that reaches its owner handle can
keep that handle reachable through the map itself. This is a representation
counterexample, not a claim of a new bug in the existing weak registry.

Go's [GC guide](https://go.dev/doc/gc-guide#common-weak-pointer-issues) explicitly
states that weak map keys do not weaken map values; a value-to-key path still
keeps the key reachable. Racket's
[ephemerons](https://docs.racket-lang.org/reference/ephemerons.html) provide the
conditional reachability that ordinary weak maps lack. Their graph rule is useful
prior art, but it does not appear in Go's weak.Pointer contract. An equivalent
Barn-level traced heap would need a complete external-root inventory and a
retirement protocol; that is a new memory-management subsystem, not a small root
map. It also would not by itself decide MOO :recycle eligibility.

Putting WAIF images behind their existing handles avoids this global rooting
problem. But then a captured numbered root does not determine a WAIF's image:
the handle needs timestamped state or another per-snapshot binding. That is the
separate lifetime mechanism the proposed single root was supposed to remove.

### Disproof attempt 4: one identity means one payload in every context

Checkpoint capture supplies a direct counterexample. At
`store_snapshot.go:312-331`, NewWaifWithIdentity builds a detached payload with
the same identity as the live WAIF, and memoization makes exported aliases share
that frozen payload. `store_txn_waif_test.go:190-215` requires identity equality,
frozen n=1 despite a later live n=2, shared export aliases, and a detached export
write that does not change live n=2. A process-global identity-to-image authority
would join things the current API keeps separate. A unified resolver must be
scoped to a store/view/export domain and preserve physical detached construction.

Anonymous identity has a different representation again: `Renumber` changes
numbered directory placement/owners and updates anonymous structural references
(`store_lifecycle.go:305-511`), while checkpoint anonymous IDs are remapped
(`store_snapshot.go:177-268`). Those key spaces can share storage algorithms;
they cannot share an unconditional identity-rekey operation.

### A visibility difference that a uniform model must not hide

The anonymous branch in `store_txn_snapshot.go:140-142` clones only a current
anonymous image whose version is <= readTS; otherwise it has no anonymous history
fallback. `anon_concurrent_race_test.go:35-41,85-90` explicitly accepts stale-read
E_PROPNF/E_INVIND. Its producer uses DirectTxn.CreateObject(anonymous=true), then
concurrent normal property writes. Full immutable anonymous snapshots would
remove that limitation, changing observable behavior rather than merely moving
ownership. This study does not judge its MOO conformance or propose preserving a
defect: no Toast oracle run was performed. It means exact current-behavior parity
cannot be claimed for a uniform historical lookup. Retaining the current rule as
a typed policy is possible, but preserves a distinction in the model.

### Smallest coherent shared model, with explicit ownership/release rules

The strongest compatible proposal is a **common discipline with typed owners**,
not one universal strong root. It can be expressed without new production code:

1. One store domain owns publication order and validates resolved observations.
   Numbered/anonymous identities belong to that store; a detached WAIF payload
   binds to a domain once. Checkpoint copies remain detached in their export view.
2. An image is immutable once readers can reach it. A transaction exclusively
   owns its staged state/private views until successful publication or discard.
   Identity handles do not grant mutation of published images. Keep coarse
   exclusion and root-capture RLock; do not require lock-free capture.
3. A read view owns permission to resolve its required versions. Release ends
   that permission once, allowing pruning/dropping roots. Failure does not release
   the view while error handling can still inspect it: `store_txn.go:294-303`
   preserves physical maps after terminal failure; runtime release occurs at
   actual attempt/slice boundaries (`engine/task_runtime.go:191-198,227-231`).
4. Retry owns execution values separately from a store view. `vm/retry.go:20-53`
   detaches Context from its saved VM and retains execution roots; the runtime
   releases the failed StoreTxn before restoring and opening a new one. Keeping
   the old world root as the retry's read authority would retry against old state.
5. Renewal first establishes the replacement view and validates carried
   observations; only successful handoff releases the old view. Hot renewal
   copies WAIF base/staged state and releases only after success
   (`store_hot_read.go:242-297`). Boundary renewal also carries rebased WAIF
   dependencies (`store_txn_commit.go:173-176`). A common Release function cannot
   infer that values, observations, and execution roots all die together.
6. Semantic roots have kinds: live execution/persistent values protect eligibility;
   pending-finalization values retain callback data without self-protection;
   retained image edges protect still-readable descendants according to the
   existing policy. Export selects its own current graph. Keep the existing
   inventory and context-sensitive consumers; do not create parallel inventories.
7. Weak class discovery does not own WAIF lifetime. Class rename/delete resolves
   live weak entries at commit, including late arrivals, and publishes those
   changes with class definitions. A resolver must consult the candidate graph
   during that phase (`store_txn_waif.go:203-257`); tx.waifs alone is not the domain.
8. Quiescent runtime capture remains authority for mutable external values.
   `engine/runtime.go:653-676` refuses capture while a physical execution lease is
   active and includes admission roots. `engine/admission.go:89-108` owns their
   explicit release. Snapshot roots cannot replace these execution rules.

These rules are coherent with the current owners. They organize reasoning, but
they are not a new API proposal or a demonstrated deletion. The most tempting
large deletion—replacing semantic root capture with Go GC—fails rule 6.

### Alternative challenged: one versioned entity handle instead of one world root

Numbered directory slots and WAIF payloads could both contain a typed immutable
image plus history under the same store epoch; use the existing reader registry
to select/prune both. This avoids copying world-directory paths. Ordinary Value
references keep WAIF handles alive, while the numbered directory owns numbered
handles and anonymous identity lookup owns anonymous ones. Keep weak class and
prune indexes for WAIFs; preserve their cleanup behavior and export copies.

This is the smallest plausible *representation* unification found. It would move
numbered history from a store map to per-entity owners, or factor common history
algorithms without moving it. It still needs ordered multi-image publication,
reader registration, a pruning work index, fine selectors, semantic walkers,
WAIF domain binding, and special anonymous visibility. Different policies remain:
numbered history is reader-aware and diagnostic-sensitive; WAIF history's retained
edges affect semantic roots and graph-epoch invalidation. One generic history
container with these callbacks is not obviously less code than the current two.

Primary prior art supports this narrower distinction. Clojure
[Ref.java](https://github.com/clojure/clojure/blob/master/src/jvm/clojure/lang/Ref.java)
stores identity, a TVal history and transactional dereference (:44-112), while its
[transaction contract](https://clojure.org/reference/refs) distinguishes commute,
ensure and external effects. That demonstrates a shared versioned-cell model,
not elimination of history, serializable read rules, or language finalization.
Barn's positional edits and irreversible effect boundary cannot be replaced by
assuming every operation is commutative or freely retryable.

Racket's [will executors](https://docs.racket-lang.org/reference/willexecutor.html)
also separate becoming eligible, retaining the value for its callback, and
executing that callback. Go [AddCleanup](https://pkg.go.dev/runtime#AddCleanup)
cannot substitute for Barn's queue: its argument must not retain the monitored
object, execution timing is not the runtime's quiescent sweep, and it does not
carry Barn's MOO programmer/task context. These are exact limits of the prior-art
match, not reasons to import or recreate either runtime.

### Deletion/replacement ledger after testing the hypothesis

| Candidate | Actual deletion opportunity | Necessary replacement or retained mechanisms |
|---|---|---|
| Whole-world strong root | Numbered history, WAIF image chains and reader floor could disappear as ordinary lookup machinery | Conditional WAIF image ownership/retirement; labeled semantic roots; detached fallback; export-domain copies; late class propagation; diagnostic journal; anonymous policy |
| Numbered-only roots | Ordinary numbered history selection and slot publication | Persistent directory update/capture; WAIF floor/history; semantic GC and export barriers; diagnostic history |
| Versioned handles/common history helper | Duplicated selection/pruning code and possibly numbered history-map ownership | Reader registry, weak work indexes, ordering, differing pruning/edge policies, all domain/lifecycle/export rules |
| Existing typed owners with one operation implementation | No speculative mechanism deletion | Already supplies shared publication lock, epoch, validation path and VM root inventory; object-owned pending changes can simplify a different axis |

No new numeric net estimate is justified by this extension. The earlier ranges
are not reinforced by the mere existence of more obligations. Root migration
must count conditional ownership and graph handling on the addition side; handle
unification must count adapters/policy branches, not advertise the history map's
old location as deleted complexity. Neither removes the semantic algorithms.

Memory/CPU tradeoffs remain empirical: roots trade per-object history scans and
registry work for persistent path copies and retained root nodes; versioned handles
avoid directory path copying but retain per-identity version selection and prune
scheduling. Strong all-identity maps risk extra retained graphs; exact conditional
root ownership would introduce tracing. Reusing today's quiescent root capture
avoids a second inventory but does not make graph traversal free. No measured
lookup, allocation, GC pause, finalizer-delay or workload-throughput result exists.

### What is settled, and the bounded next step

Source settles that the five relations differ, that existing producers need each,
and that simple strong-root/weak-key/host-GC collapses fail the stated contracts.
It does NOT settle that no future representation can be smaller, that current
anonymous visibility is desirable, or that the root directory would be slower.
Those are respectively open design, semantic and empirical questions.

There is no justified full-world implementation plan from this investigation.
The finished bounded action is to present these counterexamples and the three
actual choices: keep typed owners and simplify changes; measure the separately
specified numbered-root slice; or explicitly choose broader lifetime/visibility
semantics before designing a whole-world replacement. The already-written
numbered-root experiment plan remains sufficient for that narrower choice. Do not
build a generic entity framework merely to discover whether its policy callbacks
reproduce the old complexity.

**Direct verdict:** a shared immutable-image/publication contract is useful and
largely present. Collapsing snapshot retention, MOO finalization and checkpoint
ownership into one liveness rule is disproved by the pending-WAIF and historical-edge
producers above. One implementation owning multiple typed policies is possible;
this research has not demonstrated that it removes enough machinery to shrink the
whole store. That is a negative result for the proposed collapse, not a proof
against every common representation or a claim that the store cannot be beautified.

## Constructive follow-up: typed references, 2026-10-26

**Answer: yes, a pending recycle reference fits a coherent common graph.** The
previous negative result excludes ordinary unqualified reachability, not this
construction. Pending can retain the callback value without counting as a live
root for eligibility. Different traversals of the same graph need not imply
different ownership models. The construction below actually derives the queries;
it does not claim that naming them alike deletes their implementation.

Read against implementation `b91e36f5`; note HEAD advanced during this reading
from `f18758b9` to `53578fda`. No implementation or experiment was performed.
This section supersedes the preceding recommendation to stop at counterexamples:
there is a concrete algebra worth using to simplify existing traversal code.

### Smallest graph that covers the observed cases

Use a **virtual graph**, not a second allocated heap or registry. Its nodes are:

* A value occurrence: scalar, list/map element, or reference to an entity.
* An entity identity, qualified by its owning domain where applicable.
* An image/payload occurrence: a numbered-object image, anonymous object payload,
  WAIF image, constructor payload, or detached checkpoint payload.
* A view/capture: current store, transaction, retained-image collection, export,
  or a frozen set of already-computed identities.

Identity is the comparison key; image is the thing traversed. They cannot be the
same node: snapshot rewriting creates `NewWaifWithIdentity` and a fresh payload
(`store_snapshot.go:308-331`). A detached export and a live WAIF can therefore
compare equal while exposing different edges. No new globally allocated identity
node is required: existing keys and values implement these conceptual nodes.

Edges have these roles, all already represented by existing fields:

| Edge | Existing representation | Meaning |
|---|---|---|
| live | VM values/context/task roots, current store properties | Seed semantic queries |
| pending | pending finalization queues plus callback context | Retain candidate; do not seed semantic queries |
| contains | list element or map key/value | Propagate the query unchanged |
| resolves | entity to image selected by a view | Choose payload before following properties |
| property | image to property value | Propagate allowed query through selected payload |
| held image | history or transaction retaining a payload | Keep readable bytes; not automatically a semantic seed |
| weak discovery | class WAIF registry, history bookkeeping | Enumerate if still present; never confer ownership |
| frozen member | captured OwnRefs or batch eligibility set | Emit identity; never re-expand its current properties |

Physical retention follows actual Go strong references, including pending and
held-image edges. A virtual traversal need not duplicate Go's collector. Semantic
eligibility follows a selected subgraph. Snapshot visibility is image resolution;
checkpoint selection is another selected subgraph followed by a transformation.
These are compact operations over one representation, rather than one universal
boolean called alive. The existing owner remains responsible for adding/removing
its edges; no new retain/release call on every property assignment is proposed.

### Closure algebra and the finite policy table

Write `C(q, R)` for the least fixed point obtained from roots R under query q.
The query is `(projection, image selector, capture)`. Projection is one of direct
finalizable roots, WAIF closure, or anonymous closure. Image selector is current,
retained, transaction-visible, or detached export. Capture identifies the view or
sampling event; timestamps are parameters, not another finite color per timestamp.

| Encounter | Direct roots | WAIF closure | Anonymous closure |
|---|---|---|---|
| list/map | recurse in existing order | recurse | recurse |
| WAIF | emit identity/value, stop | emit, then selected properties | selected properties, do not emit WAIF |
| anonymous reference | emit ID, stop | stop | emit ID; store expansion may resolve its object |
| numbered reference | stop | stop | stop |
| already-expanded node | stop | stop | stop |
| frozen member | emit if requested species matches, stop | same | same |

The anonymous projection has two existing stages: collecting IDs from values,
then resolving those IDs and feeding object properties back through the collector.
Keep that boundary: it encodes validity checks, transaction resolution and locking.
`store_reachability.go:169-176` reads `obj := tx.object(id)` and then calls
`collectAnonymousObjectRefsVisited(..., true)` on properties. Its current-store
counterpart at 193-201 uses `s.lookupAnonymousLocked(id)` and the selected retained
flag. A common value traversal need not acquire store locks or import store types.

Important disproof attempt: following **every** entity property edge produces a
different graph from today's implementation. The WAIF walker has WAIF/list/map
cases only (`vm/waif_gc.go:52-89`); anonymous expansion collects anonymous IDs
through WAIF properties (`store_reachability.go:12-45,184-205`). A WAIF query does
not automatically dereference an anonymous ID. The finite projection table
preserves this asymmetry. Source establishes existing behavior, not that every
asymmetry is necessary MOO semantics; changing it needs a separate decision.

Within a fixed query/view, existing identity sets terminate self/cycles. If walks
are combined, the visited key must include query and payload/view: a result bit
for one mode must not suppress expansion newly required by another. A worklist
could propagate only newly added mode bits. **Do not implement that machinery
merely because the algebra permits it.** Separate walks with simple existing sets
are the smaller implementation until a real shared-walk benefit is measured.

Root admission is also a finite table: live roots seed semantic queries; pending
roots seed direct-candidate and export-pending queries; retained snapshot handles
seed image retention/visibility; weak registry entries seed none. `valueRootKind`
already distinguishes live from pending (`vm/value_roots.go:8-9,132-135`), and
`CollectWaifsFromVMInto` admits only live (`vm/waif_gc.go:119-124`). These are the
seed of a shared engine, not evidence that a new root registry is needed.

### Derivation 1: dropped WAIF, pending reference, callback, escape

Producer: VM local release queues finalizable values (`vm/vm.go:558` and
`vm/waif_gc.go:19-46`). Queue entries carry the actual WAIF value, so pending
already supplies the strong reference Q suggests. Deferral attaches context and
task (`engine/waif_lifecycle.go:96-98`: `Waif: waif, Ctx: ctx, Task: owner`).

1. Removing a local removes one live root; adding pending adds a retaining edge.
2. Pending alone seeds no semantic closure. It does **not** make the target's
   properties semantically live either. Otherwise a pending self-edge or cycle
   could make its own callback ineligible. It still physically retains whatever
   payload its actual strong Go references retain.
3. At deferral, OwnRefs is already a closure from the still-live VM, captured by
   `vm.CollectWaifsFromVM(ownVM, &ownRefs)` (`engine/waif_lifecycle.go:69-72`).
   Model these as frozen member edges, not roots to re-traverse later.
4. At sweep, eligibility is `pending identities minus L`, where L is current
   persistent closure union captured sibling identities union every OwnRefs set.
   Source computes `live := s.liveWaifs(roots)` once, before the callback loop
   (`engine/waif_lifecycle.go:307-316`). Preserve candidate order and context.
5. Entering the callback adds ordinary live references:
   `recycleCtx.ThisValue = waif` (:485) and local `this` (:498). Its properties
   now participate when a live-root query is actually performed. This does not
   retroactively change the already-captured batch L.
6. Leaving execution releases those execution references according to existing
   VM/context lifecycle. If the callback stores the value in a persistent property,
   that write creates a live edge for subsequent captures; no special resurrection
   bit is required. This is a permitted ordinary VM write path, not an assertion
   that a particular running database contains a resurrection verb.

Strongest temporal trap: recomputing L after each callback, or re-expanding frozen
OwnRefs through today's properties, changes the algorithm. A callback can execute
ordinary writes; the code does not re-evaluate later entries against them. The
construction preserves the frozen decision, including this limitation. It makes
no claim that current ordering is ideal or required by Toast.

### Derivation 2: hidden old edge, checkpoint, same-identity detached value

Producer: `store_txn_waif_test.go:218-235` describes parent WAIF's old image holding
child after the current edge is removed; `:262` covers the pending export case.
Implementation: `types/waif_image.go:115-134` visits retained image properties;
`store_pending.go:90-99` passes `false` for current-only task export traversal.

Retained-mode closure from live parent reaches child through the old image. A
current export query does not. Therefore child can remain protected for existing
runtime readers and still require its own pending checkpoint entry. The same
physical graph supports both by changing the selector, without contradictory
ownership. The snapshot rewriter memoizes by identity within its export and
creates detached payloads (`store_snapshot.go:308-331`); aliases inside that export
share the frozen payload. Never merge that memo with a live-world traversal memo.

When the reader releases, existing WAIF history pruning removes its held-image
edge and graph-epoch invalidation makes the cached closure stale. The algebra
does not remove pruning by itself. A persistent root design could supply held
images differently, but it still must expose the same retained-edge projection
if behavior is to stay unchanged. Nor should every old numbered-object history
become a new semantic root: current persistent scanning starts at live directory
slots (`store_reachability.go:382-394`), not all historical numbered images.

### Derivation 3: anonymous pending batch and callback-created objects

Producer: `RecycleOrphanAnonymousBatch` (`vm/anonymous_gc.go:423-465`). It unions
captured OwnRefs and sibling IDs, expands them with current-store resolution,
then freezes `AnonymousRecycleCandidates(reachable, minFloor)` at :460. The
source comment explicitly identifies callbacks creating/persisting new anonymous
objects (:455-459). Request routing preserves floors/context and deduplicates
IDs (:472-491). This is a real producer, not a synthetic lifetime case.

In the graph, request edges carry floor/context; candidate selection is a frozen
projection of identities minus closure at this phase. Callback-created entities
are outside that frozen domain. Entering a recycle continuation adds a live
reference to its request object (`vm/value_roots.go:52`); callback exit/removal
does not require retroactive traversal of the prior capture. Anonymous IDs remain
store-resolved keys. Export's ID mapping remains a transformation, not runtime
renumbering of graph identity. These rules derive the current sequence without
requiring anonymous objects to acquire WAIF's domain/history semantics.

### Other attempts to break the construction

* Task-only WAIFs: `builtins/objects_misc.go:101-130` produces one and registers it
  weakly before returning it. Live task roots supply ownership; registry supplies
  class discovery only. Detached first access can still bind domain with baseline
  image zero (`types/waif_image.go:28-45`); no current-world root is prerequisite.
* Rename/delete: weak enumeration and commit-time `moveUnstagedWaifValuesLocked`
  (`store_txn_waif.go:203-257`) remain an operation over discoverable identities,
  including late live values. Reachability does not derive property-name moves.
* Retry/error/renewal: private transaction and retry-checkpoint roots remain view
  edges until their current owners release them. Root typing is compatible with
  `markTerminal` preserving readable error state; it does not authorize early
  release or change image resolution after renewal.
* Checkpoint task ownership is set subtraction under current selectors:
  `store_pending.go:103-111` excludes task-owned pending entries. Canonical pending
  roots additionally require ordering/coverage reduction, not only closure:
  `vm/anonymous_gc.go:195-215,237-251` sorts closures and accumulates coverage.
  A graph library cannot delete that policy by relabeling it a traversal.

### Prior art supports the positive construction

[Racket wills](https://docs.racket-lang.org/reference/willexecutor.html) explicitly
combine retained registrants, eligibility that excludes registration, and explicit
executor scheduling. This supports Q's pending-reference idea. It does not prove
Barn's callbacks or timing equivalent. [Racket ephemerons](https://docs.racket-lang.org/reference/ephemerons.html)
likewise show conditional tracing rules handling cycles without treating every
retaining path as eligibility. Barn need not adopt ephemerons: Go strong references
already retain pending values; Barn already computes semantic closure explicitly.
Both sources are official documentation, read as HTML, not extracted papers.

### Deletion potential and finished bounded next step

The positive algebra is broader than a helper: root admission, image selection,
projection, fixed-point traversal, captured-set union, candidate subtraction and
export transformation now describe every investigated sequence. Its narrowest
useful implementation would consolidate structural traversal while preserving
the existing root owners and timing. A full explicit graph heap would instead
add mutation barriers/edge bookkeeping and duplicate Go retention without a
source-demonstrated need.

Unmeasured planning ledger, excluding tests/docs and without claiming savings:

| Scope | Gross removal candidate | Replacement | Likely net |
|---|---|---|---|
| Identical WAIF closure walkers | two ~35-line bodies | one ~35-line body plus call-site changes | tens of lines deleted |
| Value recursion for direct/anonymous/WAIF projections | roughly 100–180 additional lines across VM/store | roughly 70–150 lines of typed traversal and adapters | could save tens; could break even |
| Root capture, pending contexts, frozen sets, canonicalization | no demonstrated deletion | keep current owners | zero justified savings |
| Histories, validation, publication, rename propagation, serialization | no deletion follows from this algebra | keep existing implementation | zero justified savings |

Finish the bounded design at that boundary: retain `valueRootKind`; centralize
container/WAIF traversal in `types` with explicit direct/current/retained choices
and typed result collection; leave store anonymous resolution and VM root
enumeration at their existing owners. Avoid user-supplied policy callbacks and a
general scheduler framework. Preserve first-seen ordering, current cycle keys,
fast finalizable filters, caching, and query boundaries. If adapters erase the
deletion or broaden semantics, keep only the exact duplicate consolidation.

After Q chooses implementation, existing focused lifecycle/history/checkpoint
tests and ordinary PR workflows are the correctness experiment; the documented
Elle workflow applies if isolation/publication is touched. Existing workload
profiles/benchmarks would assess traversal allocation and latency, not justify a
fresh parallel benchmark harness. Unknown: net code after adapters, callback
delay/allocation effects, cache behavior, and whether combining any walks pays.
No tests or measurements were run for this research.

**Verdict:** Q's typed-reference hypothesis works as a compact model. Pending
references, snapshot-held images and callback execution fit it without changing
their meaning. It justifies bounded representation/traversal simplification;
it does not yet justify replacing store ownership or promise a large size cut.
The strongest remaining obstacle is temporal and operational bookkeeping, not
an algebraic contradiction: captured closures, batch order/context, publication,
identity binding and export rewriting still do actual work after graph queries
are unified. Whether more of that work can share implementation remains empirical.

## Smaller construction: closure, sets, ordered effects (2026-10-26)

**Yes: the model itself can shrink substantially.** The preceding eight edge
roles and query-state tuple are an explanatory expansion, not necessary runtime
machinery. Replace them with ordinary reachability over resolved values, ordinary
sets, and the existing caller's execution order. This section is the preferred
model; the preceding lifecycle evidence still applies. Source HEAD was
`6efa7da2516525c4fb606f22d4424fa0f606c6d3`, implementation remains `b91e36f5`.

### What remains after removing unnecessary dimensions

Let `C(R,E)` return the closure of roots R through adjacency E, retaining the
existing traversal's first-seen order. Mathematically membership is the usual
least fixed point; operationally use existing depth-first encounter order rather
than introducing breadth-first order. Filter that result to WAIF values or
anonymous IDs as the consumer needs. The set supports membership and an ordered
result; `types/WaifSet` already supplies exactly that pair (`waifset.go:11-18`).

There are only three ingredients:

1. **Roots and candidate collections.** The caller supplies the values relevant
   now. A pending collection P is retained normally by Go; it is simply absent
   from roots R used to decide semantic liveness. No special pending edge exists.
2. **Resolved adjacency.** Containers yield their children. A WAIF yields no
   properties, current properties, or retained properties, as selected by this
   caller. The choice is an input to traversal, not a color propagated through
   every edge. Object resolution stays with the store and its existing locks.
3. **Set operations followed by ordered effects.** Union captured results,
   subtract live identities from candidates, reduce covering roots where needed,
   then execute callbacks or serialize in the existing order.

Capture is when the caller computes a result. A frozen set is just a result kept
in a local or pending record. View is the input that determines adjacency. Neither
needs its own graph node, epoch field, graph edge kind, or traversal bitset.
Physical retention stays with Go and existing containers. Image publication,
domain binding and history retention remain their existing owners' operations;
the closure implementation neither duplicates nor replaces them.

The necessary boundary is **which adjacency is supplied**, not a projection-mode
matrix. For the value graph, anonymous and numbered references are terminals;
WAIF properties can be expanded. WAIF results are one filter of that closure;
anonymous IDs are another. To expand anonymous objects, the store forms an
induced adjacency: an anonymous object's neighbors are anonymous IDs found by
the value traversal of its resolved properties. Apply ordinary C again to that
graph. This is precisely the two-stage shape at
`store_reachability.go:151-181,184-205`. It does not accidentally let WAIF queries
follow anonymous-object pointers, and it does not require a generic entity API.

Keep each walk within one existing domain/view. Existing WAIF identity and
anonymous-ID visited sets then suffice; no `(mode,view,identity)` key is needed.
Detached export is its own traversal/memo, never mixed with live payloads sharing
the identity (`store_snapshot.go:308-331`). This removes the multi-mode visited
state contemplated above. It preserves current identity deduplication rather than
inventing a new physical-pointer equality rule. A combined walk across views
would need more state; there is no demonstrated reason to combine them.

### The same three sequences with this smaller algebra

**Dropped WAIF.** Local release places w in P. Go retains w through P; the live
root enumeration excludes P (`vm/value_roots.go:132-135`,
`vm/waif_gc.go:119-124`). Deferral records `O = WAIF(C(Rlive,Eretained))` while
the VM still exists (`engine/waif_lifecycle.go:69-72`). Sweep computes L from
current persistent closure plus already-computed sibling/O sets (:307-311).
The ordered candidate loop skips members of L and invokes the others (:312-316).
Entering the callback puts w in normal context/local storage (:485,498); leaving
releases those execution references. A persistent escape is an ordinary new live
root for a subsequent computation. The local L is never recomputed during this
loop. Nothing changes an edge color on callback entry or exit.

**Old WAIF edge and checkpoint.** Runtime semantic traversal uses Eretained, so
the old parent-to-child edge participates while the image exists
(`types/waif_image.go:115-134`). Checkpoint traversal supplies Ecurrent, so that
edge is absent. Task-owned checkpoint values are computed with current adjacency
and subtracted from pending candidates (`store_pending.go:90-111`). Export then
rewrites selected values with its private identity memo. Reader release changes
the retained adjacency through existing pruning, not through a graph-layer
release protocol. The historical-child source cases cited above distinguish the
two computations; no contradictory liveness flag is stored on the child.

**Anonymous batch.** Compute anonymous reachability using the store's induced
adjacency, union the captured references, and freeze the eligible ordered IDs
before any effects (`vm/anonymous_gc.go:438-460`). Execute that list with request
floors/context and existing deduplication (:472-491). The source explicitly says
a callback may create and persist a new object (:455-459); it cannot enter an
already-created list. A callback's live continuation root is ordinary storage
(`vm/value_roots.go:52`). Again, time belongs to caller sequencing, not an extra
coordinate on every reference.

### Can stored derived state disappear?

Some results are mathematically derivable but no longer derivable from the inputs
available **at the later phase**. This is the important limit on deletion.

| State | Can it be derived later without changing behavior? | Source reason |
|---|---|---|
| Pending WAIF OwnRefs | No from the later VM/current graph | Owning VM is released; closure captured at defer time, `engine/waif_lifecycle.go:43-47,69-72` |
| Pending DirectRoots | Not from OwnRefs alone | Includes pending roots and only direct WAIFs; OwnRefs excludes pending and includes transitive descendants, `vm/anonymous_gc.go:71-103,133-157` |
| WAIF batch L | Could recompute only before effects with unchanged inputs | Loop intentionally uses one computed set, `engine/waif_lifecycle.go:311-316` |
| Anonymous frozen candidates | Not from post-callback store | Callback can create new objects, `vm/anonymous_gc.go:455-460` |
| Current/retained persistent WAIF caches | Yes by recomputation at query time | Cache returns recomputable top/closure, `store_reachability.go:368-406`; CPU/scan cost changes |
| Canonicalization candidate closures | Yes only while preserving the sampled adjacency | Used for sorting and coverage; rewalking later may encounter changed payloads and costs another traversal, `vm/anonymous_gc.go:188-215,228-251` |

DirectRoots cannot replace OwnRefs by being expanded later: it admits a different
root population and later retained adjacency may differ. OwnRefs cannot replace
DirectRoots: it has forgotten whether a descendant was directly owned, which
matters to canonical pending roots. The existing implementation already delays
expensive canonicalization until shutdown while capturing only its necessary
direct inputs (`engine/waif_lifecycle.go:73-77`). This is an existing successful
elimination of eagerly derived state, not a new opportunity to count again.

Cache deletion is the honest larger candidate, but not a free simplification.
The top-level list and expanded closure have different invalidation inputs:
`persistentWaifRootsEntry` stores `epoch, top, graphEpoch, closure`
(`store_reachability.go:299-304`). On a graph-only change, top is reused (:378-394).
Removing both means scanning current object properties on subsequent queries;
removing just top loses that reuse. Source comments report the motivating scan
cost (:350-357), but are not a fresh benchmark. No evidence here supports paying
that cost to save cache code while preserving Q's performance goal. Conversely,
the algebra does not prove the cache must stay forever: matched workload evidence
could justify removal. Do not replace it with a new cache under another name.

Pending entries repeat OwnRefs/DirectRoots headers for each WAIF
(`engine/waif_lifecycle.go:91-98`; `finalization/coordinator.go:20-24`), but share
the same slice/map storage. Grouping by producer would save repeated metadata,
not repeated closure payloads, and adds group iteration/shutdown-position handling.
It is not a demonstrated code deletion or a reason for a new batch abstraction.

### Two potential shared operations, with ownership and adapters counted

The first is ordinary value closure. Put shared value recursion in `types`,
which already owns WAIF payload access and ordered identity sets. Replace bodies
of `vm.collectWaifsInto` and `store.collectWaifsInto`, then evaluate
`vm.collectAnonymousRefsForGCVisited` and
`store.collectAnonymousObjectRefsVisited` against the same traversal. Direct
collection covers `vm.collectDirectFinalizationRoots` and
`store.collectTopLevelWaifsInto`; retain their output choices at callers.
Store anonymous resolution/locking, VM root enumeration, phase scheduling and
checkpoint rewriting stay where they are. Adapters for output filtering and
WAIF property choice count as replacement code, not free glue. Preserve existing
`MayHoldFinalizable` fast paths rather than visiting unrelated values gratuitously.

The second operation is ordered cover reduction. Given candidates with their
closures and a preexisting covered set: order by descending closure size and the
existing tie-break; keep an uncovered candidate; union its closure. Both
`CanonicalizePendingFinalizationValues` and `canonicalWaifRoots` implement this
(`vm/anonymous_gc.go:195-215,237-251`). Anonymous ties use numeric ID; WAIF ties
use input order. The resulting algorithm is common; the set representations and
closure producers differ. A shared reducer is worthwhile only if its typed-set
adapters and ordering inputs are smaller than these short loops. Do not force
both onto boxed entity keys or allocate a second representation just to share it.
`AppendPendingFinalizations` is incremental and order-sensitive
(`store_pending.go:25-73,138-161`); it is not automatically the same batch reducer.

Unmeasured revised ledger: value recursion offers roughly 135–215 lines of gross
duplicate bodies, replaced by perhaps 75–145 lines including adapters, yielding
anything from little change to roughly a hundred lines removed. Shared cover
reduction targets approximately 40 lines of ordering/selection loops, but about
20–40 lines of common implementation plus 10–25 adapter lines may consume all
the saving. These are planning ranges, not measured diffs. Cache removal has no
accepted performance case; retained captures have no behavior-preserving deletion
case. No owner registry, root/view object, reference color, generic graph heap or
mode propagation machinery belongs in this bounded change.

**Revised verdict:** the small algebra is `closure + set operations + ordered
effects`. Pending is a candidate list kept out of live roots. This is simpler
than the previous typed-edge proposal and explains the same source behavior.
It makes a compact traversal refactor plausible and identifies a second possible
shared reducer. It has not exposed a safe large deletion of stored state: most
captures preserve information across time, while caches trade code for real
recomputation. Finish at this bounded design unless measurements establish a
cache-removal opportunity. No code, tests, benchmarks or publication were done.
