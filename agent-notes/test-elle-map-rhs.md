# Captured map RHS producer regression

## Goal
Fill the real VM/MOO producer gap for side-effectful RHS evaluation during keyed
property assignment. Root owns publication and CI; this worker commits only.

## State
Implemented the missing real-VM regression after exact managed WSL Toast evidence.
No production changes and no local Barn tests. The original head passed all three
required PR checks. Integration with actual merged master retains both this RHS
regression and the quota regression in the same existing VM test owner. The
comment/note distinction from private review is retained. Combined-head CI remains
required before normal merge.

## Decisions and why
Reuse existing fixtures and keyed tracking; add no duplicate store helpers.

## Findings
- Existing `TestMapKeyWritebackOfAStaleBaseIsAWholeWrite` manually calls store
  capture/write helpers. `vm/map_key_test.go` covers side effects inside an index
  expression but had no side-effectful RHS writeback producer.
- `compiler/lower_assignments.go` captures the property base before compiling
  the RHS and emits OP_SET_PROP_KEYED afterward. `vm/op_property.go` asks
  KeyedWritebackView whether the pending captured base still matches the slot;
  fallback executes the ordinary whole-property setter.
- `pendingKeyedBase` rejects a changed/consumed base. The ordinary setter records
  a whole read via SetPropertyValue/markPropertyReadKey, which must conflict even
  when another task changes a third key not mentioned by the RHS.
- The new VM regression runs nested keyed and whole-map RHS assignments as MOO.
  Each checks its assignment result and actual a/b values, then tests both serial
  publication and conflict with a separately committed new key c. The keyed-RHS
  assertions do not introduce a whole-property dependency, so that case isolates
  the outer writeback guard. The whole-RHS assignment inherently records a whole
  dependency before the outer writeback; its conflict is semantic coverage, not
  independent proof of that guard.
- This is semantic producer coverage. No new Elle schedule/control is claimed:
  existing controls target their own concrete workloads; adding another helper
  would not make this minimal VM producer test an additional anomaly witness.

## Exact oracle evidence
From this worktree, documented wrapper `scripts/check-conformance.sh toast
/root/src/toaststunt/build-release/moo /tmp/barn-elle-map-rhs-oracle-executed
"rhs_keyed_write or rhs_whole_write"`, with MOO_PORT=17949,
UV_PROJECT_ENVIRONMENT=/root/.cache/barn-elle-map-rhs-oracle,
candidate-root=/mnt/c/Users/Q/code and moo-suite-root pointing to this worktree's
`.tmp/oracle` directory. Canonical capability admission ran in the same session.

The exact MOO statements verified were outer `o.m["a"]=(o.m["b"]=5)` and outer
`o.m["a"]=(o.m=["a"->2,"b"->3])["b"]`, both starting from a=0,b=0.
Actual return/result tuples matched `{5,5,0}` and `{3,3,0}` respectively.
Raw terminal summary: `3 passed, 1 warning in 8.00s`; JUnit has zero skipped,
failed or error nodes. Raw JUnit/admission files remain under
`/tmp/barn-elle-map-rhs-oracle-executed` in Debian. Probe YAML remains in `.tmp/oracle`.

## Surprises
The first managed probe collected both cases but skipped them for `Requires
feature: maps`; the copied feature marker is absent from the stock manifest.
The probe now requires real map builtins, matching the existing map_authority
suite. Canonical admission remains required. No semantic result is claimed from
the skipped probe.

## Next step
Parent publishes this independent branch and runs PR CI. Inspect the real VM
regression and existing Elle controls before normal merge. No result is inferred
from formatting or source inspection alone.
