# COW Phase 2 — coder report

## VERDICT: GO — committed

- **Commit:** `7190f97` "Decentralize property define/delete commits over the inheriting subtree (COW phase 2)"
- **Branch:** `work/mvcc-concurrent-moo` (no push/merge/rebase/switch performed)
- **Make target:** `make build` → `go build -o barn.exe ./cmd/barn/` (canonical `barn.exe`; no other server binary built; no stray `barn-*.exe`).

All five gates green AND the define/delete footprint is proven complete. Property DEFINE
(`propertyDefines`) and DEFINITION-DELETE (`propertyDefinitionDeletes`) commit-apply writes are
now on the decentralized COW path; the coarse `store.mu.Lock` commit fallback is reached only for
`liveMutated` commits.

## What changed
- `db/store/store_cow.go`:
  - New `buildImageWithPropertyDefine` and `buildImageWithPropertyDefinitionDelete` — DEFINER-ONLY
    image builds (copy `properties` map + `propOrder` slice + `propDefsCount`; share every other
    collection), mirroring `definePropertyLocked` / `deleteDefinedPropertyLocked` minus the
    descendant walk.
  - `commitDecentralized`: footprint now collects `propertyDefines` + `propertyDefinitionDeletes`
    objIDs; pre-build validation (define→E_INVARG on collision, delete→E_PROPNF if not defined),
    all before `bumpClock`/build (atomic, no partial side effect); builds defines/def-deletes on the
    definer image in fixed kind order.
  - `buildImageWithPropertyValue` new-slot branch now HONORS the staged `clear` flag instead of
    forcing `clear=false` (required: a define-propagated descendant slot stages `clear=true`).
- `db/store/store_txn.go`: removed the `len(propertyDefines)==0 && len(propertyDefinitionDeletes)==0`
  guard from `Commit` routing; now only `tx.liveMutated` falls back to the coarse path.

## Resolution of the 3 required questions

### Q1 — Footprint completeness (PROVEN COMPLETE)
The txn staging side **already enumerates the full subtree**. `StoreTxn.DefineProperty`
(`store_txn.go:1188`) stages `propertyDefines[key]=prop` for the DEFINER, then
`propagateDefinedProperty` (`:1219`) BFS-walks `current.children` and stages a per-descendant
clear-slot `propertyWrites` entry for every inheriting descendant. `DeleteDefinedProperty`
(`:1319`) + `removeInheritedProperty` (`:1353`) stages a per-descendant `propertyDeletes` for each.
So at commit, `{definer in propertyDefines/Deletes} ∪ {descendants in propertyWrites/propertyDeletes}`
= the exact set the coarse `propagatePropertyToDescendantsLocked` (`store_properties.go:628`) /
`removeInheritedPropertyLocked` (`:671`) would write. The decentralized committer locks every one of
those slots ascending by ObjID.

- Descendants that **already define an override** of the property are correctly EXCLUDED from the
  write set in BOTH paths (staging `:1241-1243` and coarse `:648-650` skip the write but continue the
  walk), and deeper descendants below an override are correctly INCLUDED in both.
- **Topology can't shift under the committer:** only coarse `store.mu.Lock` mutators change
  `children`; the decentralized committer holds `store.mu.RLock` (which excludes `Lock`), so the
  subtree enumerated at stage time is frozen through build+publish. Additionally, a concurrently
  added descendant cannot silently escape: `propagateDefinedProperty`/`removeInheritedProperty` call
  `markObjectRelationshipRead` on every walked node (`:1233,:1367`), recording each parent's
  `relationshipVersion`; `CreateObject` bumps the parent's `relationshipVersion` on child-attach
  (`store_lifecycle.go:37`), so `validateObjectRelationshipReadsLocked` fails the commit (E_INVARG)
  and the retry re-walks to cover the new child.

### Q2 — Anonymous objects (RESOLVED: cannot propagate to anon)
`attachChildToParentsLocked` (`store_relationships.go:5-23`) puts anonymous children into
`parent.anonymousChildren` and NON-anon into `parent.children`. Every propagation walker (coarse and
txn-staging) iterates `current.children` **only**, never `anonymousChildren`. So a property
define/delete footprint **never includes an anonymous descendant** — no anon slot work is needed.
This matches Toast: `builtins/properties.go:343-344,395-396` note "ToastStunt does NOT invalidate
anonymous descendants when a parent's property schema changes." No `anonObjects` involvement.

### Q3 — Verb verbs/verbList aliasing (CONFIRMED, and not exercised by define/delete)
`buildImageWithVerbCode` (`store_cow.go`) already substitutes the edited `*Verb` identity-preservingly
into BOTH `verbs` (map) and `verbList` (slice), like `cloneObjectForReadTxn`. The new define/delete
builders do not touch verbs at all (they copy only `properties`/`propOrder`/`propDefsCount` and share
`verbs`/`verbList` by reference), so verb aliasing is preserved by construction.

## New stress test
`TestCOWConcurrentDefineDeleteSubtreeRaceFree` in `db/store/cow_define_subtree_race_test.go`:
- 8 disjoint subtrees (root + 2 chains × depth 3). Each subtree writer repeatedly DEFINEs a
  uniquely-named property on its root then DELETEs it, each via `StoreTxn.Commit` (decentralized COW,
  !liveMutated). After define it asserts **every descendant resolves the inherited value** (no
  lost/torn define); after delete it asserts **every descendant returns E_PROPNF** (no lost delete).
- 16 reader goroutines hammer the raw `Store.*` funnel (`PropertyValue`/`ObjectName`/`Parents`) on
  descendants and assert: whenever the property is present, its inherited value is the single correct
  value (never torn/partial).
- 8 disjoint ordinary property-value committers run concurrently (proves define/delete commits run
  alongside ordinary decentralized commits).
- Final state asserts all subtree props gone (each writer ends on delete) and all objects readable.
- **Result:** `go test -race -run TestCOWConcurrentDefineDeleteSubtreeRaceFree ./db/store` → ok 1.756s
  (-race clean, all inheritance/correctness assertions pass).

## Gate results (all green)
1. **build/vet:** `go build ./...` clean; `go vet ./db/store` clean (no copylocks). PASS.
2. **`go test ./...`:** all packages ok (builtins, db/store, scheduler, vm, server, …). PASS. (Caught
   one real bug — see below — fixed, re-green.)
3. **`go test -race ./db/store ./vm ./scheduler -count=1`:** clean — db/store 1.978s, vm 1.161s,
   scheduler 119.132s. Includes existing COW race tests + the new stress test. PASS.
4. **Full managed conformance** (once, against the `make build` `barn.exe`):
   `16 failed, 3971 passed, 131 skipped`. All 16 failures proven PRE-EXISTING (no define/inheritance/
   anon/property regression): 9× `limits::*` E_QUOTA value-bytes (match Phase 1 verify report) + 7
   non-property (gap_followups telnet IAC, 2× verb_dispatch shadowing, 2× control_flow ternary-string,
   2× index_and_range dollar-list). The 7 non-limits were re-run against a HEAD baseline build
   (without my change): `7 failed` identically → proven pre-existing. PASS.
5. **Scaling** `go test ./scheduler -run TestConcurrencyCommitDominatedDisjoint -count=3`: all 3 runs
   PASS; speedup@32 = 1.40x / 1.17x / 1.14x (noisy microbench; monotonic increase with worker count
   holds every run). This path is property-VALUE commits, untouched by the Phase 2 define/delete edits,
   so no scaling regression. PASS.

## Bug found + fixed during testing
`TestTransactionDefinePropertyStagesAndPropagatesOnCommit` initially failed: a descendant's clear
inherited slot came back `clear=false`. Root cause: `buildImageWithPropertyValue`'s new-slot (else)
branch forced `np.clear=false`. Under the old coarse routing this was harmless (the define loop created
the `clear=true` slot first, so the propertyWrites loop hit the existing-slot branch). Under the
decentralized path a descendant has ONLY the staged `propertyWrite` (which carries `clear=true` from
`propagateDefinedProperty`), so the else branch dropped the clear flag. Fix: honor `w.prop.clear`.
Verified correct for both normal SetPropertyValue overrides (stage `clear=false`) and define-propagated
descendant slots (stage `clear=true`).

## Adversarial review
An analyst pass verified the 5 correctness properties (footprint completeness, propOrder/propDefsCount
fidelity, the clear-flag fix, error/atomicity contract, case-sensitivity) — all CLEAN. Its one raised
risk (FINDING B: a concurrently-added descendant escaping the footprint) is resolved above via
relationship-read validation + RLock/Lock exclusion. A fragility note (FINDING A) was addressed with a
guard comment at `store_cow.go:389` ("do not reorder these checks above the validLiveObject loop").
