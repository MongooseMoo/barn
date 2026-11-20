# Shared commit wait analysis

## Current disposition: direct-hook finding addressed

Read-only inspection of the actual repair confirms call_verb.go now installs
sharedCommitWaitObserver after sweep-task registration. Execution-owned entries
use ownerTaskID; sweep-owned entries use their actual registered task ID. Hook
object and verb identify the standalone entry. The common observer always emits
the phase and invokes Scope.Waited only when the scope exists, covering the
previously omitted sweep path without a nil-scope dereference.

The documentation now explicitly limits records to successful acquisitions,
identities available at the entry and no admission accounting for sweep hooks.
It states that standalone hooks may have taskID0/no commandID and that cancelled
acquires are excluded. The added focused source test drives both actual public
CallVerb property publication and private sweep ownership under production
barriers, rather than calling only the observer helper. Root reports its
combined phase/hook race run returned `ok engine1.469s`.

No remaining concrete issue found in this scoped repair. No tests or builds
were executed during this disposition review; no source edits, publication,
CI/Elle, full-change readiness or performance approval. The P2 below is retained
as historical pre-fix evidence.

## Scope

Read-only review of task-row gate-wait accounting, the separate runtime shared
commit phase, Windows mutex-profile guard and associated diagnostic claims.
No source edits, tests/builds during capture, publication or merge approval.

## State

Inspecting the uncommitted correction and its reported real-workload evidence.
Findings will be appended as checked against live source.

## P2: Shared phase coverage still omits the standalone verb-hook path

engine/task_runtime.go now installs sharedCommitWaitObserver for admitted
task slices and suspension cleanup. However engine/call_verb.go:342 still
installs only scope.Waited when a scope exists, and installs no observer for a
nil-scope sweep-owned hook. The same function commits its transaction at424.

These are real producers: public CallVerb and CallVerbWithArgstr use the
standalone path; engine/runtime.go routes no-transaction sweep/inherited host
hooks through it; waif_lifecycle.go uses it for :recycle. A direct ordinary
commit can therefore update store counters and admission accounting without
producing shared_commit_gate. A sweep-owned task-row commit updates store
counters but has no task-labelled observer.

The new helper comment says every shared commit wait is visible, and the
diagnostic documentation describes ordinary and task-row acquisitions under
their task/command without delimiting this omission. Correct the real hook
path with an appropriate identity and nil-scope handling, or explicitly narrow
the diagnostic coverage claim. This is a coverage gap, not an assertion that
the gate's locking/publication correctness changed.

## Checked source behavior

Both Commit branches reach the same acquireSharedCommitGate exactly once.
Read-only, direct, terminal and gate-exempt paths retain their previous routes.
Acquisition still precedes store/task publication locks and the same grant is
released after publication. Failed acquisitions remain uncounted, matching
the prior ordinary-commit policy; do not describe cancelled waits as covered.
The callback continues Scope.Waited and the phase is distinct from exclusive
commit_gate. The new phase is appended, preserving existing phase ordinals;
barn_logs resolves the vocabulary dynamically.

The focused regressions prove a real queued waiter before their5ms hold,
compare observer/counter equality, assert one successful acquisition and cover
both task-row-only and object-writing transactions plus exclusive exemption.
No tests/builds were executed by this analyst during the capture window.

## Mutex guard and claim checks

The guard reads the current mutex sampling fraction using the negative-rate
query and rejects only enabled profiling on Windows go1.27.0/go1.27.1 before
heavy loading. The primary upstream issue80982 documents the Windows failure;
backport issue81145 is closed against milestoneGo1.27.2. Live sources:

- https://github.com/golang/go/issues/80982
- https://github.com/golang/go/issues/81145

The diagnosis preserves the stalled capture and distinguishes consistency with
the upstream defect from a proven debugger verdict. Task-scoped trace timings
are not presented as additive CPU. One paired workload is explicitly limited,
with application failures and incomplete first-socket-output acceptance visible.
No further concrete blocker found in the scoped accounting/guard source. No
CI, isolation, full-change readiness or performance approval is established.
