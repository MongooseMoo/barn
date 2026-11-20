# Finally control transfers

## Scope

Repair break/continue through finally in the existing compiler and VM without
copying cleanup code or changing ordinary jump semantics. Coordinate shared
compiler/snapshot files with the native continuation worker. No publication.

## State

Corrected after independent analysis: singular pending return/error slots and
the separate jump stack lose an outer reason when nested cleanup creates an
inner reason that is later caught or canceled locally. Root verified all three
exact producers in nested_pending_cleanup.yaml against managed stock Toast
with admission (4 passed in 0.74s). The previous focused passing evidence did
not cover this ownership case and is insufficient for acceptance.

Corrective design: one owner-scoped pending-reason stack for return, error and
jump. Destination-based trimming removes only cleanup records actually left;
locally handled/canceled inner reasons preserve outer records. Existing old
private wire fields migrate at the codec boundary. No one-slot backups or
parallel runtime flags. Focused RED cases now pass, including nested reasons
through real suspension, retry and checkpoint restore. Current code is stable
for parent integration and independent review; no CI/isolation/merge claim.

Ownership agreed with native import worker; their final focused run completed
before shared-file edits started. New engine/finally_control_transfer_test.go
established RED: continue yielded {3}, break {}, nested cleanup {{2,2}}.

Managed stock Toast commands use make conformance-toast with canonical
capability admission and the new control_transfer_finally.yaml. Initial nested
transfer corpus: 6 passed, 1 warning in 0.91s. Additional nested cleanup pending
return/error corpus: 3 passed, 5 deselected, 1 warning in 0.89s. Sibling except
witness: 2 passed, 7 deselected, 1 warning in 0.89s. XML/admission retained in
the native clone .tmp/control-*. No new native expectation was assumed.

Implementation complete locally: explicit OP_UNWIND_JUMP uses the last available byte
opcode, absolute target and retained handler depth. Compiler loops retain that
depth; normal jumps/loops are unchanged. Pending jump records hold target,
depth and owning finally identity. Pending return/error also need an owning
finally identity: normal nested cleanup must not consume an outer pending
reason. All new state round-trips in private snapshot metadata.

An existing sibling-except lifetime defect must be corrected for lexical depth
to match runtime handler depth: a matched arm currently leaves sibling arms
active. The managed witness confirms a sibling must not catch an exception
raised in another arm. Use a one-based handler-group base, zero preserving
legacy snapshot interpretation, rather than guessing group membership.

Focused execution is GREEN after RED, including named outer transfers and
error/return overrides: ok engine1.070s. Pending transfers and pending reason
owners survive real suspend/checkpoint/reload and retry; a hand-assembled valid
program also proves a nonzero operand prefix survives cleanup. GC/root
inventories retain WAIF/anonymous values owned only by the pending stack.
Malformed pending targets/depth/stack are rejected. Focused VM run:
ok vm1.530s. Bytecode fork rebasing/disassembly/invalid targets/tick-free cycles:
ok bytecode1.139s. Added fork-under-parent-protection execution and extraction
checks: child handler depth starts at zero and absolute exit destinations
rebase correctly. Final focused commands returned:

```text
go test ./engine -run '^TestFinallyControlTransfer$' -count=1
ok github.com/MongooseMoo/barn/engine 1.126s
go test ./vm -run '^TestFinally' -count=1 -v
ok github.com/MongooseMoo/barn/vm 1.037s
go test ./bytecode -run '^TestUnwindJump' -count=1 -v
ok github.com/MongooseMoo/barn/bytecode 0.738s
```

These selectors exercised only the three new focused files, not full CI.
That result predates the independent nested-pending-reason finding below and
does not establish coverage for the corrected implementation.

## Design

Use an explicit unwind jump with target and retained handler depth. Preserve
pending cleanup transfers through suspension, retry and checkpoint. Reuse
existing handler execution and verifier, with nested cleanup identity retained.

## Next

Completed implementation: replace singular pending return/error and separate jumps with one
owner-scoped pending-reason stack. Root verified three nested cleanup witnesses
against managed Toast (4 passed including admission, 0.74s); Barn previously
returned 99, 42 and {"bad", "bad"} instead of 73, 73 and {}. A fourth managed
witness proves a pending jump canceled by a local cleanup break preserves the
outer return (2 passed, 11 deselected, 0.87s). New focused engine cases were RED.
Runtime, snapshot model, roots, writer, reader and test API have been converted.
Focused current results: engine TestFinallyControlTransfer `ok engine 1.345s`;
nested pending checkpoint/retry cases errors, returns and jump_then_return PASS
in the focused VM run `ok vm 2.547s`; explicit codec and writer selectors
`ok db/format 2.046s`; task transform `ok task 1.513s`. New simultaneous nested
return/error roots, pointer-clone and malformed-owner test `ok vm 1.104s`.
Discarded reasons enter existing finalization-candidate bookkeeping before
their records are cleared. Normal Return preserves pending values until the
existing frame-pop collector observes them. Legacy codec tests compile real
cleanup producers and cover ownerless unambiguous migration, ambiguous
ownerless rejection, explicit nested-owner migration and duplicate-owner
rejection. Normal old frames without pending reasons remain readable.
Legacy reason
ownership will migrate only where provable; ambiguous legacy history must error.
No commits or publication. Native FIN_EXIT restoration remains
explicitly unsupported in the separate converter until it maps to this state;
the original49 body-handler continuations do not require that additional case.

## Final focused verification after unified-stack cutover

```text
go test ./engine -run '^TestFinallyControlTransfer$' -count=1
ok github.com/MongooseMoo/barn/engine 1.317s
go test ./vm -run '^(TestFinallyLoopTransferSurvivesCheckpointAndRetry|TestFinallyNestedReasonsSurviveCheckpointAndRetry|TestFinallyNestedPendingRootsCloneAndInvalidState|TestFinallyJumpRetainsOperandPrefix|TestFinallyPendingReasonOwnerSurvivesCheckpoint|TestFinallyForkDoesNotInheritParentHandlers|TestExceptionReasonPendingFinallyRetryAndCheckpoint|TestExceptionReasonRestoreRejectsIncompatiblePendingMetadata|TestControlContinuationsSurviveVMSnapshot|TestRetryCheckpointDetachesMutableFrameStorage|TestVMRootCollectorsObserveEveryValueBearingField|TestPopFrameRetainsEveryFrameValueAndDiscardedOperand|TestCaughtErrorMaterializesObservedOrPendingValue)$' -count=1
ok github.com/MongooseMoo/barn/vm 1.535s
go test ./bytecode -run '^(TestUnwindJumpForkRebasingAndValidation|TestUnwindJumpCannotCreateTickFreeCycle)$' -count=1
ok github.com/MongooseMoo/barn/bytecode 1.302s
go test ./db/format -run '^(TestPendingErrorReasonPrivateMetadataRoundTrip|TestPendingErrorReasonLegacyAndInvalidMetadata|TestPendingErrorReasonRejectsMismatchedGuard|TestPendingReasonLegacyAmbiguousOwnershipRejected|TestPendingReasonLegacyExplicitOwnersMigrate|TestSuspendedVMWriterReaderPreservesReadyContinuation)$' -count=1
ok github.com/MongooseMoo/barn/db/format 1.957s
go test ./task -run '^TestTransformPersistenceValuesMatchesQueuedAndSuspendedWriterSurfaces$' -count=1
ok github.com/MongooseMoo/barn/task 1.401s
```

git diff --check emitted no output. No full suite, CI, publication or Elle run
was performed by this worker. Parent has the separate native49 integration and
startup-rejection checks and is beginning the player-workload smoke. Next:
independent review of unified owner stack; parent integration and diagnostics.
