# GC direct-reference index (branch perf/gc-direct-ref-index)

## Goal
Remove the whole-database property walk from the deferred GC sweep: keep a
per-object index of the anonymous and WAIF refs each directory object's
property values hold directly, refresh it where noteWaifRootsChanged is called,
and seed PersistentAnonymousReachability and PersistentWaifRootSet from it.
Design: agent-notes/mongoose-faceoff-2026-10-28.md "GC sweep cost" in the main
checkout; Q approved ("make it so").

## State
Implemented and committed locally on perf/gc-direct-ref-index; not pushed, no
PR. db/store/direct_ref_index_test.go (TestDirectRefIndexMatchesFullScan)
passes; go build ./... and go vet ./db/store pass. Full suite and conformance
not run locally (CI's job). Sweep cost on Mongoose not re-measured.

## Decisions and why
- The index covers directory objects only (refresh reads s.load(id)): both old
  scans walk s.dir only, so objects in s.anonObjects never seeded either walk.
  AddAnonymous and createAnonymous therefore add no entry.
- Commit refreshes every written entry and joined image when waifRootsDirty;
  the existing dirty test already proves the others' direct refs unchanged.
  Joined images only exist with defines/definition deletes (dirty) or renames
  (values unchanged).
- ChangeParents refreshes inside reseedInheritedPropertiesLocked: obj after
  rebuildInheritedSlotsLocked, each descendant after applyAncestorChange.
- WAIF top is built from holders sorted by id, so its order equals the old
  dir.forEach order.
- Removed the memo bumps in createAnonymous (object goes to s.anonObjects,
  which neither scan reads, and its slots are None) and Recreate (old image is
  recycled, so no entry; new slots come from copyInheritedPropertiesLocked,
  all inheritedSlot with value None).
- The full-scan oracles live only in the test file.

## Surprises
- There are 8 production noteWaifRootsChanged call sites, not 12.
- Mutation-checked the test: dropping the refresh at Renumber, direct Recycle,
  Add, the commit, or ChangeParents each makes it fail.

## Next step
Lead pushes/opens the PR; re-measure barn.gc_sweep_last_ms on the Mongoose
world.
