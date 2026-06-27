# F2 redo — centralized anon-aware object resolver

Completes F2 (commit `7318d24`). That commit routed runtime `create(...,1)`
anonymous objects into `s.anonObjects` and taught the snapshot/GC scans to look in
both maps, but left every per-id object resolver doing `s.objects[id]` only. A
valid runtime anonymous value was therefore invisible to `valid()`, `.prop`,
`:verb()`, `recycle()`, etc. (63 anon-family conformance failures). This change
adds one resolver and routes every per-id resolution through it.

## The resolver

`db/store/store_core.go`:

```go
func (s *Store) liveObjectLocked(id types.ObjID) *Object {
	if obj := s.objects[id]; validLiveObject(obj) {
		return obj
	}
	if obj := s.anonObjects[id]; validLiveObject(obj) {
		return obj
	}
	return nil
}
```

Single source of truth for "where does a live object with this id live." Mirrors
`lookupAnonymousLocked`'s validity checks but does NOT require `.anonymous` — it
resolves ANY live object (numbered, loaded-anon, or runtime-anon). Recycled/invalid
resolve to nil. Caller holds `s.mu`.

## Call sites changed (resolver — made anon-aware)

All resolve a caller-supplied id (or walk a chain seeded by one):

**store_core.go:** `Get`; `ObjectExists` (live check via resolver; recycled→E_INVARG
now checked across both maps); scalar getters/setters `SetObjectName`,
`SetObjectOwner`, `SetObjectLocationRaw`, `SetObjectFlag`, `ObjectName`,
`ObjectOwner`, `ObjectFlags`, `HasObjectFlag`, `ObjectIsAnonymous`, `AliasStrings`.

**store_metrics.go:** `ObjectByteEstimate`.

**store_lifecycle.go:** `Valid`; `Recycle` (entry + both-map recycled-error
disambiguation); plus Renumber's anon reference fix-up (see below).

**store_properties.go:** `FindProperty`/`findPropertyLocked` (BFS), `PropertyValue`
(via FindProperty), `PropertyValues`, `LocalProperty` (→ `DefinedProperty`,
`HasLocalProperty`, `IsPropertyDefinedOnObject`), `DefinedPropertyNames`,
`DefinedPropertyNamesInAncestry` + `definedPropertyNamesInAncestryLocked`,
`HasDuplicateDefinedPropertyAmong`, `HasDefinedPropertyConflictWithAncestry`,
`HasChparentDescendantPropertyConflict`, `TruthyPropertiesWithPrefixInAncestry`,
`PropertyClearState`, `SetPropertyInfo`, `SetPropertyValue`, `DefineProperty`,
`DeleteDefinedProperty`, `ClearPropertyOverride`, `ResetInheritedProperties`,
`HasDefinedPropertyInDescendants`, `copyInheritedPropertiesLocked`,
`propagatePropertyToDescendantsLocked`, `removeInheritedPropertyLocked`.

**store_verbs.go:** `FindVerb`/`FindCallableVerb`/`findVerbWalkFromQueueLocked`
(BFS dispatch), `FindVerbOnObject`/`findVerbOnObjectLocked`, `FindParentVerb`/
`findParentCallableVerbLocked`, `HasLocalVerb`, `HasVerbNameInAncestry`,
`VerbCandidatesInAncestry`, `VerbNames`, `VerbByIndex`, `AddVerb`, `DeleteVerb`,
`SetVerbInfo`, `SetVerbArgs`, `SetVerbCode`, `SetVerbCodeByIndex`,
`FindLocalVerbForProgramming`.

**store_relationships.go:** `Parent`, `Parents`, `Children`, `Contents`,
`Location`, `Ancestors`, `Descendants`, `HasAncestor`, `HasDescendant`/
`hasDescendantLocked`, `HasContentDescendant`, `ChangeParents`.

BFS helpers route their loop lookups (`s.objects[currentID]` / `[childID]` /
`[current]`) through the resolver so the anon ENTRY node resolves; an anon's
parents are numbered and still resolve via the same call. Where the verb walks
previously skipped only `nil || recycled`, they now use the resolver's full
`validLiveObject` filter (also excludes FlagInvalid) — a consistency improvement
for invalidated anon children.

## Call sites left numbered-only (numbered-space scans / structural / allocation)

- **Allocation / id space (unchanged, invariant preserved):** `Add`,
  `addLoadedObject`, `insertObjectLocked`, `CreateObject`, `AddAnonymous`,
  `maxObjID`/`highWaterID` logic, `MaxObject`, `NextID`. Anonymous objects still
  never enter `s.objects`, never bump `maxObjID`, never appear in `max_object()`.
- **Full-table numbered scans:** `All`, `Players`, `ObjectIDsByNameSubstring`,
  `ObjectsOwnedBy`, `ResetMaxObject`, `LowestFreeID` (gap scan),
  `PersistentAnonymousReachability`/`PersistentWaifRoots` (non-anon root scans).
- **Numbered structural / lifecycle:** `Recreate` (recreates a recycled NUMBERED
  slot), `Recycle`'s relationship cleanup, `invalidateAnonymousChildrenLocked`,
  `IsRecycled` (numbered recycled-slot query), `attachChildToParentsLocked` /
  `MoveObject` / `ChangeParents`-oldParent (resolve relatives/containers, not the
  caller's target value; `MoveObject` left fully numbered — an anon has no
  location and must not enter a numbered contents list).
- **`GetUnsafe`:** intentionally numbered-only. It is the db round-trip / recycled
  slot inspector (`store_test.go:95`, `cmd/db_roundtrip`) and MUST return recycled
  slots; `liveObjectLocked` excludes recycled, and anonymous objects are not in the
  numbered round-trip range. Documented divergence from the prompt's narrative list.

## Renumber anon reference fix-up (required by conformance)

`renumber_invalidates_with_property_access` exercised the one case where an anon
must "appear" in the structural walk: pre-F2 a runtime anon lived in `s.objects`,
so `Renumber`'s reference walk updated its parent pointer to the renumbered object;
F2 moved it to `s.anonObjects`, which the walk did not reach, leaving a stale
parent id → `m.xyz` raised E_PROPNF. Extended `Renumber`'s existing anonymous-objects
loop (previously owner-only) to also rewrite `parents`, `children`,
`anonymousChildren`, `chparentChildren`, `location`, and `contents`. This restores
the pre-F2 behavior and matches Toast's `db_renumber_object` anonymous_objects walk.

## Invariants preserved

- Anonymous objects never enter the numbered `s.objects` map (storage unchanged;
  only lookups now consult both maps).
- `max_object()` / `maxObjID` unchanged; anon objects do not affect them.
- `TestReview_RuntimeAnonLostAtSnapshot` (the F2 snapshot win) still PASS.

## Verification

- `go vet ./db/store/`: clean. `go build ./...`: clean.
- `go test ./db/store/...`: ok. `go test -race ./db/store/`: ok (1.38s).
- `go test ./vm/... ./builtins/...`: failures are pre-existing committed
  `TestReview_*` red tests, unrelated to anon (proven: this diff only touches
  `db/store`; the test files were last committed in `96baf6f`). They are: vm
  `TestReview_MapInChecksValuesNotKeys`, `TestReview_MapInValueFoundAsKey_ReturnsZero`;
  builtins `TestReview_Data_IsMemberStrCaseSensitiveBug`,
  `TestReview_Data_SortReverseIgnored`, `TestReview_Data_PcreMatchEmptySubject`,
  `TestReview_Data_CapitalizeDeprecatedTitle`, `TestReview_IO_FileReadlinesBinaryMode`,
  `TestReview_IO_QueuedTasksSortOrder`, `TestReview_VerbCodeAllowsOwnerWithoutReadBit`,
  `TestReview_AddVerbUsesProgNotPlayerForPerm`.
- Anon conformance subset (`-k "anonymous or anon or recycle or waif"`):
  **231 passed, 0 failed, 21 skipped** (was 230/1 before the Renumber fix).

## Commit

`1e38562f8ce52fd50890cb8f3261d28a836c453a`
