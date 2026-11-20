# Map first and last index semantics

## Goal
Repair the newly executed map conformance failure using exact managed Toast
evidence, the raw MOO input as a VM regression, and the smallest production fix.
Root owns publication, CI and merge; this worker commits only.

## State
Pinned managed WSL Toast confirmed the exact reported input, with canonical
capability admission: `2 passed, 126 deselected`. Raw evidence is retained at
`/tmp/barn-map-first-last-oracle-corrected/{admission.json,toast.xml}`.
The raw compiler/VM regression is added; local Barn tests are prohibited.
Source inspection shows range markers use map size while index markers expose
keys. Toast OP_RANGE_REF seeks integer endpoints as keys too; Barn reads and
assignments special-cased integers as positions. The fix now resolves integer
endpoints through the existing KeyPosition path, and normalizes map range marker
operands to the existing index-marker key resolver. Numeric inverted reads still
return an empty map even if keys are missing. Lists and strings are unchanged.
The existing full-map read/assignment assertions remain, with the misleading
positional test name corrected. New regressions retain the exact reported input,
sparse-key reads, missing-key E_RANGE, and inverted missing-key reads.
`gofmt` was limited to the two changed Go files; `git diff --check` passed.
Barn tests and CI remain unrun by this worker; root owns exact-head CI.

## Reported failure
```text
expected [[1,1],[1,2],[1,3],[1,5],[1,8]]
actual   [[1,1],[1,2],[1,3],[1,4],[1,5]]
1 failed, 13453 passed, 316 skipped
```

## Decisions and why
- Preserve the exact reported MOO case as regression input before fixing source.
- Use the documented managed oracle, including canonical capability admission.
- No local Barn tests, publishing or unrelated worktree changes.
- Managed sparse-key edge session returned `5 passed, 1 warning` including
  canonical capability admission. It confirmed integer-key range selection,
  missing-key E_RANGE, inverted missing-key empty result, and sparse-marker
  assignment. Evidence: `/tmp/barn-map-first-last-oracle-corrected/edges.xml`
  and `edges-admission.json`; input remains in `.tmp/map-bounds-oracle.yaml`.
- Scope does not change other key-ordering, empty-marker, or range-assignment
  merge semantics. The existing map ordering and key lookup owners remain.

## Surprises
The first process launch preceded worktree creation and failed before running any
command. Root created the assigned worktree; no recovery or mutation was needed.
The initial wrapper run was `1 passed, 1 skipped, 126 deselected`: its editable
controller dependency lacked the merged semantic map feature probe. This is not
oracle confirmation. The corrected controller in mct-maps-capability executed
both tests successfully. The wrapper has no controller override; the documented
managed CLI selected that controller directly without changing either checkout.
An additional edge-probe session was refused before execution because its
candidate root excluded the supplied controller database. Exact refusal:
`candidate primary database escapes candidate checkout root`. Use the verified
common checkout parent as candidate root; preserve controller/database ownership.
One subsequent custom-selector invocation forwarded an unrecognized positional
YAML name to pytest (`module or package not found`, no tests ran). Passing the
documented explicit `--moo-suite-path` selected the custom suite successfully.

## Next step
Commit the scoped source, regression and note; draft the PR body for root.
Root reviews, publishes and runs exact-head CI including the conformance suite.
