# Store ownership exploration

## Goal
Explore whether shared object ownership and operation implementations can greatly
reduce db/store, preserving behavior and performance. Research only; Q has not
requested implementation. Q explicitly requested pulling latest master first.

## State
- Pulled origin master with `git pull --ff-only origin master`:
  `5b0cd9ca..cae320f3`, fast-forward. HEAD and origin/master both resolved to
  `cae320f354038ee7b7d6fe8a27f4e150b42d0718` immediately afterward.
- Master advanced during research. Fetched again and rebased the two local
  note-only commits onto `b91e36f5fe98aa665da9e4ae3ee2bb1f4e847c18` successfully.
  Existing untracked files were preserved. The intended committed changes are
  this note and the two linked research reports; no production changes.
- Production Go physical lines (including comments/blanks, excluding *_test.go)
  were 11,981 at cae320f3, versus 12,624 before the initial pull. They are 12,144
  at b91e36f5. These totals span all fetched store changes, not one refactor.
- Unless explicitly marked as the b91e36f5 refresh below, source line references
  and deletion ledgers describe cae320f3, the initial research base.
- No tests or benchmarks run; no performance or isolation verdict claimed.

## Decisions and why
The original proposed property-write unification is already present on this base.
Do not propose implementing it again or restoring the old objectImages interface.

Current property-write trace:
1. vm/op_property.go:332,350 calls txn.SetPropertyValue after permission checks.
2. db/store/store_txn_properties.go:351 has one operation body. Direct mode enters
   Store.writeNow and calls that same method on a locked private writer.
3. store_txn.go:226 locks the store, obtains its reusable writer, runs the operation,
   and calls publishStagedLocked only after success with staged writes.
4. store_txn_snapshot.go:69 obtains private ownership of requested object facets;
   :102 clones only the missing facets. SetPropertyValue requests facetProperties.
5. store_txn_properties.go:98 records the write, retaining its original value for
   net-no-op elision. The private image provides read-your-writes.
6. store_cow.go:190 builds the publication image from the live object (or creation
   base); :275 applies staged property changes. Publishing the entire old private
   snapshot instead would require proving unrelated committed changes survive.

Remaining bounded candidate: share the descendant traversal/slot decisions for
property definition propagation. store_txn_property_inheritance.go:295-369 and
store_properties.go:398-455 are approximately 133 physical lines together.
Both walk numbered children recursively and anonymous children separately, skip
local definitions, and replace inherited slots. Transaction mode additionally
records relationship/property dependencies and stages writes; publication mode
owns commit images and stamps versions. store_cow.go:390 chooses live propagation
for its walkLive path; the locked writer skips staged descendant propagation.

Smallest prospective design: one propagation traversal with the existing
transaction and commit-image mechanisms supplying reads and slot application.
Keep read marking, staging, version stamping, and publication timing explicit.
Rough exploration estimate: a 60-80-line shared traversal plus 30-50 lines of
mode-specific operations, yielding only tens of net deleted lines for this pair.
This is an estimate, not a coded or measured result. If the necessary machinery
costs as much as the duplicate walk, leave the pair alone. Do not generalize to
all object fields, introduce a universal cell model, or remove write sets on this
evidence. Deletion and rename walks are separate follow-up candidates, not scoped
implementation work.

## Surprises
Latest master already contains the principal no-twins change. Earlier local
source was accurate for its checkout but no longer describes current master.
The remaining duplication is partly the distinction between a private task view
and publication on current state, rather than two public operation implementations.

## Deep exploration requested by Q

Q: "Yes, let's actually deeply explore and find the truths". This expands the
research beyond the first-pass propagation pair above. No implementation is
authorized. Source base remains cae320f3; 206482cd is the first note-only commit.
The earlier tens-of-lines estimate describes only one pair, not the store's
architectural possibilities. Current reading covers transaction representations,
validation, publication, renewal, live adoption, map-key access, property lookup
and mutation, relationship mutation, verb edits, object history, WAIF history,
property-table sharing, checkpoint capture, and representative production callers.
It is not a completed correctness audit of every store operation.

### The useful algebra

For an ordinary optimistic transaction, distinguish:

- B: the snapshot state on which it ran.
- R: observations that constrain a successful commit.
- D: its pending changes, including ordered edits where necessary.
- V: its private, materialized view, conceptually apply(B, D).
- L: current committed state when publication is attempted.

Publication conceptually validates R against L, checks operation preconditions,
then applies D to L and publishes atomically. It does NOT generally replace L
with V. This is a model of the ordinary path, not a claim that the existing
live-mutated and renewal paths already have one immutable B throughout.

The change identity is "do nothing". Ordered changes compose as state
transformations, but composition is not generally commutative. Normalizing a
change sequence must preserve successful intermediate observations, errors,
definition order, and dependency provenance, not only its final values.
An identity change does not imply an empty observation set.

Concrete source boundaries:

| Constraint | Source and evidence |
|---|---|
| Apply edits to current state | store_cow.go:190 clones the live image; :275 applies pending property writes; store_txn_mapkeys.go:293 rebases keyed writes on the live map |
| Retain dependency after no-op | store_txn_properties.go:351 marks the original read before same-value elision; property_elision_test.go:128 expects a conflict if that value changes |
| Preserve distinct map entries | store_txn_mapkeys_test.go:82 commits both different-entry writers and asserts a=2, b=11; :102 expects same-entry writers to conflict |
| Capture matters | store_txn_mapkeys.go:130 compares the captured map entry to baseline; the current write set alone cannot classify the earlier observation |
| Shape differs from value | store_txn_snapshot.go:245 records shape versions; property_shape_scan_test.go:65 expects an unrelated value write not to invalidate a fall-through lookup, :79 expects a definition to invalidate it |
| Relationship edits are ordered | store_txn_relationships.go:62 applies positional contents deltas in order; :123 stages movement and :147 captures last_move.time once |
| Local reads need private changes | vm/op_property.go:332,350 calls SetPropertyValue; its body updates the private image and records the pending write |
| Rename is not two arbitrary assignments | store_properties.go:547 removes all old names before installing destinations; property_rename_test.go:187,216 cover swaps and written slots |
| Identity and order differ | store_txn_verbs.go:23 records deletions by successive private-list index, while code writes use stable verb IDs |
| Errors retain a usable private view | store_txn.go:298 markTerminal preserves physical maps for error handling while prohibiting later publication |

The tests cited here were READ, not run. They document existing expected results,
not a fresh passing verdict. Formulae are reasoning aids, not an equivalence proof.
In particular, the source calls room edits "commutative", but position-sensitive
list inserts need not produce the same order when reversed. Preserve serial
application order; do not derive a CRDT or arbitrary reordering from that comment.

### A. Strongest bounded candidate: make pending writes object-owned

Current organization is field-kind first: StoreTxn has separate maps for scalar,
relationship, property, verb, creation, and recycling changes. Commit reconstructs
an object-first representation. Renumber and forgetting then repeat object filters.

Evidence:
- store_cow.go:136-184, writesByObject, translates the maps into objectWrites.
- store_txn_properties.go:56-91 reconstructs definition order by object;
  :544 filters all renames to get one object's renames.
- store_txn_live.go:169-228 filters maps for ForgetObject;
  :235-295 moves selected pending writes to a new object ID.
- store_txn_property_inheritance.go:148-179 builds another per-object grouping
  of staged slots for a changed subtree.
- A production-only search found 187 lines referring to the eleven write
  collections across 13 files. This is a migration surface count, not a saving.
- Real caller: builtins/objects_misc.go:73-91 renumbers, transfers pending writes,
  forgets the old identity, adopts the new image, and reapplies those writes.

Smallest design worth implementing if separately authorized:
1. Make the existing writeSet/objectWrites concept the authoritative pending
   write collection, rather than a temporary commit-time conversion. Keep object
   IDs as its outer key and typed slot/verb keys inside each object's record.
2. Preserve the existing change forms and phase ordering: scalar replacements,
   ordered relationship deltas, definition deletion/rename/definition ordering,
   property writes/deletes, stable-ID code writes, ordered verb deletions, and
   recycle-last. Preserve the pristine creation base and original property values.
3. Keep the current private object cache and ownership facets. Do not remove
   materialized views, change conflict granularity, or adopt a new storage engine
   in this slice. Keep read sets unchanged initially.
4. Have commit, replay, preflight, and subtree reseeding consume the same records.
   Recreate publication-only scratch as needed; do not preserve a failed attempt's
   old/img publication pointers as authoritative transaction state.
5. Replace object-filtering write-map walks with direct record access. Renumber
   still transfers only the kinds the current method transfers; it is NOT a blind
   record move. It must retain the owner self-reference rewrite and topology rules.
6. Preserve definition order from the private object's propOrder; keep ordered
   verb-deletion entries per object. No iteration-order change may alter the first
   preflight error or the classification of terminal versus retryable failure.
7. Keep joined publication-only descendants small. Do not turn a wide direct
   define into thousands of full transaction records. Reuse the current compact
   entry/index approach only with its pointer rule: a get/at pointer expires when
   at grows the slice; reacquire after any nested operation that may add an entry.

Size estimate: roughly 500-900 changed production lines across the write users,
with perhaps 40-150 net lines removed, plus necessary existing-test adaptations.
This is a planning estimate, NOT a measured diff or a quota. The important deletion
is the representation conversion and repeated ownership bookkeeping. It can still
fail to pay for itself if per-object nested maps and access methods grow too much.
No speed or allocation improvement is established. Keep read-only transactions
free of write records; do not make every object read pay for a large write record.

Historical performance evidence makes that last point concrete:
experiments/2026-10-26-no-twins.md records an intermediate DirectDefinePropertyWide
allocation regression (25,282 versus 14,085) and its later repair by omitting
descendant staging on the locked writer. Its final table reports 12,110. These are
recorded past measurements, not rerun results or current-head performance proof.
The source's tx.locked short-circuit in propagateDefinedProperty and the
writeSet.joined path implement that distinction now.

### B. Real but smaller: remove duplicated property payload

propertyWrite holds both value and prop.value (store_txn_properties.go:20).
All five production constructors populate them from the same value; staging sets
prop.value before constructing the record (:98). Map rebasing explicitly updates
both together (store_txn_mapkeys.go:328). Renaming moves the record unchanged
apart from its name. Consumers split between the two copies in applyTailWrites,
hot renewal, and map rebasing.

Smallest design: retain prop.value as the authority and remove the redundant value
field, changing those constructors and consumers. Approximately 15-30 production
lines touched, a handful deleted; tests constructing internal records need matching
adaptation. This is a concrete redundancy, not the architectural answer by itself.
Do not conflate it with eliminating orig: orig preserves net-no-op bookkeeping.

### C. Observations can share bookkeeping, not one coarse version

The seven ordinary version-read maps implement a common constraint shape:
selected resource still has the observed version. store_txn_validation.go:66-96
already shares validation of the five object-level kinds. A typed resource key
could unify recording, copying, dropping, and renewal without removing selectors.
However, map-key reads validate entry values/presence plus slot metadata, WAIFs
validate image timestamps, and memo hits use a shape epoch. They are not simply
another object-wide timestamp.

Potential removal: repeated map copying/rekeying in CommitAndRenewCarryingReads,
renewAtCurrentClock, and ForgetObject. Replacement cost: resource-key layout,
lookup switches, deterministic validation-stage order, fault hooks, conflict
diagnostics, and the provenance needed to exclude new hot-read observations.
Since the validator itself is only 183 physical lines and is already partly
shared, do not sell a universal dependency map as a large deletion on its own.
No implementation proposal is selected here; A is independent of this change.

### D. Larger algebra: one change description, two materializations

The same D could drive both apply(B,D) for reads and apply(L,D) for publication.
That can remove handwritten private mutation plus handwritten commit mutation,
especially in scalar/property paths. Existing applyHeadWrites/applyTailWrites
already serve commit and ApplyStagedObjectWrites; this is an extension of a real
shared applier, not a new command framework.

The minimal representation must distinguish replacements from ordered edits.
It must also keep observation R independent of normalized writes D. Storing only
the final private image discards which changes should be rebased; storing only an
operation log adds replay and normalization costs. A private view may remain a
cache of D rather than a second authority. That is a useful design direction, but
no measured net deletion estimate is defensible yet for all property/topology cases.

Counterexamples to attractive shortcuts:
- Publish V whole: loses another commit's different map entry or same-room move.
- Compute D only by diffing B and V: cannot in general recover operation intent,
  captured reads, or the required positional edit order from final values alone.
- Drop R when D cancels: loses a dependency the no-op test explicitly requires.
- Replay MOO operations at commit: repeats callbacks/time acquisition/permission
  decisions. Only already-resolved internal edits belong in the commit applier.
- Put every mutation through a uniform descendant-staging path: reintroduces the
  wide-direct-write cost documented above.

### E. The architectural alternative: persistent world roots

Concept: a snapshot retains an immutable root from IDs to immutable images.
Commit validates against the current root, applies the transaction's edits to
that current root, and publishes one replacement root. Old roots retain their
reachable images; Go GC reclaims roots and structure no reader retains.

This could replace numbered-object history lookup, reader-floor registration,
and pruning, and make the numbered-object publication boundary one root swap.
The directly relevant current files are object_dir.go (135 physical lines) and
store_history_gc.go (368), plus history/publication parts of store_core.go,
store_txn_snapshot.go and store_cow.go. Those 503 lines are an affected surface,
NOT 503 guaranteed deletions: the new persistent directory has implementation cost.

Why it is not already a drop-in design:
- objectDir.slot uses dense-ID segment indexing; a persistent tree adds a different
  lookup/update cost and path-copy allocation. No Barn workload measurement exists.
- Anonymous objects currently lack numbered COW slots and histories and are copied
  on read/in-place on write (store_txn_snapshot.go:122; store_core.go:497). They must
  be covered coherently without confusing runtime identity and serialization IDs.
- WAIF state has a separate domain-bound version chain in types/waif_image.go:28-95.
  Merely swapping the object root does not snapshot mutable WAIF referents.
- Semantic GC follows retained WAIF images (types/waif_image.go:115 and
  store_reachability.go:12). Go reachability is not a substitute for those MOO
  lifecycle rules. Old roots may retain an entire world's old bindings, not just
  objects a task actually consulted.
- SnapshotWithRoots also rewrites anonymous IDs and external task values; its
  451-line file is not all snapshot isolation machinery that a root pointer deletes.
- Coarse mutation, checkpoint capture, and irreversible effects still need the
  correct boundary. Serializing publication already exists; root publication
  does not justify serializing whole task execution or broadening conflicts.

Verdict: a real alternative with potential mechanism deletion, not a demonstrated
improvement and not an implementation-ready replacement. It needs an isolated
design/measurement study including the points above before production work.

### F. Remove the mixed live/private mode?

This is another substantial potential deletion: store_txn_live.go is 454 lines,
with additional adoption/reseed/memo logic elsewhere. But real callers still need
it: renumber above, chparent adoption in builtins/objects_hierarchy.go, verb-info
adoption in builtins/verbs.go, and coarse movement in objects_movement.go:160-165.
The engine acquires the exclusive gate and renews at the irreversible boundary
(engine/task_runtime.go:296-337); this alone does not prove that replacing the
remaining task view with DirectTxn is safe. Programming input also calls direct
SetVerbCode (server/input_processor.go:839), and writeNow itself takes store.mu,
not the task commit gate. Outer exclusion of every direct writer was not audited.

Removing the mixed mode would require bringing the remaining coarse operations
under the same transaction authority or proving a complete live-execution mode.
It must preserve callback-visible changes and exception-prefix effects. This is
not a helper extraction or a safe deletion established by this exploration.

### Prior art checked against primary sources

- Clojure's documented Refs separate ordinary updates, commute and ensure:
  https://www.clojure.org/reference/refs . Its implementation retains both vals
  and deferred commutes; doCommute calculates a private value (lines 441-464),
  and commit reapplies commutes to the current value (262-284):
  https://raw.githubusercontent.com/clojure/clojure/master/src/jvm/clojure/lang/LockingTransaction.java .
  This supports the change/view distinction; it does not establish Barn equivalence.
  Clojure documents snapshot isolation, so it is not an automatic replacement for
  Barn's serializable validation contract.
- FoundationDB documents conflict ranges separately from atomic mutations:
  https://apple.github.io/foundationdb/developer-guide.html#conflict-ranges and
  https://apple.github.io/foundationdb/developer-guide.html#atomic-operations .
  The relevant lesson is separating observed state from deferred changes, not
  weakening reads or importing a distributed database into Barn.
- HashiCorp's immutable radix implementation holds a root per tree and a modified
  root per transaction, with writable-node reuse:
  https://raw.githubusercontent.com/hashicorp/go-immutable-radix/master/iradix.go
  (Tree at 26, Txn at 48). This is concrete prior art for E, not a selected dependency
  or evidence it beats Barn's dense-ID directory. External source URLs are moving
  branches as read during this investigation, not pinned implementation inputs.

### What remains unknown, and verification for any approved implementation

No code, tests, prototype, benchmark, or live workload was run or changed in this
exploration. No new CI, tests, gates, or instrumentation were introduced. Existing
test bodies and historical measurement records are evidence for requirements and
past tradeoffs, not fresh verification. No Toast behavioral assertion is proposed
or changed, and no oracle result is claimed.

For A/B, use the existing property-elision, map-key, rename, move/create,
renumber/reseed, commit-atomicity, immutable/shared-facet, direct-reader, and renewal
tests in the PR's existing CI. Use the pinned Elle workflow described in
docs/elle-isolation.md, keeping raw histories, witnesses and nonzero coverage.
Compare the existing direct/commit benchmarks and real-workload evidence against
the actual implementation base before claiming performance parity. Do not run CI
locally to gate a push or add new safeguards as part of this research.

The investigation supports simplification opportunities, especially eliminating
representation conversion. It neither proves a tenfold reduction possible nor
proves it impossible. File splitting, deleting documentation/tests/diagnostics,
or moving storage complexity into a dependency is not a tenfold simplification.

## Completed comparison of the two research directions

Both forked Astra research reports are complete:

- [Object-owned changes](store-write-records-research.md): eleven write forms,
  lifecycle callers, algebra, deletion ledger, bounded migration and prior art.
- [Immutable roots](store-immutable-roots-research.md): numbered-only and full-world
  alternatives, publication, retention, WAIF lifetime, checkpoint ownership and
  replacement-cost ledger.

The parent read both reports and checked the central claims against current
source. These are research conclusions, not implementation or runtime results.

| Direction | What it removes | What remains | Estimated net production size |
|---|---|---|---|
| Object-owned pending changes | Regrouping eleven write collections and repeated object filtering | Typed edits, observations, private views, publication phases, detached renewal footprint | 40-150 lines removed |
| Numbered-object immutable roots | Ordinary numbered history lookup/insertion/pruning and mutable directory slots | Fine validation, rebasing, WAIF floors, anonymous behavior, checkpoint coordination, diagnostic journal | 150 added to 250 removed |
| Full immutable world roots | Potentially both numbered and WAIF version chains/floors | New identity resolution, detached WAIF baseline, semantic retirement, retained-root accounting, external task capture | 950 added to 100 removed |

All figures are unmeasured planning ranges, not promised diffs. Full-world figures
include the numbered slice and depend on an unresolved lifecycle design. No
performance parity has been measured. Neither report establishes a tenfold
reduction, and neither proves that some other design could never achieve it.

Source checks that materially affected the conclusion:

- store_txn_commit.go:61 captures a footprint before commit; store_cow.go:499
  clears writes; renewal later uses the footprint. Authoritative records need
  detached IDs across clearing. This reduced A's initial optimistic estimate.
- store_txn_live.go:169-295 selectively forgets/transfers kinds. Replacing these
  loops with unconditional record deletion/transfer changes existing behavior.
- store_core.go:497 publishes a fresh clone before its caller edits it under the
  exclusive store lock. BeginSnapshot at store_txn.go:135 already takes RLock.
  Keeping that acquisition lock avoids requiring a new coarse-operation builder;
  shared-lock ordinary commits still need one final coherent root publication.
- store_history_gc.go:228 already prunes versions between reader timestamps.
  Immutable roots must be compared with that reader-aware baseline, not a claim
  that today's store retains every intermediate version since the oldest reader.
- store_conflict_census.go:328 walks intervening images to classify stale reads.
  A root design must preserve this enabled diagnostic capability, for example
  through a diagnostic journal; deleting it is not equivalent simplification.
- types/waif_image.go:28 binds detached WAIF baseline images; :115 visits retained
  images for semantic liveness. A strong persistent map of all WAIFs does not
  explain retirement or semantic GC. Full roots remain a larger design question.

The two directions are compatible: one owns pending changes; the other owns
committed snapshots. Choosing one first is a choice of investment, not an
exclusive architecture fork. For immediate simplification, A is the bounded
candidate. For investigating a different snapshot representation, numbered roots
are the bounded experiment. Full-world roots are not a finished production plan.

## Latest-master refresh: b91e36f5

Read the production delta from cae320f3, including vm/op_property.go. PR #548
adds WAIF value propagation for class-property renames/deletes; #549 changes
readTS padding documentation and adds a benchmark. No fresh performance result
is inferred from those comments.

In store_txn_waif.go, moveUnstagedWaifValuesLocked reads propertyRenames and
propertyDefinitionDeletes, traverses live descendants, resolves the weak class
registry and publishes affected WAIF images. store_cow.go:466 calls it before
staged WAIF publication and the commit clock CAS. Object-owned records must
migrate this additional consumer without dropping its commit-time work. Numbered
roots must keep this publication timestamp coherent with the numbered root;
full-world roots must include these affected WAIF images in the same publication.
The new code reinforces the distinction between private edits and applying
changes against current live state. It does not establish additional deletions.

vm/op_property.go now calls ReplaceWaifProperty for already-held values and
findPropertyForRead for absent names before SetWaifProperty. A general map update
cannot replace this path without retaining those observations. This is a Barn
source observation only; no new Toast behavior claim or conformance run is made.

The report estimates remain approximate ranges from the initial ledger, with
additional migration surface from #548. They are not recalculated measured diffs.

## Next step

Q requested a further agent investigation of the central unresolved question:
Can one ownership model cover numbered objects, anonymous objects, WAIFs and
retained snapshots while preserving behavior and eliminating enough machinery
to make the whole store simpler?

Smallest authorized deliverable: extend the existing immutable-root report with
a source-backed answer, approximately 150-300 Markdown lines if needed; zero
implementation lines. Reuse the Astra immutable_world_roots researcher and its
existing report. No new production protocol, representation or dependency is
selected by this research request. Treat the parent's proposed "one ownership
model" as a hypothesis to test, not a required affirmative conclusion.

The report must distinguish Go allocation reachability, snapshot visibility,
identity/domain ownership, MOO semantic liveness/finalization and checkpoint
capture; show whether these can actually share authority. Compare the simplest
unified proposal against keeping separate owners. Account for replacement code,
retained memory, publication and hot-path costs, and identify counterexamples
using real producers. Do not claim performance or behavioral parity from source.

Do not code until Q says "make it so". No implementation, tests or benchmarks were
changed or run in this research. Research notes are the only intended commit
content; no push or PR publication is part of this task. Parent will check the
agent's substantive claims against primary source before relaying its answer.

## Central-question result

The Astra extension is complete in store-immutable-roots-research.md under
"Central ownership hypothesis". Parent independently checked the two decisive
counterexamples against current production code and the existing test bodies:

- vm/vm.go:558 releases a previous local into pending bookkeeping;
  vm/waif_gc.go:36 strongly retains the WAIF Value. vm/value_roots.go:132 labels
  it pending, and vm/waif_gc.go:119 excludes that kind from live VM roots.
  engine/waif_lifecycle.go:313 tests semantic liveness before :recycle, whose
  context and local this still receive the WAIF at :485/:498. Allocation
  reachability and eligibility for finalization cannot be one unqualified rule.
- store_txn_waif_test.go:218 requires historical WAIF edges to protect children
  until reader release, while :262 requires those edges not to erase checkpoint
  pending ownership. store_pending.go:91/99 passes false for retained traversal;
  store_reachability.go:263 distinguishes retained-image traversal. Tests were
  read, not executed. Snapshot retention and export ownership select different
  edges even when sharing an image representation.

Conclusion: a shared image/publication implementation remains possible, but a
single unqualified liveness rule cannot replace these existing contracts. This
does not prove typed representation unification impossible; substantial net
deletion is unproven. No full-world rewrite is recommended from this evidence.
No code, tests or benchmarks were changed/run. The research answer is complete;
further implementation or architectural investment awaits Q's direction.

## Follow-up: typed-reference graph

Q challenged the inference: a pending recycle reference can be a distinct kind
of edge in one model. The parent agrees. The earlier counterexamples exclude
unqualified reachability, not shared ownership machinery with typed references.
Treating different meanings as proof of separate machinery was too strong.

Q authorized continuing research. Smallest deliverable: extend the existing
immutable-roots report with a concrete typed-reference model and source-backed
transition examples (roughly 150-300 Markdown lines, zero implementation lines).
Reuse the existing Astra researcher. Determine traversal state, propagation and
root-selection rules; show whether pending callback retention, semantic liveness,
snapshot visibility and export ownership follow from those rules. Compare with
existing typed VM roots and retained/current walkers; count actual deletions and
replacement obligations. No generic graph framework or code is authorized.

Parent's fresh source checks for this follow-up:
- vm/value_roots.go:5-12 already labels live and pending roots.
- vm/waif_gc.go:52 and db/store/store_reachability.go:263 have matching recursive
  WAIF/list/map closure logic with retained/current selection. This is a concrete
  shared-walker opportunity, not yet evidence for deleting lifecycle machinery.
- engine/waif_lifecycle.go:65 captures OwnRefs at defer time; :307-316 adds those
  captured identities to a live set computed before the callback batch. A model
  must preserve capture phase as well as edge kind. Rewalking current edges or
  recomputing after each callback is not source-equivalent by assumption.
- types/waifset.go:40 suppresses repeat expansion by identity; sharing a visited
  set across different views/modes would require additional reasoning. Separate
  calls to a shared traversal can preserve today's smaller identity set.
- Racket's official will-executor documentation explicitly separates eligibility
  from retaining the registrant for callback execution. This supports the typed
  reference interpretation, not automatic Barn equivalence:
  https://docs.racket-lang.org/reference/willexecutor.html (read 2026-10-26).

The Astra typed-model writeup is complete in store-immutable-roots-research.md
under "Constructive follow-up: typed references". Parent checked its core
construction against the paths above, store_snapshot.go:308-331 (detached export
payload with same identity), store_pending.go:83-113 (current-graph ownership
subtraction), and vm/anonymous_gc.go:423-491 (captured candidate set and ordered
callback contexts). No code/tests/benchmarks run.

Positive result: a virtual graph with typed roots, selected images and captured
sets can express these cases together. A pending edge keeps callback data but
does not seed semantic reachability. Callback this supplies an ordinary live
edge. Query state contains projection, image selector and capture/view; it does
not require a newly allocated graph or new global registry. Frozen members emit
identities without re-expanding present-day properties. Checkpoint copies remain
separate payloads despite equal identity. Anonymous resolution stays with its
existing store owner and lock scope.

The direct/current/retained traversal table is a concrete unification, not a
proof of measured code reduction. Existing root admission, closure, set union,
candidate subtraction and export transformation fit the model; histories,
capture timing, callback contexts and publication still perform necessary work.
The verified duplicate WAIF walkers supply a bounded deletion opportunity. Wider
value traversal consolidation needs its adapters counted; no large savings or
performance parity established. Research is complete for this question. No
implementation is authorized; present the positive model and its limits to Q.

## Next reduction: simplify the typed model itself

Q asks to simplify further. Smallest authorized deliverable: refine the existing
research, not code. Reuse the Astra researcher to challenge whether edge labels,
projection modes and frozen-member nodes can disappear into ordinary root sets,
image resolution and materialized results. Parent independently traces the
smallest traversal and scheduling boundaries. Approximately 100-200 Markdown
lines of additional derivation if needed; zero implementation lines.

Test candidate: reachability is one operation on a selected view; pending is an
ordinary candidate collection kept outside its live roots; a frozen closure is
an ordinary result set owned by a phase, not a special graph edge. Preserve
existing first-seen order, identity/view distinctions, anonymous resolution and
sampling times. Determine what is a necessary input versus an accidental policy
dimension. Do not require a physical unified heap or universal callback engine.
No implementation authorization or performance claim follows from this question.

Result: the smaller construction is ordinary closure over chosen adjacency,
set operations, and ordered effects. Pending remains a retained candidate list
excluded from live roots; captured closures are ordinary results owned by callers.
No edge-color propagation, graph registry, view nodes or combined-mode visited
state is necessary. Keep existing logical identity keys and separate walks/views.
The report's "Smaller construction" section supersedes the expanded typed-edge
model as the preferred formulation.

Parent read the new section and checked the central code opportunities:
- Shared value recursion in vm/waif_gc.go:52 and store_reachability.go:263;
  direct/anonymous collectors provide related potential sharing with adapters.
- vm/anonymous_gc.go:175-257 has two cover reducers: candidate closures sorted
  descending by size, then selected only while uncovered. Anonymous ties use ID;
  WAIF ties preserve input order. Sharing is conditional on adapters shrinking it.
- shutdown_roots_test.go:12-96 documents current-edge selection and parent covering
  child; these bodies were read, not run.
- persistentWaifRootSet:368-406 reuses top-level inputs separately from the expanded
  closure. Deleting caches would introduce recomputation, not algebraic elimination.
- engine/internal/finalization/coordinator.go:19 and engine/waif_lifecycle.go:97,
  :309,:322 retain OwnRefs and DirectRoots for distinct later consumers. These
  capture different populations and times; one is not a lossless replacement for
  the other. Their backing slices/maps are shared across entries, not deep-copied.

Research conclusion: simpler model, concrete bounded traversal consolidation and
a possible second common reducer. No measured net reduction, behavior parity or
performance result. No implementation/tests/benchmarks run. Present this result;
implementation remains contingent on Q's "make it so".
