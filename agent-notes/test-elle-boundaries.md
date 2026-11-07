# Elle boundary expansion

## Goal
Extend Elle's boundary schedules and recorder regressions for the approved
expansion campaign. This worker owns this isolated branch; root publishes and
merges after CI. No posting or pushing from this worker.

## State
Root supplied the completed pinned-baseline result before implementation:
workflow 42344496116, healthy verdicts true, all ten faults false, errors empty.
This branch's new tests and controls are implemented but unverified; no local
tests run. Root owns PR publication and CI evidence collection.

Changes: two list-append controls (`hot-map-carry`, `hot-memo-guard`), held runtime
reader/writer pairs, three actual-runtime recorder regressions, and one store
regression with own-publication and later-conflict cases. Reserved tokens 210-213.

Production trace: `task_runtime.go` holds the exclusive commit gate from the
irreversible-effect boundary until the slice commits. An ordinary writer cannot
interleave there. The recorder splits actual publications and must not invent a
post-boundary read for a value retained in a local variable.

## Decisions and why
Keep observations attached to actual publication boundaries and preserve
serializable. Add only deterministic schedules whose real dependency path can be
shown by a fault or a semantic recorder assertion. Source inspection is not a
passing verdict. The new branch must earn its own CI and fault witnesses.

Root agreed to demonstrate hot-renewal dependencies with Elle and keep the
irreversible carry details as semantic regressions. Planned held map pair: reader
observes a keyed row, writer publishes appends to that row and the hot property,
reader then attempts a stale-hot renewal. Healthy renewal refuses because the
keyed read is stale and returns the old property snapshot; removing cloned keyed
reads permits old-map/new-property skew. Planned memo pair uses a warmed missing
verb, a concurrent definition and hot-property append, and the renewal's dispatch
shape guard. Both observations stay in one actual reader publication.

## Surprises
There is no `responds_to` builtin in this checkout. Missing dispatch must be
observed through an actual caught verb-call error, not a fabricated helper result.

## Next step
Format the literal Go files, inspect the diff, commit this independent change and
return it to root for CI. Required evidence: healthy append/register true; both
new list-append faults false with cycle witnesses; committed pair counters and
specific renewal assertions; recorder and store regressions in CI.

## Scope remaining after this change
The irreversible boundary's memo materialization already has a store regression
in `store_txn_boundary_renew_test.go`. This change adds the missing republished-map
case and runtime recorder compositions. It does not claim a competing ordinary
commit can occur inside the exclusive gate, or impose whole-task serializability
across suspend/fork/publication boundaries. Cross-products of every map/dispatch
fixture from other campaign branches remain for later composition if a concrete
uncovered path requires one.
