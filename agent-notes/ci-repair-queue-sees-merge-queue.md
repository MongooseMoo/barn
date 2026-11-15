# The repair queue step sees the merge queue

## Goal

Q 2026-11-15 ("Make it so"): Claude fix run 43743116517 failed at "Queue the
repair only when every check allows it". Find whether the step refused
correctly or is defective; fix it.

## What happened (verified from run logs and PR timelines)

- 43743116517 was issue #849, repair #850: in scope, Toast step success,
  outcome `test`. The step printed "Pull request #850 passed every check but
  its merge was not armed" and exited 1.
- #850's timeline: added_to_merge_queue by claude[bot] 18:09:27, merged
  18:21:16. The queue step ran 18:09:41-18:09:45, while #850 sat in the queue.
- Same on 43648919626 (#839, #840: queued 23:06:00, step 23:06:12) and
  43604679634 (#830, #831: queued 14:43:14, step 14:43:26). All three merged.
- master has an active merge-queue ruleset. The repairing run waits for its
  checks (prompt step 6), so the PR is clean when the second Claude run calls
  enablePullRequestAutoMerge, and GitHub enqueues it directly; no auto-merge
  request is left, and REST `.auto_merge` is null.
- The step read only `.auto_merge`, so it called a queued repair "not armed".
  Its disarm only called disablePullRequestAutoMerge, which cannot take a PR
  out of the merge queue.

## State

claude-fix.yml queue step:
- Reads `state`, `isInMergeQueue` and `autoMergeRequest` via GraphQL (query
  checked read-only on #850: `MERGED false false`; on an open armed PR:
  `OPEN false true`). Armed = auto-merge request, in queue, or merged.
- Disarm: dequeuePullRequest for a queued PR, disablePullRequestAutoMerge for
  an auto-merge request; a merged PR with a reason is an error (it cannot be
  undone). Label and comment are posted before the disarm so a failing disarm
  still leaves the reasons on the PR.
- actionlint 1.7.12 passes (shellcheck not installed locally).

## Unverified

- That GITHUB_TOKEN may call dequeuePullRequest (DequeuePullRequestInput.id
  is the PR node id, checked by introspection). No run has disarmed a queued
  repair yet.
- permission_denials_count was 1 in 43743116517's second Claude run; the
  transcript is deleted with the job, so what was denied is unknown.

## Next step

Watch the next clean repair: the step should end "has merged" or "is in the
merge queue".
