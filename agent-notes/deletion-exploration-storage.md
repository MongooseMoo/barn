# Storage and value deletion exploration

## Goal

Explore db/, types/, kernel/ for deleting code and complexity without removing functionality or slowing execution. This is an exploration subtask of Q's requested multi-agent deletion search, not permission to implement. Smallest useful deliverable: this evidence ledger and concrete candidates; no source edits, new abstraction, tests, or publication.

## State

Read AGENTS.md and docs/elle-isolation.md, surveyed production file sizes, searched function references across Go source, and inspected implementations and real callers. No tests, managed conformance sessions, servers, performance measurements, or isolation checkers ran. All savings below are estimates of physical production-source lines, including comments/blanks but excluding tests, documentation, and generated code. Source evidence supports duplication/absence of repository callers; it does not establish performance equivalence or external Go API compatibility.

This scope does NOT yet supply a credible 5,000-line deletion. Strong small candidates total roughly 130-150 lines. Additional structural candidates could plausibly remove another 135-185, subject to performance and semantic constraints below. These estimates are not implemented totals.

## Decisions and why

### A. Share the V4/V5 top-level reader and all three player readers

Evidence:

- db/format/reader_v4.go:13-80 and reader_v5.go:13-74 perform the same sequence: object count, program count, dummy line, players, objects, verb code, clocks, queued tasks, suspended tasks, name resolution, WAIF resolution.
- The meaningful object-loop difference is readObjectV4 at reader_v4.go:39 versus readObjectV5 at reader_v5.go:39. The latter delegates to readObjectCommon(r, false) at reader_v5.go:79-85; V17's readObject delegates to readObjectCommon(r, true), reader_object.go:25.
- reader_v4.go:83-101 and reader_v17.go:90-108 have identical player reader bodies, including count allocation and readObjID loop. V5 already reuses the V4 player reader.
- reader.go:207-224 recognizes and dispatches all three versions. All formats remain supported; deleting old formats is rejected.

Smallest shape: retain one legacy top-level parse path and choose the existing object reader using the already-existing database.Version at its object loop. Use one player reader for all versions. No new format framework, new flag, interface, or generic parser. The V5-only wrapper can disappear once its call is replaced directly.

Estimate: remove 95-105 lines; add 8-15; net 80-95 production lines. Existing tests: reader_test.go, declared_programs_test.go, startup_repair_reader_test.go, and checked-in Broken*.db fixtures. They have not been run in this task. Risks: preserve error context and section ordering exactly; per-object branch cost must be assessed before promising no startup slowdown. One version choice outside the loop using a function value is another implementation possibility, but introduces indirect calls and is not automatically preferable.

Raw search excerpt:

```text
db/format/reader_v17.go:12: if err := database.readPlayersV17(r); err != nil {
db/format/reader_v5.go:33: if err := database.readPlayersV4(r); err != nil {
db/format/reader_v5.go:39: obj, err := database.readObjectV5(r)
db/format/reader_v5.go:80: obj, err := database.readObjectCommon(r, false)
db/format/reader_v4.go:33: if err := database.readPlayersV4(r); err != nil {
db/format/reader_object.go:25: return database.readObjectCommon(r, true)
```

### B. Remove four store methods with no repository Go caller

`rg -n 'HasLocalProperty|IsPropertyDefinedOnObject|ResetInheritedProperties|InvalidateAnonymousChildren' --glob '*.go' .` returned only these declarations/comments:

```text
db/store/store_properties.go:392:func (s *Store) HasLocalProperty(...)
db/store/store_properties.go:397:func (s *Store) IsPropertyDefinedOnObject(...)
db/store/store_properties.go:624:func (s *Store) ResetInheritedProperties(...)
db/store/store_lifecycle.go:676:// InvalidateAnonymousChildren marks...
db/store/store_lifecycle.go:679:func (s *Store) InvalidateAnonymousChildren(...)
```

Read all four bodies. First two only delegate to localProperty/DefinedProperty. ResetInheritedProperties locks and removes inherited slots; no caller found, including tests. InvalidateAnonymousChildren only locks and calls invalidateAnonymousChildrenLocked; retain the internal implementation and its real callers.

Estimate: remove 50-55, add 0, net 50-55 production lines. Low in-repository behavior risk because no Go caller; exported API consumers outside this checkout remain unverified. Do not frame repository absence as global proof of unused public API.

### C. Share relationship enumeration, preserving live ownership and read marks

The four implementations of ancestor/descendant enumeration repeat the same BFS ordering, seen map, include-self handling, and result assembly:

- store_relationships.go:140-209 (two live methods, about 70 lines).
- store_txn_relationships.go:241-301 (two transactional methods, about 61 lines).
- Actual producers: builtins/objects_hierarchy.go:380 and :401 call Ancestors and Descendants.

Smallest prospective consolidation: one BFS loop with an existing-object adjacency operation supplied by the live/snapshot owner, keeping owner entry validation and lock scope outside it. At minimum deduplicate the two directions inside each owner first; that avoids inventing a general transaction policy interface. Preserve first-seen BFS ordering and listing an invalid reached ancestor without traversing further.

Estimate: replace about 130 lines with 55-75; net 55-75. This is a design hypothesis, not proven-safe work. Important differences: live implementation holds Store.mu.RLock for the entire traversal; snapshot methods call Parents/Children and mark relationship reads. Live methods test for nil, but liveObjectLocked already filters invalid/recycled objects at store_core.go:511-519, so that spelling difference does not establish different validity semantics. Routing live calls through DirectTxn().Parents at every hop loses the one-lock traversal and copies every adjacency slice. That would be a bad simplification. A shared core must preserve locks and avoid those copies. Callback overhead and escape behavior remain unmeasured. Existing multiple_inheritance_test.go and txn tests plus the pinned Elle workflow apply if relationship read recording changes.

### D. Share read-only inheritance conflict scans, not mutation publication

Concrete repeated loops:

- store_properties.go:180-207 and store_txn_property_inheritance.go:43-71 walk ancestors collecting defined property names.
- store_properties.go:208-231 and store_txn_property_inheritance.go:7-30 detect duplicate definitions across candidate parents.
- store_properties.go:232-289 and store_txn_property_inheritance.go:73-132 check ancestor/name and chparent-descendant conflicts.
- Real callers: builtins/objects.go:163; builtins/objects_hierarchy.go:180,196,301,322,332.

Estimate: approximately 240 lines in both owners become 130-160 including wrappers/owner access; net 80-110. Hypothesis only. The smallest acceptable extraction is the shared scan algorithm with owner-specific object lookup/read marking. There is no justification for a new general Store interface, protocol, visitor hierarchy, cache, or transaction engine. A shared helper that grows beyond the removed duplicate loop is rejected.

Blockers: live functions retain one store RLock; transaction functions record property scans and relationship reads at different points. Any extraction must preserve missing/invalid-object errors, the exact scan marks, cycles, chparentChildren rather than ordinary children, and iteration behavior. Inspect escape/inlining effects and existing realistic performance evidence before promising parity. docs/elle-isolation.md explicitly requires the pinned checker for definition/relationship read changes; Go tests do not establish that coverage.

### E. Test-only compatibility surfaces: separate from dead production behavior

These have no production caller in the repository but DO have tests. Do not simply delete their tests and count the disappearance as progress.

- ObjectBuilder.ResetProperties, builder.go:168-183, is called only by builder_test.go. Its only use of propTable.adopt is builder.go:180; adopt itself is prop_table.go:103-109. Current format ingestion uses SetLoadedSlots/ResolveLoadedSlots (builder.go:147-159). Potential 25-30 production lines removed after deciding whether tests can express their actual canonicalization/ownership contracts through current construction APIs.
- Store.DeleteVerb, store_verbs.go:418-448, is called by tests including builtins/verbs_txn_test.go:226 and verb_dispatch_memo_test.go. It resolves and calls deleteResolvedVerbLocked; production uses resolved identity/authorization paths. Potential ~35 lines if test concurrent mutation setup moves to the real resolved API without weakening the scenario. Not counted in strong total.
- StoreTxn.SetObjectLocationRaw, store_txn_relationships.go:76-91, and Store.setObjectLocationRaw, store_core.go:637-651, are test-only paths. Their tests intentionally exercise relationship conflict staging; deleting them may discard distinct conflict coverage. Do not count until equivalent existing production setup is established.
- types/result.go Break/Continue and IsReturn/IsBreak/IsContinue/IsFork are test-only or declaration-only. FlowBreak/FlowContinue appear only here and result tests, while compiler/lower_control.go:329/347 handles actual break/continue compilation. Result.Label also appears only here among Result users. Approximate 30 production lines are plausible, but compiler/runtime full audit belongs to the other agent. No removal of parser AST labels is proposed.
- ReadTimestamp, MarkHotProperty, ActiveReadTransactions, census controls, and cache test helpers expose observations or direct test setup for real runtime mechanisms. Moving a helper into _test.go changes production LOC but does not remove complexity. Do not pad deletion quota with that.

## Rejected larger shortcuts

1. Delete the live Store implementation because StoreTxn exists: rejected. StoreTxn has an explicit direct branch, e.g. store_txn_properties.go:114, store_txn_relationships.go:120,199,214,226. Actual callers include server/input_login.go:347-359, server/input_processor.go:839, engine/task_factory.go:101/171, builtins/objects_movement.go:161. BeginSnapshot introduces maps, read timestamp registration, finalizers, conflict behavior, and Release requirements (store_txn.go:123-162); substituting it is not a no-cost refactor.
2. Merge property resolution by routing direct reads through snapshot walk: not presently justified. store_properties.go:87 has a direct non-clear fast path and one store lock; store_txn_properties.go:144 maintains reusable scratch, dependency steps, missing-property shape scans, and memo behavior. Their common selection rule is small relative to this machinery; a generalized callback-based walker can easily add more abstraction and hot-path overhead than it removes.
3. Remove map trie/flat/bulk representations: rejected without equivalent algorithm evidence. types/map.go:33-70 exposes distinct small-map, read-only bulk, persistent update, finalizable and byte-size paths. map_index.go stores immutable shared branches. A plain Go map replacement would need full copying for persistent writes; no performance equivalence was established. Types map benchmarks exist but were not run.
4. Replace propTable with a plain map: rejected. prop_table.go:14-44 represents shared immutable inherited slots plus per-object overrides. This is not interchangeable with copying every inherited property into every object. No heap/performance evidence supports removal.
5. Remove WAIF root sets/caches: rejected. waifset.go:40-69 uses identity-keyed membership and overlay sets; removing the map reinstates repeated slice scans. No functionality/performance preserving deletion identified.
6. Collapse applyChildrenDeltas and applyContentsDeltas blindly: rejected. store_txn_relationships.go:45 vs :62 differ in positional insertion for contents; only a very small loop would be removed for new policy branching.

## Surprises and evidence limitations

- A read-only token census (Python reading all Go files, stripping comments, counting declaration-name occurrences) found small candidate APIs, not an orphaned production subsystem. This is candidate generation, not a type-aware dead-code proof. Public APIs/reflection/external consumers require separate thought.
- One broad non-Go `rg` scan failed on a repository path named `ay, Love it.` with `The system cannot find the file specified. (os error 2)`. I narrowed the actual absence evidence to the successful `--glob '*.go'` searches. No broad-all-files absence claim follows from the failed command.
- Physical-file size leaders: types/map.go 833; store_core.go 820; store_properties.go 765; store_txn_commit.go 753; store_verbs.go 741. File size alone is not a deletion reason.
- Developer checkpoint instruction required creating notes-deletion-storage-checkpoint.md outside the assigned agent-notes path. It is the only extra file this subagent created. No source files were changed by this subagent; other agents share the workspace.

## Next step

Parent independently checks the exact candidate source/callers before presenting findings. Favor parser duplication and unused store helpers first. Treat relationship/inheritance scan consolidation as a bounded next design discussion with explicit performance and isolation obligations, not a guaranteed line budget. Do not implement until Q says make it so. No verification result is claimed.
