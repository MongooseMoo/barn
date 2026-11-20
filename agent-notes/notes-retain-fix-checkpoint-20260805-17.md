# Retain/fix checkpoint 17

## Completed

- moo-conformance-tests PR #75 normal-merged as `49f08f5b8b025109ab9885ea90f60756b0faae16`, verified two parents; issue #71 closed.
- PR #99 freshness audit: mechanically correct Go 1.24 hygiene, update after #115, normal-merge master, preserve corrected conformance workflow while adding format/vet gates.
- PR #94 freshness audit: behavior/scope correct for #93, update after #115, correct inaccurate candidate ancestry claim, rerun corrected default conformance and independent review.
- PR #110 freshness audit: behavior correct but architecture BLOCKED because delete_verb duplicates the owner/wizard/flag predicate instead of satisfying issue #102's required single store-owned authority rule.

## PR #96 current state

- Exact remote head: `cc436313d892cba6b6e2cb6385849e9a360f0392`, draft, literal `Closes #95`.
- Serialization plan includes persistent values, pending finalizations, and all task-persisted values; managed Anon7 passes on repaired Barn branch evidence.
- Scheduler owns refcounted physical execution leases across full `runTask` mutation/handoff, independent of logical task state.
- Explicit global GC fails closed around active siblings or duplicate same-ID caller execution; per-task GC skips active siblings; deferred GC waits for quiescence.
- Latest lifecycle repair moves deferred settlement to the singular post-lease `runTask` boundary for every caller, including synchronous callers and recovered panics.
- Red prior head: direct synchronous runTask left orphan valid; exact CI grew from 82 to 86 failures due four waif lifecycle failures.
- Green local: focused lifecycle/GC x20, full repo, scheduler race 252.282s, build, vet, gofmt, diff-check.
- GitHub build/unit green at cc436313; legacy conformance rerun in progress. Final independent exact-head review in progress.

## Next

1. Require CODE APPROVE at `cc436313` and confirm legacy rerun returns to the known 82 base/integration failures.
2. Normal-merge corrected PR #96 history into PR #115; do not merge #96 alone.
3. Run full corrected default-profile conformance on PR #115, now including merged Anon7; expected current total is one more passing test than prior 11,175/410 result.
4. Independent integration review, exact audit, ready, normal GitHub merge, two-parent verification, issue closure verification.
5. Update #99, then repair/update #110's single-owner authority rule, then begin Toast-oracle/conformance work for critical issue #13.

