# Checkpoint
- Finding: protected `master` requires a PR; the change implements general MOO behavior, not Toast-specific behavior.
- Current state: `fix/toast-movement-login-hooks` has two rewritten commits with MOO terminology and a durable Toast-oracle naming rule.
- Blocker: none; the earlier detached review was stopped before the terminology repair.
- Next: verify the repair, rerun detached review and proportional gates, then push the branch and open the PR.

## Checkpoint 2
- Finding: managed ToastStunt proves MOO resumes move hooks, propagates both hook errors, and uses the post-`accept` source location.
- Current state: the review's error-handling finding is rejected; continuation and post-`accept` location remain valid defects.
- Blocker: none; implementation research is focused on using the owning VM call stack rather than a disposable nested VM.
- Next: add red regressions, implement the bounded repair, run the managed Barn rows and local gates, then publish the PR.

## Checkpoint 3
- Finding: focused Go tests and the managed Barn session reproduce exactly two failures; hook-error propagation already matches MOO.
- Current state: move now captures its source after `accept` and can delegate lifecycle work to the owning VM.
- Blocker: the VM continuation, suspended-task snapshot fields, and database metadata round trip are not yet implemented.
- Next: finish that persistent continuation path, make both red Go tests green, then rerun the identical managed rows.

## Checkpoint 4
- Finding: the repair preserves hook continuation and post-`accept` location while retaining MOO's measured error propagation.
- Current state: exact head `728f33c` passes formatting, vet, Staticcheck, build, full Go tests, and the five-row managed conformance gate.
- Blocker: none; `uv.lock` remains unchanged under the new frozen harness guard.
- Next: fetch and audit the exact base/diff/commit list, push `fix/moo-movement-login-hooks`, and open the PR.
