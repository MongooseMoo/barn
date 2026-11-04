# ci/skip-tested-tree

## Goal
Q 2026-11-03: "if the head is precisely the tested head, CI doesn't rerun."
Q said "make it so".

## State
- Built: .github/actions/tested-tree (check/record), wired into ci.yml and
  elle.yml. PR opened against master; verifying live that the merge queue run
  skips.

## Decisions and why
- The same tree gets tested up to three times: pull_request (refs/pull/N/merge),
  merge_group (same parents, so the same tree when master hasn't moved), and
  push on master (fast-forward to the queue commit).
- Key = Git tree hash; ci.yml adds the moo-conformance-tests main SHA, read
  once in the `tested` job, and the conformance checkout is pinned to that SHA
  (it was `ref: main`) so the record names the suite commit that was actually run.
- A record counts only if `workflow_run.head_repository_id` == the repository
  ID: a fork PR runs its own workflow file and could upload any artifact name.
- Skipped jobs report Success and don't block required checks (GitHub docs,
  "Using conditions to control job execution"), so the required job names
  stay the same.
- Elle: workflow_dispatch always runs.
- The API lookup uses python3, which the runners already have (bench tests);
  gh and jq on the self-hosted runners were not verified.

## Surprises
- PR #588's queue run was the motivating case: identical tree re-tested.

## Next step
- Read the PR's CI. On merge, confirm that the queue and master runs show
  "already passed on this tree: true" and that the required jobs were skipped.
