# Elle definition and dispatch expansion

## Goal
Extend the existing Elle workload with actual definition, inheritance, and dispatch
dependencies and demonstrated negative controls. Parent owns publication and CI.

## State
Implemented sixteen held scenarios and four precise negative controls. The first
PR CI accepted the healthy histories and rejected each deliberate fault. This
branch now integrates actual default-branch changes after the history-shapes,
campaign, map/permission, boundary, verb-read, RHS and map-range repair PRs
merged. New integration CI is pending. No Barn tests run locally.
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
Root reviews and pushes the final integration merge and inspects exact-head PR CI
before normal merge. The boundary, verb-read and RHS PRs were confirmed MERGED
before fetching actual origin/master; no unmerged peer reference was integrated.
Root owns the next push and exact-head CI; no local Barn tests or publication by
this worker.

## Prior PR evidence and integration
Retained `barn-elle-evidence/pr628/elle-evidence/results.json` reports explicit true
with exit 0 for append/register histories of seeds 412, 395 and 396; all ten old
controls and the four new controls report false with exit 1. The retained
seed-412 manifest has every one of the sixteen definition/NAME paths equal to 1.
These are pre-integration results, not proof for the merged tree.

The final integration retains the default branch's four-transaction ring,
variable transaction shapes, disjoint random token allocator, map/permission
histories and committed row counts, hot-renewal pairs and counters, recorder and
store boundary regressions, manual workflow inputs, action pins and self-hosted
cache selection. It also preserves the merged map first/last production repair,
RHS map writeback regression, quota cases and verb metadata runtime dependency
regression unchanged against actual default branch.

The fault order is baseline ten, ring-reads, map-key-metadata, map-key-presence,
map-key-whole-read, hot-map-carry, hot-memo-guard, then property-scans,
property-shapes, verb-scans and verb-memo. A source-only structural comparison
reports `Go=20, Python=20, Table=20, GoPythonOrder=True, TableOrder=True`.
Definition helper/scenarios are unchanged. Shared actual conflicts in fault
lists, helper calls and documentation were resolved by union, without changing
fault scope, native errors or assertions. Integrated runtime acceptance remains
unknown until the new CI run.
