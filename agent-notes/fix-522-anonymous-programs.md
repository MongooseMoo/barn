# #522: anonymous verb programs on reload

## Goal
Attach saved anonymous verb programs without placing anonymous builders in the numbered Objects map.

## State
Base origin/master verified as c537d6c952791b2fe49bcb9d7cba90102767a36f. Implemented 14 added production lines across reader.go and reader_object.go; extended existing persistence test by 31 lines. Local-only delivery; parent owns publication and PR CI.

## Decisions and why
Smallest repair: add a private parse-lifetime anonymous builder index, fill it while reading anonymous objects, consult it only when a verb reference has no numbered builder, and release it alongside existing parse scratch state. About 14–20 production lines across reader.go and reader_object.go. Existing AnonymousObjs keeps ownership/order; no numbered map changes. This adds O(anonymous objects) indexing once and O(1) expected lookup per anonymous program rather than a scan per program.

Barn assigns anonymous serialization IDs above maxObjectID (store_snapshot.go:249-258). Recycled object records return nil and never enter Objects (reader_object.go:55-59, reader_v17.go:53-56), so fallback also resolves an anonymous builder whose ID matches a recycled slot. Existing numbered-object precedence remains unchanged for malformed collisions with live numbered builders.

Extend the existing TestRoundTripPreservesAnonymousInheritedOverride fixture with absent, source-bearing and empty-present verbs, checking their original indexes after normal writer/parser/store reload. This uses the existing rooted anonymous producer; no native server launch or conformance semantic change.

## Surprises
No existing anonymous builder ID index exists in the reader.

## Next step
Parent reviews the scoped commit, publishes the PR and reads CI. No full suite was run locally. Coordinate reader.go merges with #525/#526; this change only adds loadedAnonymous next to loadedSlots and clears it beside other parser scratch state.

## Focused verification
Command: `go test ./db/format -run '^TestRoundTripPreservesAnonymousInheritedOverride$' -count=1`

Before repair, the extended existing test reproduced the real producer's source loss:
```text
--- FAIL: TestRoundTripPreservesAnonymousInheritedOverride (0.03s)
    dump_persistence_test.go:221: anonymous verb index 1: HasProgram=false Code=[]
FAIL github.com/MongooseMoo/barn/db/format 1.325s
```

After repair:
```text
ok github.com/MongooseMoo/barn/db/format 1.308s
```

The test also retains its inherited-property assertion and verifies that anonymous builders do not enter numbered Objects. The source/HasProgram checks run after conversion into a Store and a fresh snapshot. Load throughput was not benchmarked; the implementation adds one map entry per anonymous object and one fallback map lookup per reference absent from Objects, without repeated slice scans. No new historical/native compatibility conclusion is claimed; malformed live-numbered/anonymous ID ambiguity remains outside this scoped repair.

## PR draft
Title: fix(db/format): restore saved programs on anonymous objects

Barn emits programs for anonymous objects, but the reader previously attached programs only to numbered builders, silently losing anonymous source and empty-program state on reload. Index anonymous builders during parsing and resolve missing numbered references through that index. Keep anonymous ownership separate and release the index after parsing.

Extended the existing rooted-anonymous persistence test to check original indexes, nonempty and empty-present programs, and a never-programmed verb through writer/parser/store reload. The focused test reproduced source loss before the change and passes after it. PR CI remains pending.

Closes #522.
