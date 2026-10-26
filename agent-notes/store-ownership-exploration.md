# Store ownership exploration

## Goal
Explore whether shared object ownership and operation implementations can greatly
reduce db/store, preserving behavior and performance. Research only; Q has not
requested implementation. Q explicitly requested pulling latest master first.

## State
- Pulled origin master with `git pull --ff-only origin master`:
  `5b0cd9ca..cae320f3`, fast-forward. HEAD and origin/master both resolved to
  `cae320f354038ee7b7d6fe8a27f4e150b42d0718` immediately afterward.
- Existing untracked files were left alone; tracked working tree was clean before
  pulling. This note is the only authored change for this exploration.
- Production Go physical lines (including comments/blanks, excluding *_test.go)
  are 11,981, versus 12,624 measured before pulling. That net 643-line reduction
  spans all fetched store changes; it is not a measurement of one refactor.
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

## Next step
Report that the original bet has already landed and that no tenfold reduction is
supported by this trace. If Q requests further design, work through the one
propagation pair before proposing implementation. Any implementation requires Q's
"make it so" instruction. Isolation/conformance/performance evidence remains
necessary before claiming preserved behavior or speed; use the project's existing
CI and Elle workflow, not additional local gates.
