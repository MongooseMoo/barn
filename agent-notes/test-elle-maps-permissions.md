# Elle maps and permissions expansion

## Goal
Extend the existing Elle corpus with deterministic map transition and ordinary-user
permission conflicts, using real MOO observations and demonstrated fault controls.
This worker owns branch `test/elle-maps-permissions`; publication belongs to root.

## State
Root reported pinned baseline healthy true/fault false with retained evidence.
Implemented eight held map/authority pairs and three narrow checker-only faults.
No local Barn tests were run; exact-head CI and Elle verdicts are pending root's
publication. `gofmt` and `git diff --check` completed without output.

Managed WSL Toast command in this worktree:
`make conformance-toast K="map_authority or builtin_property_write_permissions or owner_change_permission or clear_property_call_shapes"`
ran through its terminal summary: `1 failed, 27 passed, 13219 deselected`.
The sole failure was the existing integer-key comparator case
`int_keys_separated_by_2_to_32_collide_and_replace`; its raw mismatch is preserved
below. This branch uses string keys and does not alter integer comparison.

## Decisions and why
- Preserve the existing disjoint-key positive control and serializable model.
- Use unique register values for replacements and append values only for genuinely
  monotone rows. Permission races require a dependency witness, not wall time.
- Keep each change local to this worktree and preserve concurrent workers' work.
- Use one-shot transitions normalized to `{}` before and `{token}` after a real
  operation, matching the existing valid/recycle encoding. Each scenario owns a
  map and marker; its writer observes the marker and publishes the transition
  before the held reader appends. Missing-key `E_RANGE` and denied-read `E_PERM`
  are caught only where they are the explicit observed data for the scenario.
- Production sources: `vm/op_property.go` records a keyed observation before
  `vm/op_index.go` returns E_RANGE for a missing key. `store_txn_mapkeys.go`
  validates missing/present entries and permission metadata. A whole-property
  read after a keyed write records its original version in
  `store_txn_snapshot.go:markPropertyReadKey`.
- Planned controls remove only absent-key validation, keyed metadata validation,
  and whole-property dependency promotion after a keyed write. Existing broad
  controls continue to exercise their original schedules.
- Eight implemented schedules cover keyed-vs-whole replacement, whole-vs-keyed,
  whole-after-keyed, absent-entry insertion, inherited-map fallback, clearing an
  override, nonwizard permission revocation, and nonwizard owner revocation.
  Existing disjoint-key and captured-index waves remain unchanged.
- Captured RHS base changes, scalar object flags, verb permissions, and quota
  rebase behavior are not added by this first independent PR. Root must retain
  those campaign items rather than infer them from these map permission pairs.

## Surprises
Toast returned one colliding entry for integer keys 0 and 4294967296, where the
current conformance assertion expected two entries:

```text
expected [[2, 'zero', 'wide', [0, 4294967296]], 2, 'replacement', 'wide', [0, 4294967296]]
actual   [[1, 'wide', 'wide', [4294967296]], 1, 'replacement', 'replacement', [0]]
1 failed, 27 passed, 13219 deselected in 295.63s (0:04:55)
```

No Barn-side diagnosis or comparison change was made for that oracle mismatch.

## Next step
Root publishes the independent branch, runs PR CI and inspects healthy/fault
verdicts and witnesses. Repair findings in this worktree before root merges.
