# Item 1, Phase 3 (Object) — coder report

## RESULT SUMMARY (all gates green; NOT merged — verifier next)
- Commit: `9ee4e30` on branch feat/item1-object (worktree C:/Users/Q/code/barn-item1-object).
- `go build ./...` EXIT 0.
- `go vet ./...` only the 2 known findings (cmd/moo_client IPv6, vm/stack ReadByte).
- `go test ./...` all pass EXCEPT `barn/conformance` 3 loader unit tests
  (TestConformance/TestLoadAllTests/TestYAMLParsing) which fail with "could not find conformance
  test directory (../cow_py/tests/conformance)" — ENVIRONMENTAL (missing cow_py checkout), not my
  change. TestLoadMongooseSnapshot passes (snapshot db copied to worktree ROOT).
- `go list -deps ./db/store | grep parser` → EMPTY.
- db_roundtrip GREEN. Command:
  `go build -o db_roundtrip.exe ./cmd/db_roundtrip/ && ./db_roundtrip.exe -db C:/Users/Q/code/barn/toastcore.db -out _roundtrip_out.db`
  → "Loaded: maxObj=#127, players=6, objects=127" ... "SUCCESS: Round-trip test passed!" EXIT 0.
- Conformance EXACTLY **3871 passed, 0 failed, 131 skipped** (148.64s), run synchronously in
  foreground via `uv run --project ../moo-conformance-tests moo-conformance --server-command
  "...barn.exe -db {db} -port {port}"`.
- Seal probe (throwaway pkg, then removed): `store.Object{name:..}` keyed literal, `o.name=`,
  `o.properties[..]=` all fail to compile; `s.Get(...)` returns (ObjectView, bool) not *Object;
  `s.All()` returns []ObjectView not []*Object. Confirmed.

## DESIGN CHOSEN (the fork)
ObjectBuilder in db/store is the loader's construct+relink API; startup_repair STAYS in db/format
operating on the builder graph (Database.Objects became map[ObjID]*store.ObjectBuilder). This is
correct because repair + inherited-name resolution run ENTIRELY PRE-INGEST (NewStoreFromDatabase
ingests b.Build() afterward) — no live store object is ever relinked, so no store-relink setters
and no relocation into db/store were needed. The builder exposes getters AND setters/appends for
relations (cross-object repair reads neighbours) plus AppendVerb/SetVerbCodeByIndex/SetProperty/
ResetProperties. ADDITIONAL consumer not in the brief: the database WRITER reads object fields
from store.Snapshot; I converted Snapshot to hold a new read-only value type SnapshotObject
(VerbList []VerbView, Properties map[string]PropertyView) so the writer never holds a live *Object.

---

# (work log below)


Worktree: C:/Users/Q/code/barn-item1-object, branch feat/item1-object off 8371063.
mongoose7_snapshot.db copied into db/format/.

## Baseline (before changes)
- go build ./... exit 0
- go vet ./... : 2 known findings (cmd/moo_client/main.go:53 IPv6, vm/stack.go:49 ReadByte) — exit 1
- HEAD 8371063 = "Encapsulate Verb behind a value View (item 1, phase 2)"

## Architecture findings (drives design)
- **Object fields currently exported** (object.go:10-38). Property & Verb already sealed in phases 1/2.
- **Loader (db/format) build model is `Database.Objects map[ObjID]*store.Object`** (reader.go:16).
  The loader reads each object into that map (reader_object.go/reader_v4.go/v5/v17), runs
  `repairStartupIssues()` (startup_repair.go) ON THAT MAP, resolves inherited property names
  (reader_helpers.go resolvePropertyNames rewrites Properties map + PropOrder), THEN ingests via
  `NewStoreFromDatabase()` -> `s.Add(obj)` (reader.go:37-45).
  => startup_repair mutates PRE-INGEST build objects, NOT live store objects. This is the key fact:
  the relink happens entirely in db/format before any store ingest.
- **Writer (db/format/writer_object.go) reads `*store.Object` fields directly** from a
  `store.Snapshot` (store_snapshot.go). Snapshot currently holds `*Object` (cloned). This is an
  ADDITIONAL external consumer beyond loader/repair/fixture — the prompt did not list it but it
  must be migrated too (reads ID/Name/Flags/Owner/Location/Contents/Parents/Children/VerbList/
  PropDefsCount/Properties/Recycled/Anonymous).
- **conformance/setup.go fixture** writes `obj.Properties[name]=&p` and mutates `obj.Flags`; uses
  store.Get(...) live object. Store has DefineProperty + SetObjectFlag already.
- **cmd tools**: cmd/db_roundtrip (GetUnsafe/All), check_player, dump_prop, dump_verb, barn/main.go.
- Internal store field refs: ~293 (will rename mechanically).

## DESIGN DECISION (chosen)
Two value types + a builder, mirroring PropertyView/VerbView idiom:
1. `ObjectView` value snapshot (scalars) for external scalar reads.
2. Unexport all Object fields; add `NewObjectBuilder(id, owner)` in db/store that db/format uses to
   construct AND relink during load. Because startup_repair/resolvePropertyNames cross-read other
   objects' relational fields and props, the builder must expose getters+setters for relations,
   plus AddVerb/AddProperty/SetProps. `Build() *Object` returns the object the store ingests.
   The loader's `Database.Objects` becomes `map[ObjID]*ObjectBuilder` (mutable build model); ingest
   calls `s.addBuiltObject(b)` or `s.Add(b.Build())`.
3. startup_repair: stays in db/format, operating on the builder map (cleanest — it already runs
   pre-ingest on db/format's own objects; no store involvement needed). No store-relink setters
   required. THIS IS THE FORK RESOLUTION: builder, not store-relink, because repair is pre-ingest.
4. Writer: Snapshot exposes ObjectView-style read access (or keep internal *Object but add exported
   read accessors the writer uses). Leaning: give Snapshot a method returning a writer-friendly
   view. TBD detail.
5. Get/GetUnsafe/All -> ObjectView; migrate cmd/db_roundtrip.

## STATUS (update 1): implementing
- object.go: Object fields unexported; added ObjectView + (*Object).view().
- builder.go NEW: ObjectBuilder (get/set/append relations, AppendVerb, SetVerbCodeByIndex,
  SetProperty/Property/ResetProperties, Build() *Object). This is the loader's construct+relink API.
- store_snapshot.go: Snapshot now holds *SnapshotObject (NEW value type w/ VerbList []VerbView,
  Properties map[string]PropertyView) instead of *Object — writer no longer holds live *Object.
  snapshotObjectValue() builds it; PropertyNames computed over live s.objects.

## FORK RESOLUTION (decided, NOT a blocker)
startup_repair stays in db/format operating on its OWN build graph. Loader's Database.Objects
becomes map[ObjID]*store.ObjectBuilder. Repair/resolution use builder getters/setters/appends.
No store-relink setters needed because repair is entirely PRE-INGEST (NewStoreFromDatabase ingests
after repair). This is the cleanest of the three options the prompt named.

## TODO
- Remove now-dead cloneObjectForSnapshot/cloneVerbForSnapshot if unused.
- Rename ~293 internal store field refs obj.Field -> obj.field across store_*.go (careful: Store
  has PUBLIC methods Parents/Children/Contents/Location/Owner/Name/Flags — only rewrite obj.FIELD
  on *Object locals, NOT s.Method()). store_reachability.go has val.ID() method call collision.
- Migrate db/format readers (reader_object/v4/v5/v17/helpers/startup_repair/writer) to builder/view.
- Migrate conformance/setup.go to DefineProperty/SetObjectFlag + builder for anon.
- Migrate cmd tools (db_roundtrip key gate, check_player, dump_prop, dump_verb, barn/main).
- Migrate external _test.go literals (vm, server, db/format).
- Get/GetUnsafe/All -> ObjectView. GetUnsafe -> (ObjectView,bool).

## STATUS (update 2): store package builds clean.
- All Object fields renamed to lowercase across store_core/lifecycle/metrics/properties/
  reachability/relationships/verbs via gofmt -r (per field; .ID handled manually in reachability
  due to val.ID() method collision).
- Deleted dead cloneObjectForSnapshot/cloneVerbForSnapshot (Snapshot builds SnapshotObject now).
- Confirmed Get/GetUnsafe/All have no internal store callers; GetAnonymousObjects already absent.
- NEXT: convert Get/GetUnsafe/All return types; migrate db/format, conformance, cmd, tests.

## STATUS (update 3): store done, loader in progress
- Get/GetUnsafe -> (ObjectView, bool); All -> []ObjectView. Store builds clean.
- reader.go: Database.Objects now map[ObjID]*store.ObjectBuilder; NewStoreFromDatabase ingests
  b.Build(). reader_object.go fully migrated to builder methods (Set*/Append*/AppendVerb/
  SetVerbCodeByIndex/SetProperty/SetPropOrder; .ID() getter).
- REMAINING db/format: reader_v4.go, reader_v5.go, reader_v17.go, reader_helpers.go
  (resolvePropertyNames/rawPropertyNames/propertyNamesSelfFirst/finalPropertyOrder/
  collectWaifPropNames take *store.Object -> must take *store.ObjectBuilder + use getters/
  ResetProperties), startup_repair.go (all on builders), writer_object.go + writer.go (consume
  SnapshotObject value).
- THEN: conformance/setup.go, cmd/*, external _test.go, seal probe, gates.

## STATUS (update 4): loader + writer + db/format build clean; production builds clean
- db/format fully migrated (readers v4/v5/v17/common, helpers, startup_repair on builders; writer
  consumes SnapshotObject). go build ./db/format and ./db/store green.
- go build ./... : ONLY cmd/* + conformance break (confirms scout: vm/server/builtins/kernel never
  read raw *Object). 
- conformance/setup.go migrated to store.Get(ok)/LocalProperty/DefineProperty/SetObjectFlag +
  ObjectBuilder for anon creation.
  RISK NOTED: original fixture wrote #0.Properties map directly (no descendant propagation);
  DefineProperty propagates clear slots to descendants (more MOO-correct). Conformance gate
  3871/0/131 will confirm no regression; revisit if counts shift.
- db_roundtrip migrated to GetUnsafe (ObjectView,bool) + VerbCount/PropertyCount.
- REMAINING cmd: check_player, dump_prop, dump_verb, cmd/barn/main.go (9 Get sites), then tests.

## STATUS (update 5): ALL non-test code builds clean (go build ./... EXIT 0)
- All cmd tools migrated (check_player, dump_prop, dump_verb, cmd/barn all Get sites ->
  ObjectView + store accessors; verb listing via VerbByIndex/VerbNames; props via
  DefinedPropertyNames/LocalProperty).
- REMAINING: test files only - db/store/store_test.go (Get), db/format/dump_persistence_test.go
  (Get), vm/anonymous_gc_test.go (store.Object{} literals), server/scheduler_login_test.go
  (obj.Name etc + obj.Verbs map), vm/bytecode_execution_test.go, db/format/reader_test.go.
- THEN: seal probe, gates (build/vet/test, db_roundtrip, go list deps, conformance 3871/0/131).

## STATUS (update 6): test migration ~80% done
- Added builder getters Name(), VerbNamesAt(i) for test inspection.
- Migrated: db/store/store_test.go, db/format/dump_persistence_test.go + reader_test.go +
  startup_repair_reader_test.go (all vet clean), vm/anonymous_gc_test.go (builder + DefineProperty).
- REMAINING tests: vm/bytecode_execution_test.go (Verb{}+root.Verbs[k]=), vm/dump_persistence_test.go
  (Get + Properties), server/scheduler_login_test.go (obj.Name/Flags/VerbList + obj.Verbs[k]=).
- Behavioral note still pending gate: conformance/setup.go DefineProperty propagation.

## STATUS (update 7): ALL code+tests compile. go vet ./... = only 2 known findings.
- All test files migrated (vm, server, db/store, db/format). Builder getters used for inspection.
- NEXT: run go test ./..., go list deps, db_roundtrip, seal probe, conformance gate.

## STATUS (update 8): GATES passing so far
- go build ./... EXIT 0. go list -deps ./db/store | grep parser = EMPTY.
- go test ./...: all pass EXCEPT barn/conformance (3 tests: TestConformance/TestLoadAllTests/
  TestYAMLParsing) which fail with "could not find conformance test directory
  (../cow_py/tests/conformance)" - ENVIRONMENTAL (missing cow_py dir), not my change.
  TestLoadMongooseSnapshot now PASSES after copying mongoose7_snapshot.db to worktree ROOT
  (test looks at ../../ from db/format, not db/format/).
- NEXT: db_roundtrip gate, seal probe, conformance 3871/0/131.

