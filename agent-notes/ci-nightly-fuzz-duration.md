# Nightly fuzz duration and local schedule

## Goal
Run each fuzz target for thirty minutes nightly, starting at or after 3 AM.
Q approved this plan with "Make it so!" after reducing two hours per target
to thirty minutes.

## State
Workflow uses 03:17 America/Denver, thirty-minute campaigns, a forty-minute
Go timeout and a forty-five-minute job timeout. Documentation reflects it.
Awaiting PR CI and merge; no daytime fuzz workflow dispatch is needed.

## Decisions and why
Use GitHub's schedule timezone field to follow Mountain daylight-saving time.
Keep seven targets and two concurrent jobs: four waves take roughly two hours
plus setup and queue delays. Only the first wave starts around 03:17.
The smallest change is four workflow lines and the documentation sentence.

## Surprises
No workflow audit tools were found on the Windows PATH.

## Next step
Publish the independent PR, wait for its CI, merge when its gates pass, and
remove this task's worktree and branch.
