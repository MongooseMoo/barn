# Elle maps and permissions expansion

## Goal
Extend the existing Elle corpus with deterministic map transition and ordinary-user
permission conflicts, using real MOO observations and demonstrated fault controls.
This worker owns branch `test/elle-maps-permissions`; publication belongs to root.

## State
Root reported pinned baseline healthy true/fault false with retained evidence.
Implemented eight held map/authority pairs and three narrow checker-only faults.
After the assembly and fault-scope fixes, workflow 42346406743 accepted all six
healthy model/seed histories and rejected all controls with no driver errors.
Integrating merged master retains the ring control and all varied history shapes,
plus manual campaign inputs and their standard-cache exclusion. The combined
source needs its own exact-head CI; earlier evidence is not integration proof.
PR CI failed because scenario source fragments were concatenated without a token
separator. Raw `seed-412-go.log` from workflow 42345312373 reported:
`CompileMOO() failed: Line 1: Unknown built-in function: endtryelle_observe`.
The completed prefixes of those histories are not valid negative-control evidence;
in particular the metadata/presence true verdicts came from incomplete workloads.

The repair changes the source assembly owner: every setup, reader, writer and
final-observer fragment is joined with a newline. This mechanically separates
future endtry/endif block endings from following tokens instead of depending on
individual case strings to carry whitespace. No checker or expected verdict was
weakened. No local Barn tests were run; root must publish and inspect fresh CI.

The fresh run from workflow 42345893470 completed healthy histories with true
verdicts and all thirteen fault append histories with false verdicts, but the
property-reads Go workload failed before completion. Its raw log reported task
900023 E_INVARG at the final `map-whole-after-keyed` assertion expecting {600004}.
Source trace confirms that the broad property-read fault disabled the whole-map
dependency after the reader's keyed write, allowing its old whole map to replace
the writer's updated entry. The assertion correctly exposed injected corruption;
a false Elle verdict from this incomplete workload does not satisfy the driver.

Repair the fault workload's scope rather than its assertion: healthy runs keep
all eight scenarios; metadata faults keep both revocations; presence faults keep
the absent-entry scenario; whole-read faults keep whole-after-keyed. Other faults
do not run these off-target map fixtures and keep their original witness waves.
Final assertions, native error handling, selected committed-row checks and driver
requirements are unchanged. No local tests; new exact-head CI remains required.

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
Root pushes the source-assembly repair, runs fresh PR CI and inspects complete
healthy/fault histories and witnesses. The failed run's partial histories remain
retained as fixture-build failure evidence, not passing control evidence.

## Follow-up scope after the first PR merges
Read-only research requested by root; no follow-up implementation yet. New work
must start independently from the merged default branch, never stack on this PR.

### Existing regression mapping: retain rather than duplicate
- `vm/map_key_test.go:TestKeyedReadCapturedBeforeIndexWriteConflicts` already
  executes the captured-index MOO producer through the VM and checks a real stale
  transaction loses validation. `TestKeyedReadCapturedAfterOwnWriteRemainsIndependent`
  covers the inverse case and checks the published values. Existing Elle's
  captured-map wave plus map-key-capture control already gives the external
  witness. No extra captured-index case is needed.
- `builtins/verbs_txn_test.go:TestDeleteVerbAuthorityRevocationConflictsBeforeMutation`
  admits the real delete_verb builtin as a nonwizard, revokes both object owner
  and write flag before commit, checks E_INVARG/ValidationFailed and preservation
  of the live verb, then verifies a fresh serial retry returns E_PERM. This is
  concrete existing scalar-owner/flag permission coverage. Keep it mapped in
  docs, rather than duplicate the same deletion scenario in Elle.
- `db/store/store_verbs_test.go:TestDeleteResolvedVerbAuthorizedUsesCurrentLiveAuthority`
  covers the live authority producer for owner, write flag, wizard and revoked
  owner. `db/store/object_authority_test.go:TestObjectAllows` covers the pure rule;
  these alone would not prove retry behavior, which the builtin regression does.
- `vm/bytecode_execution_test.go:TestMapIndexAssignmentUsesPendingListValueByteLimit`
  proves assignment uses the task's list-byte limit; existing map range/literal
  tests distinguish the map-byte limit. These are adequate local assignment
  semantics, but they do not exercise merging disjoint writes at commit.

### Concrete remaining gaps and minimal follow-up
1. Captured RHS writeback: `db/store/store_txn_mapkeys_test.go:TestMapKeyWritebackOfAStaleBaseIsAWholeWrite`
   proves the store fallback by calling MapPropertyForKey/SetPropertyMapKey
   directly. The VM map tests do not execute a property indexed assignment whose
   RHS writes the same slot. Add one held map pair using the existing helper:
   an outer assignment writes b from a captured map while its RHS writes a;
   outer writeback must restore the captured a and retain its whole dependency.
   Concurrently append a token to a. The final observed a is the row, and b is
   private unobserved scratch. Verify exact MOO evaluation order with managed
   Toast before adding it. Existing property-read validation is the protection;
   do not add a second generalized fault framework.
2. Map rebase quota: every store map-key regression passes zero sizeLimit, while
   `rebaseMapKeyWritesLocked` explicitly rejects a merged map whose ValueBytes
   reaches the saved limit. Add one deterministic engine regression using two
   tasks writing disjoint keys from the same snapshot. Choose values so each
   staged map fits but their union does not; a first-attempt barrier makes both
   individual assignments succeed. The second commit must lose validation and
   retry into the ordinary E_QUOTA assignment result; assert exactly one stored
   value and one quota result. Set the limit through existing task/server option
   facilities and check it survives retry. Use an explicit semantic invariant,
   not a fabricated append cycle: disjoint-key histories can be serializable
   even when an oversized map violates the separate quota contract.
3. Verb read permission changes: the existing deletion regression covers object
   owner/flag authority, but not a verb's own read bit. Add a nonwizard
   verb_code(...,"secret") success-to-E_PERM transition against set_verb_info
   removing r, using the existing held helper and marker cycle. Read producer is
   `builtins/verbs.go:builtinVerbCode`, backed by resolved verb version validation.
   Verify the exact scenario with managed Toast first. Record expected E_PERM as
   data and preserve any other native error.

The follow-up should document the existing scalar/flag regression mapping, add
only these concrete gaps, and retain the original map/disjoint-key controls.
