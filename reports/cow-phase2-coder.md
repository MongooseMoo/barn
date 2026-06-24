# COW Phase 2 — coder working notes (IN PROGRESS)

## Mission
Move property DEFINE (`propertyDefines`) and DEFINE-DELETE (`propertyDefinitionDeletes`)
commit-apply writes onto the decentralized COW path in `commitDecentralized`.

## Build rule
ONLY `make build` -> `barn.exe`. No other server binary name (firewall modal hangs run).

## KEY FINDINGS SO FAR

### Footprint (Q1) — the staging already decomposes the subtree
- The **txn staging side already enumerates the full define/delete footprint**:
  - `StoreTxn.DefineProperty` (store_txn.go:1188) stages `propertyDefines[key]=prop`
    for the DEFINER only, then `propagateDefinedProperty` (1219) WALKS DESCENDANTS via
    `current.children` and stages a `propertyWrites` entry (a clear inherited slot) for
    EACH inheriting descendant.
  - `StoreTxn.DeleteDefinedProperty` (1319) stages `propertyDefinitionDeletes[key]` for
    definer, then `removeInheritedProperty` (1353) walks descendants via `current.children`
    and stages `propertyDeletes` for each inheriting descendant.
- So by the time we reach Commit, the footprint = {definer in propertyDefines/Deletes}
  ∪ {descendants already present in propertyWrites/propertyDeletes}. The descendant images
  are ALREADY on the decentralized path (Phase 1 handles propertyWrites/propertyDeletes).
- The COARSE commit path's `definePropertyLocked`/`deleteDefinedPropertyLocked` RE-WALK
  descendants (store_properties.go:516,550). This is REDUNDANT with the staged descendant
  writes but idempotent (writes the same clear slot / deletes same key). NEED TO VERIFY
  the decentralized path must apply define on the DEFINER's image only (the propOrder/
  propDefsCount/defined-slot bits the descendant propertyWrites don't carry), and let the
  already-staged descendant propertyWrites/propertyDeletes handle descendants.

### Anon objects (Q2) — RESOLVED: cannot propagate to anon
- `attachChildToParentsLocked` (store_relationships.go:5-23): anonymous children go into
  `parent.anonymousChildren`, NON-anon into `parent.children`.
- ALL propagation walkers (propagatePropertyToDescendantsLocked store_properties.go:628;
  removeInheritedPropertyLocked :671; txn propagateDefinedProperty :1219; removeInheritedProperty
  :1353) iterate `current.children` ONLY — never anonymousChildren.
- builtins/properties.go:343-344, 395-396: explicit note "Toast does NOT invalidate
  anonymous descendants when a parent's property schema changes."
- CONCLUSION: define/delete footprint NEVER includes anon objects. No anon slot work needed.

### Verb aliasing (Q3)
- buildImageWithVerbCode (store_cow.go:135) already preserves verbs-map/verbList identity.
- Define/delete don't touch verbs, so the definer image build only touches `properties`,
  `propOrder`, `propDefsCount`. Need a build helper that copies properties map + propOrder
  slice + propDefsCount, leaving verbs/verbList shared. Verb aliasing irrelevant for define.

## PLAN
1. Add `buildImageWithPropertyDefine(old, prop, ts)` and `buildImageWithPropertyDefinitionDelete(old, actualName, ts)`
   — DEFINER-ONLY image (copy properties map + propOrder + propDefsCount). Descendants handled
   by existing staged propertyWrites/propertyDeletes (Phase 1 builders).
2. Extend `commitDecentralized` footprint collection to include propertyDefines/Deletes objIDs,
   apply define/delete to the definer image in fixed kind order.
3. Remove the `len(tx.propertyDefines)==0 && len(propertyDefinitionDeletes)==0` guard from Commit routing.
4. Add stress test. Run all gates.

## OPEN VERIFICATION
- Confirm the coarse re-walk redundancy: does descendant propagation get applied by the
  staged propertyWrites alone, or do I need the definer image build to also re-walk? Believe
  staged writes are sufficient — must TEST (define-on-object-with-descendants conformance + unit).
- propOrder ordering on define: definePropertyLocked inserts at propDefsCount position.

## IMPLEMENTATION DONE (not yet built/tested)
- store_cow.go: added buildImageWithPropertyDefine + buildImageWithPropertyDefinitionDelete
  (DEFINER-ONLY image builds: copy properties map + propOrder slice + propDefsCount; share
  everything else). Mirror definePropertyLocked / deleteDefinedPropertyLocked exactly minus
  the descendant walk.
- commitDecentralized: footprint now collects propertyDefines + propertyDefinitionDeletes objIDs;
  added pre-build validation (define => not-already-exists E_INVARG; del => must be defined E_PROPNF);
  grouped defines/def-deletes by object; build loop applies defines then def-deletes then propWrites etc.
- Commit routing: removed `len(propertyDefines)==0 && len(propertyDefinitionDeletes)==0` guard.
  Now ONLY tx.liveMutated falls back to coarse store.mu.Lock.
- Updated commitDecentralized doc comment with full footprint proof + anon argument.

## FOOTPRINT PROOF (Q1) — COMPLETE
Footprint locked = definer (from propertyDefines/Deletes) ∪ all staged descendants (from
propertyWrites/propertyDeletes that the txn's propagateDefinedProperty/removeInheritedProperty
enumerated by BFS over `children`). Descendants whose inherited value does NOT change (those
with their own `defined` override) are correctly excluded (staging skips writing them; coarse
propagatePropertyToDescendantsLocked also skips them). Topology frozen: only coarse store.mu.Lock
mutators change children; committer holds RLock => excluded.

## NEXT
- go build ./... ; go vet ./db/store
- go test ./...
- add stress test (TestCOWConcurrentDefineDeleteSubtree)
- go test -race ./db/store ./vm ./scheduler
- make build; full managed conformance
- scaling test
## BUG FOUND + FIXED during testing
- TestTransactionDefinePropertyStagesAndPropagatesOnCommit failed: descendant clear slot
  came back defined=false clear=FALSE (want clear=TRUE).
- Root cause: buildImageWithPropertyValue else-branch (new slot) FORCED np.clear=false.
  Under coarse path this was fine because define created the clear=true slot first and the
  propertyWrites loop hit the EXISTING-slot if-branch. Under decentralized path the descendant
  has ONLY the staged propertyWrite (clear=true) and hits the else-branch -> lost clear flag.
- Fix: else-branch now HONORS w.prop.clear (removed forced np.clear=false). Safe for normal
  SetPropertyValue (stages clear=false) and correct for define-descendant (stages clear=true).
- After fix: go test ./db/store green; go test ./... green.

## GATE RESULTS SO FAR
- Gate 1 build/vet: PASS (go build ./... BUILD_OK; go vet ./db/store VET_OK)
- Gate 2 go test ./...: PASS (all packages ok)
- New stress test: TestCOWConcurrentDefineDeleteSubtreeRaceFree in
  db/store/cow_define_subtree_race_test.go (disjoint subtrees define<->delete on roots via
  decentralized COW, descendants assert inherited value present after define / E_PROPNF after
  delete; readers assert no torn inherited value; disjoint committers concurrent).

## GATE RESULTS (updated)
- Gate 1 build/vet: PASS
- Gate 2 go test ./...: PASS
- Gate 3 -race ./db/store ./vm ./scheduler -count=1: PASS (db/store 1.978s, vm 1.161s,
  scheduler 119.132s) — includes new TestCOWConcurrentDefineDeleteSubtreeRaceFree (also
  ran standalone -race: ok 1.756s).
- Gate 5 scaling TestConcurrencyCommitDominatedDisjoint -count=3: PASS (all 3 runs PASS;
  speedup@32 = 1.40x / 1.17x / 1.14x — noisy microbench, monotonic increase with workers
  holds; this path is property-value commits UNCHANGED by Phase 2 define/delete edits).
- make build: PASS -> barn.exe (only server binary present).

## BLOCKER: conformance runner
- `uv run --project ../../moo-conformance-tests moo-conformance ...` fails:
  "failed to remove file .venv/lib64: Access is denied (os error 5)" — uv venv sync issue,
  not a barn issue. Need alternate invocation (uv tool run, or pre-existing venv, or python -m).
## CONFORMANCE (Gate 4): 16 failed, 3971 passed, 131 skipped (144.91s) against make-built barn.exe
Failures categorized — NONE are define/inheritance/anon/property:
- 12x limits::* (named pre-existing category): setadd/listinsert/listappend/listset/appending/
  rangeset(list+map)/decode_binary/mapdelete max_value_bytes E_QUOTA checks.
- 1x gap_followups::audit_telnet_iac_delivered_as_oob_command (telnet IAC)
- 2x verb_dispatch::audit_non_executable_verb_shadow / pass_skips_non_executable (verb dispatch)
- 2x control_flow::ternary_concatenated_string_* (parser/control flow)
- 2x index_and_range::dollar_list_nested_* (index/range $)
=> all unrelated to property schema. Need to confirm pre-existing (compare vs baseline).

## ANALYST REVIEW (adversary pass) — items 1-5 CLEAN; FINDING B raised + RESOLVED:
- FINDING B (footprint hole if a child added concurrently after stage): RESOLVED.
  propagateDefinedProperty/removeInheritedProperty call markObjectRelationshipRead on EVERY
  walked node (store_txn.go:1233,1299,1367) recording relationshipVersion. CreateObject bumps
  the parent's relationshipVersion via stampObjectRelationship (store_lifecycle.go:37). At commit
  validateObjectRelationshipReadsLocked checks them -> a concurrent child-add fails validation
  (E_INVARG) -> retry re-walks and covers the new child. ALSO: decentralized path holds RLock,
  CreateObject holds Lock -> mutually exclusive anyway. No hole.
- FINDING A/C: cosmetic fragility (the validLiveObject loop at store_cow.go:378 runs before the
  define/delete property checks, so live is non-nil — safe). Adding a guard comment.

## CONFORMANCE PRE-EXISTING PROOF (rigorous)
- git stash blocked; instead backed up my 2 files to /tmp/cow2bak, `git checkout -- ` both to
  HEAD (baseline WITHOUT my change), `make build`, ran the 7 NON-limits failing tests:
  => 7 failed, 4111 deselected — IDENTICAL 7 failures fail on baseline. PROVEN pre-existing.
- The 9 limits::* failures match Phase 1 verify report exactly (pre-existing E_QUOTA value-bytes).
- Restored my files from /tmp/cow2bak, rebuilt. All 16 conformance failures = pre-existing.
  ZERO define/inheritance/anon/property regressions.

## STATUS: all gates green, footprint proven. Committing.
