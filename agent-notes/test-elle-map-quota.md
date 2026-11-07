# Map-key rebase quota regression

## Goal
Prove the existing commit-time map merge respects the task's exclusive list-byte
limit: allow disjoint updates whose union fits, reject a union at the limit, and
show serial VM retry returns E_QUOTA. Root owns publication and CI.

## State
Implemented a three-boundary VM regression in the existing `vm/map_key_test.go`
owner. No production changes or local tests. Exact-head CI and pinned Elle
verification remain root-owned. Branch `test/elle-map-quota` is independent.

## Decisions and why
- Extend existing test owners and helpers; do not copy them or invent a harness.
- Quota is a semantic invariant, not an Elle graph anomaly. Keep those claims
  separate and use actual VM property assignments for task-limit behavior.
- No local tests; root will publish the committed change for CI.
- Both transactions begin before either executes. Real indexed property
  assignments record the task-local MaxListValueBytes through
  `vm/op_property.go:executeSetPropKeyed`; `SetPropertyMapKey` saves the limit;
  `rebaseMapKeyWritesLocked` checks the union at commit. This is deterministic
  without another barrier, worker pool, hook, or copied test helper.
- Reuse `mapKeyVMStore` for the fixture and `runBytecodeProgram` for execution.
  Task-local ServerOptions supplies the same limit on both original attempts and
  the explicit serial retry. `VM.beginProgram` installs and primes its frame
  through the existing production helper.
- Calculate partial and merged sizes through types.ValueBytes, and assert both
  initial assignments fit. Cases put the merged size one byte below, exactly at,
  and one byte above the exclusive limit. Check both final entry values, no
  publication after a rejected merge, a precise map-key validation conflict, and
  E_QUOTA with no staged write on fresh-snapshot retry.
- This covers an explicit serial retry, not the engine scheduler's automatic
  retry machinery. No new conformance behavior is proposed: the exclusive
  assignment quota already has a VM regression; this adds commit-time merging.

## Surprises
None yet.

## Next step
Root publishes the commit, runs CI and inspects the regression and Elle evidence.
No quota failure or production defect has been claimed before that execution.
