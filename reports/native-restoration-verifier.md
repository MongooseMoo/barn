# Native restoration verifier

## State

Review complete. Scope is the shared native restoration, cleanup reason,
bootstrap schedule preservation and supporting saved evidence. No implementation
edits, local test runs, publishing or merge are authorized for this review.

The role's advertised C:/Users/Q/.Codex/AGENTS.md.d/gauntlet.md is absent. The
installed gauntlet skill previously resolved by the parent supplies the same
Scout/Coder/Analyst/Verifier workflow and default NO-MERGE requirement.

## Verdict

NO-MERGE pending required exact-head PR CI and pinned Elle evidence. No new
concrete source blocker was established in this scoped read-only review.

## Checked source

- compiler/native_import.go parses once into the existing IR, obtains an exact
  native callsite, lowers through the existing Barn lowerer with import-only
  metadata, and translates retained values. It does not execute source effects.
  vm/persistence.go:190 rejects mixed frame encodings/nonzero VM function IDs;
  validateVMSnapshot then invokes the existing bytecode stack verifier.
- Unsupported builtin continuations, active native pending-finally reasons and
  incompatible byte-string iteration reject explicitly. These are compatibility
  limits, not complete generic native checkpoint support.
- vm/control.go uses one owner-scoped PendingReasons stack. Nested transfer
  cancellation tests containment at the next actual destination. Snapshot/retry
  copies detach operand prefixes and mutable VMException pointers. Root and
  persistence traversals cover reason values, errors and operand prefixes.
- db/format/writer_vm.go writes one canonical pending-reasons-v1 tail, leaving
  old reason slots empty. reader_vm.go rejects mixed state, invalid kinds and
  unprovable legacy cleanup ownership; validation checks owner containment and
  nested order. The previously reported singular-slot loss is removed.
- scripts/bench_players.py:1053 compares saved schedules before replacement.
  Its main failure record starts unsuccessful, records/rethrows BaseException,
  and deletes inputs only after actual success. This addresses the analyst's
  interruption-retention finding without weakening the schedule guard.
- engine/task_load.go aggregates suspended failures; server/server.go:276 stops
  the runtime and closes its session on such failure. Original-world harness
  errors are retained, not reclassified as success.

## Evidence inspected

No local tests or builds were launched. Existing managed XML was inspected
directly, including canonical capability admission in each session:

```text
suspended-call-toast.xml errors=0 failures=0 skipped=0 tests=2 time=24.158
native-env-toast.xml     errors=0 failures=0 skipped=0 tests=2 time=24.151
native-string-toast.xml  errors=0 failures=0 skipped=0 tests=2 time=24.164
```

Fresh hashes of the retained committed fixtures match their provenance:

```text
toast-suspended-call.db
51158BEE3D37E0B59225611E3A138DD274261E4C5993A8846F73FC74D229F0D8
toast-command-environment.db
B395EDF39A8F79B2BB5670D78573B157C1064F90F41A3538BD25301CBFF67DE1
decoded toast-string-iteration.db.b64
C3E24D9CCBE800F3FC84DB3B5B655C827204F0E501BBBE826D38AB14D427659C
```

The focused worker/analyst commands and results were read in their reports;
they are existing evidence, not tests independently rerun by this verifier.
The current control-transfer analysis records the original failing endpoints
corrected to 73, 73 and {}. Native converter/layout census, inherited command
environment, range capacity/type, named-while and cleanup findings are addressed
in the current source and task note, while old analyst sections retain history.

The raw original-world smoke console ends:

```text
--- PASS: TestMongooseRealWorkload (58.99s)
PASS
```

It also retains native home E_NACC and who E_PROPNF failures. A passing harness
does not mean those application commands succeeded or that tail lag is solved.

## Remaining acceptance work

Exact-head full PR CI and pinned Elle verdicts with nonzero path counts are
absent. No isolation, race, deployment or merge acceptance is claimed. The
planned quiescent paired workload comparison is still required before claiming
a mutex speedup on the faithfully restored original checkpoint. Native layout
oracle widths/PCs and the 70-frame census do not independently prove every
possible semantic origin/control shape; the explicit support limits remain.

## Incremental shared-wait disposition

Reviewed only the subsequent shared-gate accounting/diagnostics correction,
Windows workload guard and diagnosis claims. The earlier native corpus was not
re-reviewed. No local tests or builds were launched. NO-MERGE remains the final
merge disposition until exact-head PR CI and required pinned Elle evidence.
No new concrete source blocker was established in this scoped followup.

Both real commit paths now acquire through StoreTxn.acquireSharedCommitGate.
The helper retains the existing grant, cancellation result, lock order and
release placement; it adds successful task-row acquisitions to the same counters
and callback already used by ordinary commits. Gate exemption remains outside
the helper in both callers. Read-only and terminal fast paths retain their
existing behavior. This does not establish an isolation verdict.

The runtime observer records the appended bounded shared_commit_gate phase and
calls Scope.Waited only for a nonnil admission scope. Standalone CallVerb and
sweep-owned hooks now install that observer after actual sweep registration,
using their available task/hook identities. Task execution and suspension cleanup
install it at their existing transaction boundaries. Cancelled acquisitions are
not counted, matching existing ordinary-commit accounting.

The new source regressions exercise an actual held exclusive grant and proven
queued task-row waiter, both object-writing and task-row-only paths, exclusive
exemption, and actual public/sweep hooks. These are meaningful producer tests;
their reported race outputs were read in the coder/analyst reports, not rerun.
The Windows guard queries the current mutex sampling fraction and rejects the
two named affected release versions before heavy workload loading. The report
correctly treats the prior stalled capture as incomplete and the upstream
explanation as consistent evidence rather than a debugger-established cause.

The later paired report supersedes the earlier statement above that no pair
exists. It is one sequential instrumented pair, with changing disposable-world
state, native application failures and mixed cohort results retained. It does
not prove universal speedup or first socket output. Raw warmed console was
inspected and ends:

```text
--- PASS: TestMongooseRealWorkload (104.60s)
PASS
```

The file is explicitly console.partial.txt; complete console retention is not
claimed. Trace/block/CPU files and a manifest with input hashes/tool settings
are present beside it. Task-scoped trace observations are correctly described
as overlapping wall/synchronization/scheduling intervals, not additive CPU.

Two prose corrections were sent to the parent for its final note/report edit:
replace the living note's historical standalone-hook gap with addressed state,
and qualify admission-observer claims to admitted work because sweep hooks have
no scope. Detailed workload diagnostics already state these limits correctly.
No publishing or implementation editing was performed. An initial read-only rg
probe guessed a nonexistent admission directory; it failed without reading or
changing files, and was corrected by rg --files to internal/admission/scope.go.
