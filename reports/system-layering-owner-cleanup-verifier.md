# Existing-owner cleanup final verification

## State

Final verification is complete for actual open Barn PR #638. Published source at
the assigned head is the review target; future uncommitted file-grep, frame,
checkpoint and hash changes are excluded.

The verifier inspected EvalSession closure/error handling, CLI exit behavior,
late-bound transaction release, analyst findings, terminal CI, explicit Elle
evidence and live reviews. No source edit, local broad test, publication or merge.

## Source findings

No actionable defect found. EvalSession.Close delegates to existing Runtime.Stop
before builtin Session.Close. The latter removes and closes file/SQLite handles,
cancels/waits for active SQLite operations and joins native close errors.
Expression/file helpers preserve earlier errors and join closure errors through
their named return. CLI cleanup reports a closure error and sets exitFailed after
either batch or REPL result. Every production NewEvalSession caller consumes the
new return value. Sequential repeated closure uses the existing owners' behavior.

The standalone hook defer reads the current ctx.StoreTxn at exit. CommitAndRenew
releases its predecessor and installs a new snapshot, so the closure releases the
replacement instead of only the original registration. Release is nil/direct-safe
and idempotent. Sweep suspension saves VM/context; ordinary resume releases the
previous transaction and begins a fresh snapshot before execution. The added
defer does not alter saved frames or replace transaction payloads. It executes
after commit/effects and VM handoff, before outer execution ownership release.

Regression coverage retains transaction pointers with Go finalization disabled,
checks zero active readers and preserves results/published writes across read,
write, native exception and ordinary GC-renewal hooks, plus real sweep hooks.
Real eval file/SQLite handles are valid before closure and invalid afterward;
repeated closure is exercised. Cleanup-error injection and a newly focused
suspended-sweep test are absent; those paths were traced through existing code.
No native error is hidden by this change, and those test limits remain explicit.

## Exact-head CI, isolation and reviews

```text
Build & unit tests: pass (1m15s)
MOO conformance suite: pass (10m43s)
Serializable histories and negative control: pass (4m11s)
Both Check for an earlier pass jobs: pass (7s)
Both Record pass jobs: pass (8s)
mergeStateStatus: CLEAN
mergeable: MERGEABLE
reviews.nodes: []
reviewThreads.nodes: []
```

Downloaded the pinned Elle workflow artifact from run 42362523432 to
.tmp/system-layering/elle-owner-pr/elle-evidence.tar.gz and parsed it directly:

```text
healthy verdict true: 6
fault-control verdict false: 20
seed-412 list-append paths:
parallel=484 exclusive=357 exclusive_gate=460 hot_renewal=3
fork=14 suspend=241 native_exception=24 verb_memo=479
raw EDN histories=46 checker logs=26 witness files=105
```

## Verdict: MERGE

The changes release resources through their existing owners, expose cleanup
failures and repair the current-snapshot registration leak without changing
publication semantics. The analyst has no unresolved major/blocker finding.
All seven checks pass at the assigned published head; explicit checker verdicts,
negative controls and nonzero path counts are preserved. No measured performance
result is claimed. Future uncommitted slices are outside this bounded verdict.
