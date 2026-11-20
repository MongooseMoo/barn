# Control-transfer analysis

## Current scoped result: prior finding addressed

Reviewed the actual unified owner-scoped PendingReasons replacement, excluding
unchanged loop/layout work. The three original runtime probes now return73,
73 and{} respectively. Root supplied managed native confirmation for those
cases and the cancel-before-final-destination case. No further concrete problem
was found in this scoped correction. This is not whole-change merge approval.

Checked that old pending reasons survive while a nested transfer is only
entering cleanup, and are trimmed at the next actual destination only when
their owner is left. Snapshot and retry paths detach reason stacks and mutable
VMException pointers; live/finalizable root inventories and persistence value
rewriting cover every reason value/error/operand prefix. Canonical metadata has
one reason stack; legacy separate slots migrate only with provable ownership,
and ambiguous histories reject. Native imported except-group bases remain
frame-relative and agree with runtime group lifetime. Unsupported native
FIN_EXIT remains an explicit error outside the original-checkpoint claim.

Independent focused outputs after this actual fix:

```text
go test ./engine -run '^TestFinallyControlTransfer$' -count=1
ok github.com/MongooseMoo/barn/engine 1.228s
go test ./vm -run '^(TestFinallyLoopTransferSurvivesCheckpointAndRetry|TestFinallyNestedReasonsSurviveCheckpointAndRetry|TestFinallyNestedPendingRootsCloneAndInvalidState|TestFinallyJumpRetainsOperandPrefix|TestFinallyPendingReasonOwnerSurvivesCheckpoint|TestFinallyForkDoesNotInheritParentHandlers)$' -count=1
ok github.com/MongooseMoo/barn/vm 1.288s
go test ./db/format -run '^(TestPendingErrorReasonPrivateMetadataRoundTrip|TestPendingErrorReasonLegacyAndInvalidMetadata|TestPendingErrorReasonRejectsMismatchedGuard|TestPendingReasonLegacyAmbiguousOwnershipRejected|TestPendingReasonLegacyExplicitOwnersMigrate)$' -count=1
ok github.com/MongooseMoo/barn/db/format 1.531s
go test ./task -run '^TestTransformPersistenceValuesMatchesQueuedAndSuspendedWriterSurfaces$' -count=1
ok github.com/MongooseMoo/barn/task 1.348s
```

No full-suite, race, CI, Elle or clean performance comparison was performed.
The earlier candidate finding and pre-fix outputs below are historical evidence.

## Scope

Read-only review of the new unwind-jump implementation, pending cleanup state,
checkpoint/retry/root ownership, compiler handler depths and native-import
handler grouping. Earlier range/reader findings are excluded except for direct
interaction. No implementation edits or publication.

## State

Inspecting the completed control-transfer diff and focused tests. Findings will
be recorded with producer/source evidence; no approval is implied.

## Candidate finding awaiting exact native witness

The new finally-IP owner fields distinguish a pending reason's cleanup, but
PendingError and PendingReturn remain singular slots. Entering nested cleanup
for a new error or return overwrites an outer reason. Catching/cancelling that
new reason inside the outer cleanup does not restore the outer reason.

Actual current Barn probes (ordinary moo execution, no test/server launch):

```text
try try raise(E_TYPE); finally try try raise(E_INVARG); finally x=1; endtry except(E_INVARG) x=2; endtry endtry except(E_TYPE) return73; endtry return99;
=>99

try return73; finally for j in[1..1] try return99; finally break; endtry endfor endtry return42;
=>42
```

Whitespace is abbreviated in the excerpts; full exact commands were sent to
root. The expected outer73 needs the managed Toast witness before a parity
finding is finalized. Source paths: vm/vm.go handleErrorWithReason overwrites
PendingError; vm/stack.go Return overwrites PendingReturn; executeEndFinally
clears the active slot when the nested reason completes. No implementation edit.

A third probe shows the same loss for the newly introduced PendingJumps stack:

```text
seen={}; for i in[1..2] try continue; finally for j in[1..1] try return99; finally break; endtry endfor endtry seen={@seen,"bad"}; endfor return seen;
=> {"bad", "bad"}
```

Return unconditionally clears all pending jumps before invoking an inner
cleanup. If that inner return is then cancelled by a local break, the enclosing
continue is already lost and code after the try runs. This is the same pending
reason preservation finding; exact native expectation was requested from root.

## Focused checks

The three authorized focused selectors passed, despite the missing cases above:

```text
go test ./engine -run '^TestFinallyControlTransfer$' -count=1
ok github.com/MongooseMoo/barn/engine 1.361s
go test ./vm -run '^TestFinally' -count=1
ok github.com/MongooseMoo/barn/vm 1.419s
go test ./bytecode -run '^TestUnwindJump' -count=1
ok github.com/MongooseMoo/barn/bytecode 0.922s
```

Reviewed pending-stack cloning, typed value-root traversal, metadata codec,
handler-group bases, fork depth reset/rebasing and ordinary jump compatibility.
No separate proven defect found there in this review. The unfinished native
FIN_EXIT converter is explicitly outside the acceptance claim and excluded.
