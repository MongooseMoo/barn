# Object-owned pending changes: research, not implementation

## Goal

Investigate candidate A (one authoritative object-owned write representation)
and candidate D (one change description driving private and published images)
from `store-ownership-exploration.md`. Q requested Astra research before choosing.
The smallest proposed production change is to reorganize the existing eleven
write collections around their object, delete commit-time regrouping, and keep
the existing operations, dependency sets, private images and publication order.
Estimated production surface: 500–900 changed lines; estimated net reduction:
roughly 40–150 lines, unmeasured. No production implementation is authorized.

## State

Initial source read in this investigation was the working tree at:

```text
git rev-parse HEAD
206482cd2ee5a3715037893151333a5d6fb6fa23
```

Parent identifies cae320f3 as the implementation base and 206482cd as its
note-only successor. This report does not independently establish latest remote
master. Parent's living exploration note was read in full. This agent edits only
this report; no code, tests, benchmark, CI, server or oracle run was performed.
Existing tests below were read as obligations, not reported as passing.

### 2026-10-26 base refresh: b91e36f5

Parent fetched and rebased the note-only work onto b91e36f5 during research. I
read `git diff cae320f3 b91e36f5 -- db/store` and the corresponding
`vm/op_property.go` diff. Unless explicitly labeled b91e36f5 here, source line
citations and estimates in the rest of this report refer to cae320f3, the
implementation base of the initial investigation, and are historical anchors.
No implementation, test or benchmark was run for this refresh.

The material migration addition is a WAIF consumer of object write records.
At b91e36f5, `store_txn_waif.go:203-254` reads the rename and definition-delete
collections to move values on WAIFs the transaction did not read. Raw source:

```go
for key, rename := range tx.propertyRenames {
    add(key.objID, rename.from, rename.to)
}
for key, name := range tx.propertyDefinitionDeletes {
    add(key.objID, name, "")
}
```

`store_cow.go:466` calls `tx.moveUnstagedWaifValuesLocked(ts)` after object image
publication and before staged WAIF publication; the clock still advances later.
The method walks live class descendants, excludes identities already present
in tx.waifs (:236), and publishes remaining applicable WAIF edits at the same
timestamp. The migration must read renames and definition deletes from the new
object records without clearing them early, retain this ordering and exclusion,
and preserve substitution semantics for swaps (`moveWaifProperties`, :146-158).

RenameProperty and DeleteDefinedProperty now call moveWaifValues while staging.
That method (:165-197) visits registered live WAIFs of descendant classes through
the weak registry and materializes their staged value maps. Thus the earlier
instruction to leave WAIFs alone means **retain their separate identity/image
representation**, not ignore these new object-write consumers. A now also
migrates store_txn_waif.go's two collection loops; it does not need a unified
object/WAIF write domain or a new operation log.

The real VM producer also changed: `vm/op_property.go:380` tries
ReplaceWaifProperty for a held value; when absent, :395 reads the class definition
before the containment-error return at :399. Preserve these observation and error
ordering paths; record regrouping does not authorize simplifying them away.
The new `waif_property_move_test.go` bodies cover swaps, deletion/redefinition,
a WAIF published after the staging walk, and a stale class-property writer.
Include those existing tests in A's relevant PR validation evidence.

The diff also adds a read-timestamp registry benchmark and corrects its padding
comment; it does not remove A's record-conversion opportunity. The 500–900 touched
and 40–150 net-line estimates below remain rough **cae320f3 planning estimates**,
not refreshed diff measurements. Add the WAIF consumer migration to the touched
surface; no additional net-deletion credit is claimed. The verdict remains a
bounded consolidation with unknown performance, orthogonal to immutable roots.

## Decision and why

**A is a plausible bounded cleanup, not a new storage algebra and not a tenfold
shrink.** It eliminates a conversion and repeated object filtering. It does not
eliminate the eleven different meanings of writes, or the distinction between
private state and commit state. A representation-only first slice preserves the
existing behavior; this is not proof that every current distinction is fundamental
or that no other encoding can represent it with fewer forms.
Its performance is unknown; nesting maps can trade
commit allocation savings for staging allocations and slower hot reads.

**Do not combine A with a universal change interpreter in the first slice.** D
has useful mathematics, but the existing normalized write records are not an
operation log. Running the whole accumulated record after every operation would
replay relationship deltas, delete verbs twice and insert definitions twice.
A complete incremental interpreter needs new semantics for normalization,
staging-only generations and publication stamping. The concrete duplication it
would remove has not justified that larger machinery.

A and immutable world roots are orthogonal: A organizes transaction intent;
roots organize committed images and snapshots. A root implementation would still
need a write intent representation to merge changes with the latest root. A can
be done without roots; roots do not require A. Choosing one first is a sequencing
decision, not a claim that they are mutually exclusive.

## What is actually represented

`store_txn.go:26-42,83-88` contains the eleven write collections. The following
inventory traces their producers and consumers; paths without prefixes here
are under `db/store/`.

| Collection | Producer / normalization | Consumers and retained meaning |
|---|---|---|
| scalarWrites | `store_txn_objects.go:103-155`: name/owner/flags setters retain field presence | `store_cow.go:211-222` overlays only set fields; live transfer :240-245 rewrites self-owner; forget :177; topology/hot eligibility differs from property values |
| relationshipWrites | `store_txn_relationships.go:75-159`; create/recycle in objects :202-214,252-270 | cow :223-242 applies location/time replacements and ordered contents/children deltas to current live image; topology detection, forget, preflight object footprint |
| propertyDefines | properties :449-482; value/info writes update staged definition :103-105,430-432; rename moves key :520-524; delete cancels :430-431 | property order reconstruction :57-92; preflight commit :388-395; cow :257-268; direct descendant propagation; read-mark suppression; creation/map eligibility |
| propertyDefinitionDeletes | inheritance :406-446 records original published name, undoing a pending rename | commit :396-402 checks definition existence; cow :243-253 deletes before rename/define; exclusive descendant removal |
| propertyRenames | properties :487-540 collapses name chain to published source and final destination | cow head via renamesOf; preflight :403-409; descendant walk; renamedAway permits reuse of vacated names; staged reads suppressed |
| propertyWrites | properties :98-127 tracks original slot for value cancellation; :434-438 metadata writes; inheritance :333-338,361-366,479 inherited slots | cow :276-302; hot restaging :283-292; map rebase :305-329; private replay/reseed; staged reads suppressed; keyed reads may still observe unchanged entries |
| propertyDeletes | properties :568-587; inheritance rename/remove :465-479,546-550,576-577 | cow :303-308 removes local slot and stamps shape; reseed filters by surviving ancestry; later writes cancel deletion |
| verbWrites | verbs :141-146 copies code keyed by stable runtime ID; deleting target removes its pending code :49 | preflight commit :380-384 checks ID; cow :309-323 resolves ID; live adoption/replay; read-mark suppression; dispatch invalidation |
| verbDeletes | verbs :35-53 appends current private-list index and advances private generation | validation :163-182 simulates shrinking lengths; cow :324-327 applies indexes in per-object sequence; flush refuses them; topology forces validating boundary |
| createdObjects | objects :168-214 allocates ID and saves pristine base, separate from mutable private object | validation skips live checks for newly created IDs; preflight checks ID occupancy first; cow :191-194 clones base and stamps; allocation/max-ID publication; map-key eligibility |
| recycleWrites | objects :240-282 stages tombstone and edits inverse edges | cow :328-337 tombstones last; publication appends recycled ID; flush keeps recycled cache identity; IsRecycled :291-296; topology/read validation retained |

WAIF writes are not a twelfth object collection: `tx.waifs` holds another identity
domain and publishes separately at cow :465-469. Leave it alone in A. Map-key
state likewise combines observations and intent provenance; it cannot be folded
into the final property value without losing information.

### Evidence that regrouping is real, and partly already paid for

At `store_cow.go:136-184`, writesByObject reconstructs each object from the eleven
collections. Representative raw lines:

```go
for id, write := range tx.scalarWrites {
    w := at(id)
    w.scalar, w.hasScalar = write, true
}
for key := range tx.propertyWrites {
    w := at(key.objID)
    w.props = append(w.props, key)
}
```

It does not already constitute an authoritative record: property payloads still
come from the transaction map at apply time (`store_cow.go:277`), and renames are
looked up separately (`:155-158`). The latter comment explicitly avoids adding
a rename field to every commit entry. Object grouping therefore has a measured
history of size sensitivity; simply embedding every nested map is not free.

### Private image is not a redundant final value

At `store_cow.go:195-202`, publication starts from live:

```go
live := s.liveObjectLocked(id)
if live.anonymous {
    w.img = live
    return
}
w.old = live
w.img = cloneObjectSharingVerbs(live)
```

At relationships :62-72, changes apply sequentially to the supplied contents
list; at cow :234-239 the supplied list is the publication image. Two moves into
the same room preserve both additions. Final private contents alone cannot say
which entries were inherited from the old snapshot and which were this task's
edits. Position-sensitive inserts are ordered transformations; the source's
"COMMUTATIVE" comments must not be used as proof they can be reordered.

Snapshot :69-120 separately implements first-write ownership of only requested
facets. Removing that cache would require materializing reads repeatedly;
publishing it whole would overwrite concurrent unrelated changes. A keeps both
the private materialization and the normalized intent.

## Smallest representation proposal

Reuse writeSet's existing slice plus optional object-ID index as the owner of
pending records, created lazily on first real write. A record retains scalar
presence flags, relationship deltas, pristine creation base and recycle marker.
Its typed property maps use canonical local names; its verb-code map uses stable
verb IDs; its verb deletion slice retains within-object order. This deliberately
retains the existing write forms. Do not introduce a tagged universal cell, a
command interface, closures, a persistent edit log, or broad object-level reads.

Separate publication scratch (`img`, `old`, and `joined`) from pending records.
Publication may reference authoritative records but must not persist attempt
images in them. Rebase property map entries before the applier reads their
payload; no copied stale property payload list may survive that rebase.

Keep the current definition-order derivation from the private object's propOrder,
including the fallback for definitions absent after live adoption. Making
definition order an independently maintained edit sequence would add another
authority and more rename/delete rules. With A it becomes a per-record ordering
function rather than propertyDefinesByObject's global grouping.

Do not store a record for every read object. For queries such as hasStagedWrites,
HasStagedTopology and onlyPropertyValuesStaged, inspect active record contents;
an allocated but cancelled-to-empty record is not a write. Read-mark suppression
must check the specific property/verb entry, not the presence of its object
record. Existing aggregate maps give these queries cheap length checks; replacing
them with per-record scans is a real cost, not an automatic performance win.

Prefer scans in the initial design over new synchronized kind counts. A cached
kind bitmask/count requires every cancellation, rename, reseed and reset to
maintain it; no measured problem justifies that extra state yet. If active
record scanning proves material, evaluate that evidence before expanding A.

Reuse the existing writeSet lookup strategy rather than adding a map of heap
allocated records. Its pointer lifetime remains explicit: `get/at` pointers
expire on slice growth. Do not retain a pointer across a descendant walk or an
operation staging another object; reacquire it. Keep publication-only descendants
in the compact joined image map. Direct writes must still skip descendant
transaction staging (`store_txn_property_inheritance.go:295-299`).

Direct writer reuse (`store_txn.go:190-217`) must preserve usable nested map
capacity or its ordinary-write allocation advantage can disappear. Resetting
one record must clear its semantic fields and nested maps. Existing retention
limit remains the bound, adapted to count retained records and actual staged
entries; cancelled records must not bypass it. Do not add a separate pool.
Exact layout and reuse mechanics are implementation choices, with allocation
parity still an experimental requirement rather than a fact.

## Non-obvious lifecycle obligations

### Renewal needs an independent footprint after writes disappear

Raw `store_txn_commit.go:61,91,101-105`:

```go
footprint := tx.writesByObject()
next, publishedWrites, errCode = tx.CommitAndRenew()
if footprint.len() > 0 {
    // ...
    for i := range footprint.entries {
        id := footprint.entries[i].id
```

Successful publication calls `clearStagedWrites` at cow :499; therefore replacing
footprint with an alias to the authoritative records would erase the information
before carried reads are rebased. Retain only the relevant object IDs in a
detached slice before commit. That is one O(written objects) copy (about 8–15
lines), not a second complete write representation. Keep existing behavior on
failure: the original transaction remains unreleased and its view remains usable
(`store_txn_commit.go:182-211`). This simplification does not authorize changing
which objects the existing footprint includes.

Hot renewal is different. `store_hot_read.go:240-242` refuses keyed state;
:283-292 restages only values and compares results; :306-320 also compares owner,
perms, clear, defined and hasOrig. The early predicate is not sufficient to
identify pure value changes because SetPropertyInfo uses propertyWrites too.
Do not transfer records blindly to the new snapshot or remove the comparison.
Update the existing renewal field inventory test (:269 onward) to describe the
new mixed record's restrictions; categorizing the whole field as simply carried
would be inaccurate.

### Forget and renumber are not ordinary map erase/move

`store_txn_live.go:169-230` removes scalar, relationship, property and verb writes,
but leaves creation bases and recycle markers. Preserve this exact distinction
in the first representation-only change. A full record deletion would differ.

Renumber's real caller (`builtins/objects_misc.go:73-91`) performs:

```go
tx.MoveStagedObjectWrites(oldID, newID)
tx.ForgetObject(oldID)
// AdoptLiveObject(newID), then:
tx.ApplyStagedObjectWrites(newID)
```

Transfer at live :235-295 deliberately excludes relationship changes, verb
deletions, creation and recycle. It merges selected kinds into the target;
self-owned scalar writes rewrite oldID to newID. Do not replace this with a
blind rekey, and do not rewrite arbitrary property-value object references.
The existing renumber test checks private visibility, no early publication,
unrelated writes and preservation of the prior snapshot.

Chparent has another real caller (`builtins/objects_hierarchy.go:222-232`):
ChangeParents, adopt live relationships, then ReseedInheritedProperties. The
reseed uses only staged property writes/deletes, drops ones absent from common
old/new ancestry, and preserves unrelated facets. A can remove its temporary
per-object grouping at inheritance :148-170; it cannot replace the specialized
property replay with full-object ApplyStagedObjectWrites. That would duplicate
relationship or verb edits already reflected in the retained private image.

### Failure and publication phases are observable

`store_txn.go:298-302`:

```go
if tx != nil && errCode != types.E_NONE && !tx.validationFail && tx.terminalErr == types.E_NONE {
    tx.terminalErr = errCode
}
```

Physical private state remains for error handling; HasWrites becomes false.
Do not clear authoritative records on terminal preflight failure. Retryable
validation loss remains distinct. Preflight first checks creation-ID occupancy,
then live targets, verb code, verb deletions, definitions, definition deletes and
renames (commit :360-409). Preserve these phase priorities by retaining phase
passes over the records; an attractive one-loop-per-object preflight changes
cross-kind error precedence. Within a kind the present map iteration is already
unordered; there is no basis for promising a stable first object today.

Publication must continue: head edits for all objects; optional live descendant
walk; tail edits for all objects; WAIF publication; clock advancement only after
every image; history pruning and invalidations; clear writes. Recycling is last
within an object's tail. Anonymous writes continue to select exclusive locking.

## What the algebra does and does not buy

The useful abstract law is V = apply(B, D), with commit applying D to current L
after validating observations R. Empty D does not imply empty R. Resolved edit
composition is associative as sequential function composition, but generally
noncommutative. Normalized records are an optimized summary of those edits, not
the edits themselves.

Raw property elision (`store_txn_properties.go:107-116`):

```go
orig, hasOrig := before, hasBefore
if existing, staged := tx.propertyWrites[key]; staged {
    orig, hasOrig = existing.orig, existing.hasOrig
}
if hasOrig && !orig.clear && orig.value.Identical(value) {
    delete(tx.propertyWrites, key)
```

The caller's earlier read mark survives. `property_elision_test.go:128` explicitly
checks a concurrent change still conflicts. A generic last-write-wins cell would
need orig/clear/existence and observation provenance anyway.

Map access is stronger evidence against collapsing intent into final state.
`store_txn_mapkeys.go:140-146` compares captured entry to the immutable baseline,
not to current written-key membership. Index evaluation may have changed the
slot after capture. `:295-329` applies only written keys to live map, retaining
unrelated entries and enforcing the earlier size limit. Whole reads, live
mutation and rename deliberately degrade this to whole-slot dependencies. These
are distinct transition rules; moving the map inside an object does not erase
them.

Renames are normalized from original published name to final destination, with
temporary intermediate names removed (`properties:520-535`). `renameSlots`
removes sources before installing destinations (`store_properties.go:547-588`).
The tests :187-269 cover swaps, written descendant slots and rename-then-define-
old-then-delete-new. Sequentially applying this normalized rename map is wrong;
replaying the original rename sequence would require retaining a new log.

Verb deletion has a separate private generation increment (`verbs:52`), while
publication stamps the store timestamp (`cow:326`). Those are not a single
uniform "set version" operation. The applier already serves publication and
renumber replay (`live:327-333`), with adopted versions restored afterward.

A bounded future D candidate would share only resolved leaf mutations (property slot
installation or scalar replacement) between staging and publication, keeping
read marking, normalization, facet ownership and version stamping outside.
Definition insertion, rename batches and positional edits require their own
laws. Existing renameSlots and applyContentsDeltas already share some of these
operations. No source-backed large net deletion estimate is justified for D;
do not sell an unbounded command interpreter as the next required step.

Rejected shortcuts: publish private images wholesale; derive all intent by
diffing base and private images; clear observations when changes cancel; replay
public MOO operations at commit (MoveObject captures time at relationships :148);
reapply accumulated D to an already-modified private image; put locked direct
descendants through ordinary transaction staging; treat every nonempty object
record as a coarse object read/write conflict.

## Deletion ledger and cost uncertainty

These are planning estimates in physical production lines, not a measured diff.
They exclude tests, comments removed merely to lower counts, and code moved to
another file. The gross slices below overlap only where explicitly marked.

| Existing responsibility | Gross removal/replacement opportunity | Replacement retained or added |
|---|---:|---:|
| writesByObject conversion body, cow :136-184 | 49 | 0 conversion; publication scratch setup still needed |
| propertyDefinesByObject :57-92 | 36 | 20–30 for per-object order and fallback |
| per-object rename selection + renamedAway :544-565 | 22 | 10–16 for local rename values/source check |
| ForgetObject write filters, live :177-229 excluding read filters | 35–40 | 8–15 to reset selected write kinds, preserving create/recycle |
| MoveStagedObjectWrites selected write loops :240-294 | 55 | 20–35 for selected-kind merge and owner rewrite |
| reseed temporary grouping :148-170 excluding read invalidation | 17–20 | 4–10 local lookup; specialized filtering retained |
| aggregate writer init/reset and map metadata | 20–35 | 20–40 for authoritative record reset and reuse |
| verb-delete validation length map | 8–12 | 3–6 per-object running length |
| new footprint IDs, active-record queries, scratch boundary | 0 | roughly 30–60 across these responsibilities |

Depending on representation details, these paired edits plausibly remove
40–150 net lines. Many additional lines change their key/lookup shape without
disappearing. The parent's earlier 100–250 range is optimistic at its high end;
there is no demonstrated 250-line net deletion. Even a neutral line-count result
could improve local reasoning, but that is a design judgment, not a size win.

Historical evidence: `experiments/2026-10-26-no-twins.md` records the initial
direct-writer regression, then repairs. Its final table contains:

```text
DirectDefinePropertyWide: master 14,085 allocs/op; final 12,110
CommitScratch/objects32: +B+J+N 114,321 B/op; final 98,704–98,706
```

The report attributes an intermediate wide case to extra staging clones and
per-child props slices, and the objects32 change to slice reservation. These
are historical measurements on the listed revisions, not current-head results.
They make record size, growth, per-object nested maps, locked-writer reuse, and
wide descendant behavior concrete cost concerns. A must measure those existing
shapes before claiming parity; no new benchmark framework is needed.

## Primary prior art checked

Read as source/official documentation, not as performance evidence for Barn:

- [Clojure LockingTransaction.java](https://raw.githubusercontent.com/clojure/clojure/master/src/jvm/clojure/lang/LockingTransaction.java), lines 100–104,262–284,441–464: keeps private values and deferred per-reference functions; doCommute calculates a private result, and commit reevaluates the deferred functions on current values. This confirms that a private materialization and intent can coexist without being redundant. Barn's positional edits and serializable read obligations cannot simply inherit Clojure's commute contract.
- [FoundationDB developer guide, atomic operations](https://apple.github.io/foundationdb/developer-guide.html#atomic-operations), lines 394–423 and 325: separates deferred transformations from reads; adding an ordinary read changes conflict behavior. It also preserves read-your-writes. The transferable idea is separate observations and changes, not importing FoundationDB or converting all Barn reads into weaker reads.

These moving URLs were checked during research on 2026-10-26; neither is a
selected dependency. No research paper text or extracted PDF content was used.

## Migration sequence if Q selects A and says make it so

1. Replace the eleven top-level write owners with the adapted object-owned
   representation and typed inner maps, retaining lazy allocation. Migrate
   staging and read-mark suppression together; do not maintain dual write owners.
2. Change commit/preflight/flush to consume records directly, preserving global
   phase ordering. Keep attempt images and joined descendants separate. Rebase
   keyed writes in the authoritative slot before tail application.
3. Migrate only-property eligibility/restaging comparison, creation validation,
   diagnostics, direct writer reset/retention and post-commit footprint capture.
4. Replace object-filtering in forget, selected renumber transfer, verb adoption,
   replay and property-only reseed. Retain original slot values and all adoption
   version restoration. Remove conversion helpers once their consumers migrate.
5. Adapt existing internal-record fixtures and the renewal field inventory.
   No public API, conflict selector, cache, GC, lock or root replacement belongs
   to this slice. Do not add a new guard or testing framework.

This is one coherent representation migration, not a sequence of separately
publishable states with competing authorities. Production correctness work stays
within the existing workflow. No unresolved design reading is deferred to the
go-ahead for this scope; actual implementation/performance evidence is absent.

## Bounded validation plan using existing workflow

Use PR CI, not local reruns of CI. Relevant existing bodies/fixtures include
property elision, rename swaps and destination reuse, different/same map entry
writes and captured reads, create-then-recycle, same-room moves, renumber staged
writes, chparent reseeding, shared facets, terminal/coarse preflight failures,
direct transactions, hot renewal and carrying-read boundaries. In particular,
renumber/reseed and hot renewal exercise record consumers outside ordinary commit.

Read `docs/elle-isolation.md` for the pinned checker requirement: CLI revision
9c2a18b3719f573dab06ae05c955c9c9d5225673, existing `scripts/check_elle.py` defaults
seeds 412,395,396 and the deliberate faults. Preserve raw EDN, manifests, logs,
witnesses and explicit healthy/fault verdicts with nonzero required paths through
the existing PR workflow. Ordinary unit tests alone are not isolation evidence.

Compare existing direct/CommitScratch/TxnPropertyWriteCommit/TxnReadOnlyCommit
benchmarks on the actual implementation base and candidate through the agreed
performance workflow. Reuse the wide direct definition and chparent-reseed
benchmarks. Historical timing on a loaded host is not acceptance evidence.
No performance run was undertaken for this research, and no claim of unchanged
speed, allocation, memory use or behavior is made.

## Surprises and unresolved choices

- A grouping conversion can disappear; its semantic obligations remain even if
  a future encoding combines some forms.
- Record lifetimes are not transaction lifetimes: carrying-read renewal needs
  IDs after clearing; failed commits need private state before any clearing.
- Forget and renumber intentionally have different selected-kind behavior.
- Hot renewal's value-only test is partly post-restaging comparison, not just a
  collection-kind predicate.
- The broader immutable-root option can coexist with A. It is not an alternative
  implementation of the same change.
- Choosing to invest in this modest cleanup versus investigating root publication
  first is Q's substantive choice. There is no source fact that decides it.
- If A's allocation or line-count outcome is worse, reject or reduce A rather
  than compensating with a general command framework, counters or new storage
  machinery. The evidence does not justify those additions now.

## Next step

Parent verifies report claims against current source and compares with the
independent immutable-root report. Present A as a bounded organization cleanup
with uncertain performance, and D as a limited useful principle whose universal
form is not recommended. Await Q's selection and explicit make-it-so before code.
