# Elle boundary expansion

## Goal
Extend Elle's boundary schedules and recorder regressions for the approved
expansion campaign. This worker owns this isolated branch; root publishes and
merges after CI. No posting or pushing from this worker.

## State
Root supplied the completed pinned-baseline result before implementation:
workflow 42344496116, healthy verdicts true, all ten faults false, errors empty.
The first PR CI exposed a fixture collision, repaired below. Follow-up workflow
42345607812 completed healthy true and both new fault controls false with
witnesses, as reported by root from retained evidence. No local tests run.
Root owns PR publication and CI evidence collection.

Integrated the actual default branch after the independent history-shapes and
manual-campaign PRs merged. Resolved fault-list unions as the original ten,
ring-reads, hot-map-carry, hot-memo-guard in the same order in Go and Python.
Preserved history shapes, the ring workload, manual inputs/action pins/cache
recording restriction, and receiver 18 with its fixture-isolation assertion.
That integrated tree passed Build & unit tests and MOO conformance in workflow
42347444392 and Serializable histories and negative control in workflow
42347444400. Those results precede the next default-branch advance.

Integrated the actual default branch again after the map/permission PR merged.
The combined fault order is original ten, ring-reads, map-key-metadata,
map-key-presence, map-key-whole-read, hot-map-carry, hot-memo-guard in both Go and
Python. Kept the map helper's precise fault selection and healthy eight-pair
committed-key checks, the hot-renewal helper/counters, receiver isolation, ring and
random shapes, manual workflow controls, and quota semantic regression/docs.
All sixteen controls and healthy combined histories now require fresh CI.

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

PR CI run 42345121723 failed in seed-412-go.log:
`task 100062 native error E_INVARG` at
`if (length(verbs(#42)) != 1) raise(E_INVARG);`.
The boundary fixture added renew_missing to #42, one of the moving invariant
objects whose own verb list must contain only scratchV. The unchanged invariant
correctly detected contamination. The fixture now chooses #18, whose producer
creates a data property and parent #0 but no own verb. The existing warm task
asserts that the chosen receiver has neither an own verb nor the invariant
population's scratch property before adding renew_missing. Every call and mutation
uses the same receiver constant; the original invariant check remains unchanged.

## Next step
Return the integration merge to root for CI. Required evidence: healthy
append/register true; all sixteen list-append faults false with cycle witnesses;
committed pair and shape counters, specific renewal assertions, and recorder/store
regressions in the integrated tree's CI.

## Scope remaining after this change
The irreversible boundary's memo materialization already has a store regression
in `store_txn_boundary_renew_test.go`. This change adds the missing republished-map
case and runtime recorder compositions. It does not claim a competing ordinary
commit can occur inside the exclusive gate, or impose whole-task serializability
across suspend/fork/publication boundaries. Cross-products of every map/dispatch
fixture from other campaign branches remain for later composition if a concrete
uncovered path requires one.
