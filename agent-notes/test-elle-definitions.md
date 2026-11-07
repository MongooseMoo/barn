# Elle definition and dispatch expansion

## Goal
Extend the existing Elle workload with actual definition, inheritance, and dispatch
dependencies and demonstrated negative controls. Parent owns publication and CI.

## State
Implemented sixteen held scenarios and four precise negative controls; awaiting
parent-owned PR CI. No Barn tests run locally. Parent confirmed the existing
pinned baseline passed with every healthy history true and each fault false.
This worktree is isolated on test/elle-definitions.

## Decisions and why
Use existing workload, recorder, pause hooks and driver. Keep history values actual
MOO observations and use append rows only when the operation is monotone.

## Findings
- `store_txn_properties.go` records absent lookup fall-through with shape scans;
  `DefinedPropertyNames` records a coarse property scan. The property-reads fault
  removes neither. Add narrowly scoped property-shapes and property-scans controls.
- `store_txn_verbs.go` records list generation for numeric lookup, and walks mark
  verb scans. Store dispatch memo hits may replace those marks with a shape clock
  dependency. Add independent verb-scans and verb-memo controls.
- One-shot transitions can be normalized to append rows: absent to present, or
  present to absent, advances an empty list to one unique token exactly once.
  This is the same model as the existing valid/recycle pair, not a fabricated
  version clock. Returned dispatch/property lists carry their actual token.
- Each held pair: observer reads transition row X and stages an append to ledger
  Y; mutator waits for the X read, reads Y and X, performs transition X and
  publishes before the observer commits. Under missing validation the two rw edges form a cycle. Healthy
  observer retries and reads the new X before publishing Y.
- Fixture objects reserve IDs 400-447. Each scenario has separate objects/row;
  no mutation from one scenario changes another's definition state.

## Proposed schedules
Absent property lookup/create; property enumeration/create/delete/rename;
inherited value override/clear/reparent; absent verb lookup/create;
verb enumeration/create/delete/rename; warmed dispatch code replacement,
shadow addition/deletion/reparent; numeric verb selection after first-verb deletion.
Use minimal controls only for independently validated dependency classes. Prove
committed scenario coverage separately from general verb_memo/path counters.

## Surprises
- The hot #11.data row would independently revalidate memo dependencies while
  renewing after the held writer. Use a dedicated non-hot definition ledger to
  avoid masking the narrow verb-memo fault.
- Warming dispatch inside the definition-setup task cannot populate the shared
  memo after private shape edits. The warm read is a separate committed task.

## Oracle evidence
Documented managed command from this worktree: `MOO_PORT=17948
UV_PROJECT_ENVIRONMENT=/root/.cache/barn-elle-definition-oracle
scripts/check-conformance.sh toast /root/src/toaststunt/build-release/moo
/tmp/barn-elle-definition-oracle` with selectors for property inheritance,
definition order, deletion, metadata changes, respond_to and verb identity, plus
the script's canonical capability admission. Raw terminal output:
`35 passed, 144 deselected, 1 warning in 61.42s (0:01:01)`.
JUnit/admission evidence remains under `/tmp/barn-elle-definition-oracle` in Debian.
This proves selected MOO semantics, not concurrent Barn isolation.

## Next step
Parent publishes this independent branch, runs exact-head PR CI, inspects healthy
and fault EDN/witnesses, and merges normally only after the checks succeed.
