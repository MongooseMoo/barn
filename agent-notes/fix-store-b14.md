# fix/store-b14: history keeps the committed image at a commit timestamp

## Goal
B14 (db-store review): a coarse commit can supersede one object's published
image more than once at its commit ts T. Every history append skipped an entry
whose ts equalled the newest entry's ts, so history kept the FIRST (intermediate)
image at T, and a snapshot with readTS T read it once a later commit superseded
the final image. Q approved the smallest fix: a same-ts entry replaces the newest
entry. Store tests only, no conformance PR.

## State
Done on branch `fix/store-b14` (worktree `C:/Users/Q/code/barn-store-b14`), not
pushed. PR body drafted in the session scratchpad (`pr-b14-barn.md`).

## Decisions and why
- One helper `withHistoryEntry` (db/store/store_core.go) holds the rule; every
  history append calls it: rememberObjectLocked, republishForMutation
  (store_core.go), rememberOldImageLocked (store_cow.go, decentralized, under
  historyMu), SetVerbArgs (store_verbs.go). The prune sites in
  store_history_gc.go only shrink history and are unchanged.
- The replacement allocates a new slice instead of writing the last element in
  place: loadSnapshotObject (store_txn_snapshot.go) and the conflict census
  (store_conflict_census.go) capture the slice header under historyMu and walk it
  after unlocking.
- Replacing is right only if no section republishes an object, mutates the fresh
  image without stamping it, and republishes it again. The audit below checks
  every site; the only way to reach that is a self-cycle, which builtins reject.

## Call-site audit (all non-test history writers; every section holds s.mu.Lock unless noted)
The replace rule is wrong only if a section republishes object X, mutates the
fresh image WITHOUT stamping it, and then republishes X again: the second
superseded image carries the old version V, matches the entry (V, committed
image) the first republish recorded, and would replace it with uncommitted
content. "Bump" = bumpClockLocked relative to the first republish.

| Site | Section | Same object republished again before its stamp? | Bump |
|---|---|---|---|
| store_core.go:622 setObjectName, :638 setObjectOwner, :654 setObjectLocationRaw, :670 setObjectFlag | single republish | no | after |
| store_lifecycle.go:37 createObject parents | parents republished in a loop, attach mutates after the loop, stamp :59 | only on duplicate parent ids; both copies are unmutated clones (harmless) | before (:13) |
| store_lifecycle.go:141 Recycle obj | obj mutated :191/:201/:203, stamped :217 | only if obj is its own child (:169), new parent (:176), content (:186), location (:196) or parent (:208): self-cycles | after (:142) |
| store_lifecycle.go:169/176/186/196/208 Recycle relatives | each stamped on the next lines | no: a repeat (e.g. a child that is also a content) finds a T-stamped image | after |
| store_lifecycle.go:244 Recreate rememberObjectLocked | records the committed tombstone, unmodified | no | after (:245) |
| store_lifecycle.go:263 Recreate parent | attach :264, stamp :265 | no | after |
| store_lifecycle.go:384/386 Renumber rememberObjectLocked | records committed recycledTarget and obj, unmodified | no | after (:387) |
| store_lifecycle.go:440/449/458/467/475/502 Renumber forEach | each republish stamped on the next lines | no: repeats find T-stamped images (e.g. parent and child of oldID; a self-owned renumbered object at newID) | after |
| store_properties.go:422 setPropertyInfo, :447 setPropertyValue, :565 clearPropertyOverride | single republish | no | after |
| store_properties.go:492 definePropertyLocked, :535 deleteDefinedPropertyLocked | definer stamped :511/:548 before the descendant walk | no | after when ts==0; the applier passes its ts |
| store_properties.go:629/632 propagate, :663 removeInherited | each child stamped :637/:665 | a diamond child repeats and finds a T-stamped image | as caller |
| store_properties.go:677 anon child | anon: republishForMutation returns it, no history | n/a | as caller |
| store_relationships.go:47/53/63 moveObject | each stamped on the next lines | where == oldLoc finds a T-stamped image | before (:42) |
| store_relationships.go:317 ChangeParents old parents | stamped :325 | no | before (:309) |
| store_relationships.go:328 ChangeParents obj | parents mutated :329, stamped :337 | only if obj is one of its new parents (:333): self-cycle | before |
| store_relationships.go:333 ChangeParents new parents | attach :335, stamp :340 | duplicate new parent ids: unmutated clones (harmless); an old parent repeated finds a T-stamped image | before |
| store_verbs.go:379 AddVerb, :481 deleteResolvedVerbLocked, :514 SetVerbInfo, :588 setVerbCode, :613 setVerbCodeByIndex, :552 SetVerbArgs inline, :557 SetVerbArgs anon | single republish; E_VERBNF returns after a republish leave an unmutated clone | no | after |
| store_txn_commit.go:577/597/642/670/687/703/718 applyStagedToLiveLocked (also reached from store_txn_live.go:419) | each stamped before the next loop; `remembered` skips repeats; define/delete republish unconditionally but find T-stamped images | no | before (:540) |
| store_cow.go:528 rememberOldImageLocked (commitDecentralized, s.mu.RLock + historyMu) | one fully built image per object, published once | no | reserved ts; old version < ts |

The unstamped double republish needs a self-cycle: Recycle of an object that is
its own child/parent/content/location, or ChangeParents(X, [..X..]). chparent
rejects self-parenting with E_RECMOVE (builtins/objects_hierarchy.go:154-173) and
move rejects recursive moves (builtins/objects_movement.go:81). I found no real
path to it, so there is no test for it. Whether a loaded database can contain
such a cycle is unverified.

## Alternative rule considered (lead, 2026-10-25)
"Skip a superseded image whose version is the in-flight commit's ts; otherwise
first-entry-wins." It needs every exclusive section to bump the clock before its
first republish. The table shows most direct mutators bump after it
(setObjectName/Owner/LocationRaw/Flag, Recycle, Recreate, Renumber,
setPropertyInfo/Value, clearPropertyOverride, define/delete with ts==0, every
verb mutator). In those sections the clock still holds the previous commit's ts
at republish time, so "version == clock" also matches the previous commit's
final image and would drop it. Adopting it means moving the bump in about 20
sections. The replace rule needs no reordering and the audit finds no real path
that breaks it, so it stays.

## Surprises
- Same-ts also reaches the decentralized path: a coarse commit leaves the newest
  entry at T, and a later decentralized commit superseding the final T image hit
  the same skip. Also Renumber (store_lifecycle.go) republishes an object that has
  oldID in both parents and children twice at one ts.

## Next step
Lead pushes/opens the PR. Wave 3b replaces the coarse applier later.
