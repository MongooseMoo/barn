# Decision after gaming out scheduling with Fable

Status: design recommendation, not an implemented policy or a measured speedup.
This synthesis takes precedence over tentative recommendations in the two
review transcripts. Source baseline: Barn `ef5fcb0`.

Claude Fable was consulted in two rounds in session
`e01c6c16-a9e2-4085-9279-9c5ac05c7673`:

```text
Round 1: subtype=success is_error=False num_turns=53 duration_ms=728916
Round 2: subtype=success is_error=False num_turns=11 duration_ms=285251
```

The first round mapped source and proposed an exclusive arbiter plus
abort-and-retry on boundary contention. The second challenged resource ordering,
consumed builtin operands, GC roots, cancellation, nested calls, and shared
commits. Fable withdrew abort-and-retry as the default: replay can lengthen the
exclusive hold from a tail to a whole slice and amplify contention.

## Chosen direction

Make the commit gate a scheduled resource, coupled to execution admission.
Keep the existing serial background scheduler lane and concurrent connection
lanes initially. Do not add more parallel background dispatch before accounting
for gate contention. This preserves today's concurrency shape; it is not a
claim that the new policy has zero throughput cost.

One gate abstraction should own every raw gate access. Ordinary commits retain
shared access, now attributed and timed. Resumed/nonretryable slices, effect
boundaries, and checkpoints request exclusive access through the same policy.
The shared path is observed, not automatically serialized or subject to the
same priority ranking. Direct live-store mutation and quiescent recycle hooks
still rely on their existing lifecycle rules; do not claim the gate governs
them merely because the mutex has been wrapped.

For an exclusive slice whose need is known before it runs, wait before taking
the physical execution lease. Do not grant ownership to an unavailable worker.
The serial scheduler lane should reconsider its eligible head candidates as
new work arrives and bind the selected task at handoff, preserving same-queue
ordering. Connection lanes retain their per-connection input order. Admission
also has to coordinate with VM-start/GC exclusion: owning a goroutine is not
proof that a task can acquire its execution lease immediately.

At an effect boundary inside a running builtin, retain the Go continuation and
execution lease while waiting. Do not fake a MOO suspension: builtin arguments
may already have been consumed, and the current suspend flow resumes after the
call. The retained lease protects roots the VM walker cannot see. Keep current
read-set validation before the effect and current retry behavior on an actual
conflict. Do not replay a whole prefix merely because the gate was occupied.

This deliberately leaves a GC-liveness risk for mid-builtin waits.
The same cost exists today; it must not be hidden behind a claim that all gate
waiters are root-free. Trace deferred-finalization age and pending work. A
scheduler lane parked at a boundary also cannot service a later continuation
until that call progresses: exclusive-grant fairness alone does not eliminate
all head-of-line blocking.

## Contracts needed before implementation

1. **Grant lifetime.** Distinguish `queued`, `offered`, `claimed/running`, and
   `released`. Claim versus cancellation must be linearized. Cancellation may
   revoke an unclaimed offer, but must never unlock an active holder. Once
   started, the owning unwind path releases exactly once; cancellation remains
   cooperative and subject to existing nonpreemptible regions.
2. **Inherited ownership.** A nested call under an inherited execution lease
   must carry the appropriate grant capability/transaction relationship, rather
   than reacquire shared access against its own exclusive hold. Do not grant
   exemptions solely from an unauthenticated task-id equality check.
3. **Maintenance and lock order.** Cover checkpoint access in the migration.
   Validate gate/VMStartMu/SweepMu ordering against every nested hook. Never
   block while holding the arbiter's internal mutex. Do not move a GC wait
   behind an exclusive grant if collection needs that grant to make progress.
4. **Finite preference.** Bound foreground grant bursts and age background and
   maintenance requests. An age threshold changes eligibility/priority; it is
   not a deadline guarantee. Same-class FIFO and older aged requests still
   contribute backlog. A class-level progress bound is not a per-task bound.
5. **Accounting.** Record ready wait, lease wait, exclusive acquisition wait,
   shared-commit wait, execution/retry occupancy, gate hold, external wait, and
   finalization delay separately. Keep separate execution and gate ledgers
   initially; do not silently double-charge elapsed time using an unvalidated
   combined weight. Scheduler accounting is distinct from MOO tick/seconds quotas.

The reviewers' sketches are not executable lock protocols. In particular,
their shorthand `Cancel(granted) -> Release` requires the claimed/running
distinction above. Likewise, "one residual hold" is a useful counterexample
floor, not an unconditional upper bound on request latency.

## Why both ready and gate scheduling remain necessary

The previously captured fixed-run trace contains:

```json
{"task_id":2118589452,"queue_wait":10078709500,"ready_to_vm":10078709500}
```

These nanosecond durations are equal at recorded resolution, so this sample's
measured delay is before dispatch, not additional gate/lease wait. Other login
phases were not similarly decomposed. Keep source-proven gate serialization in the
design without rewriting this measured ready-queue delay as a gate-wait result.

Current production forks and saved VMs use solo worker batches. The immediate
ready-queue problem is draining a long snapshot, not overly wide worker batches.
Reconsider after a task and wake promptly on completions; a gate-only refactor
must not leave this separate source finding out of its acceptance criteria.

## What the policy cannot fix

An active exclusive holder cannot safely surrender ownership at an arbitrary
instruction. A long post-effect computation sets a latency floor. Synchronous
network I/O under the hold can make that floor much larger. Barn's `curl` calls
`client.Do`; Toast routes curl through `background_thread`. Converting such
operations to real suspension points is a separate oracle-verified behavior
change, including thread-mode semantics. No trace here proves curl caused the
observed Mongoose login delay.

No numerical latency or throughput guarantee has been established. In
particular, retain early exclusive admission for nonretryable resumed slices:
removing it requires a restartable continuation/snapshot representation or
another proved isolation mechanism, not a queue-policy adjustment.

## First implementation slice and follow-ons

The first coherent slice comprises the gate abstraction, complete raw-access
migration, gate-aware admission for known exclusive work, cancellable wait/claim
protocol, inherited-grant handling, between-task ready reconsideration, bounded
class preference, and the trace points above. Keep mid-builtin waits physically
rooted. Validate cancellation at each transition, nested ownership, shared
commit progress, checkpoint/GC interaction, FIFO visibility, and sustained
foreground/background progress. Check source-derived behavior against Toast;
retain the existing conformance expectations and meaningful 1p/16p workload gates.

Then choose using traces:

1. Remove proven blocking I/O from gate-held slices with Toast-compatible async
   builtins and explicit completion handling.
2. Add programmer-level fair selection and service accounting, plus bounded
   interactive lineage. This is where share weights become useful.
3. Consider restartable resumed slices and greater parallel dispatch. In-flight
   limits become part of that resource model, not a substitute for gate policy.

## Artifacts and reproduction

- First review: [Fable round one](fable-scheduler-gate-20260918.md).
- Challenge and retractions: [Fable round two](fable-scheduler-gate-followup-20260918.md).
- Source audit: [Barn/Toast scheduler comparison](barn-toast-scheduler-source-audit-20260918.md).
- Invocation and resume workflow: [Fable consultations](../docs/fable-consultations.md).

Both prompts are retained under `prompts/`. `scripts/consult-fable.ps1` fixes the
model to `fable`, records the session, handles single-result and verbose-array
JSON output, and supports follow-ups. The original wrapper tripped on the array
shape after round one's successful CLI exit; round two exercised the corrected
wrapper and resume path successfully. No runtime implementation or conformance
tests were changed or run during this consultation.
