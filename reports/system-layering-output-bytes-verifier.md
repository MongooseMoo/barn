# Output byte accounting final verification

## State

Verification is complete for actual open PR #636, Count buffered output in bytes.
The published head matches the assigned review head. Future uncommitted
TaskYielder and tool-cleanup work is excluded from this review.

Final raw PR status:

```text
Build & unit tests: SUCCESS (1m34s)
Serializable histories and negative control: SUCCESS (4m15s)
MOO conformance suite: SUCCESS (10m44s)
Both Record pass jobs: SUCCESS
Both Check for an earlier pass jobs: SUCCESS
mergeStateStatus: CLEAN
mergeable: MERGEABLE
```

Full conformance is complete. This verifier inspected the published diff,
mutation coverage, framing, existing evidence and live review state.
No source edit, local broad test, publication or merge was performed.

## Completed source and evidence inspection

No concrete defect found. Production queue mutations are limited to the two
append sites and flushLocked in server/connection.go. Both appends add captured
notification bytes under the existing mutex. Flush subtracts only successfully
written notifications; the failed entry and suffix retain their byte total.
Delivered slots are still cleared, and the getter remains O(1) under that mutex.
Validation precedes append/increment. No output ordering or draining policy
changes. The real notify producer decodes binary payloads and captures newline
framing before deferring effects. Pending effects retain their existing order
and use the same byte calculation, filtering by recipient.

The edited regressions cover exact byte counts, first/middle/last failures,
retry suffixes, UTF-8, empty payloads, captured option changes and other recipients.
Managed XML was read directly:

```text
toast-byte-v2.xml: tests=5 failures=0 errors=0 skipped=0
barn-byte-before.xml: tests=5 failures=4 errors=0 skipped=0
barn-byte-after.xml: tests=5 failures=0 errors=0 skipped=0
reviews: 0
reviewThreads.nodes: []
```

Read results.json and seed-412 manifest directly from the preserved Elle tar:

```text
healthy verdict true: 6
fault-control verdict false: 20
seed-412 list-append paths:
exclusive=357 exclusive_gate=460 fork=14 hot_renewal=4 native_exception=24
```

The artifact contains raw histories, manifests and checker/witness outputs.
No measured speedup or performance non-regression is claimed. The connection
queue getter remains constant time; its existing mutation sites now maintain
the necessary total. Pending-effect summation retains its existing linear scan.

## Verdict: MERGE

The implementation corrects the confirmed count-versus-byte behavior in its
existing owners, maintains its cached-total invariant at every production
mutation, and preserves acceptance, effect and transport order. Tests cover
both accumulated totals and failure suffixes. The analyst has no unresolved
major/blocker issue. All seven checks pass at the assigned published head;
live reviews and review threads remain empty, and GitHub reports CLEAN and
MERGEABLE. The preexisting queue-capacity/refusal policy and later output
ownership work are outside this bounded verdict.
