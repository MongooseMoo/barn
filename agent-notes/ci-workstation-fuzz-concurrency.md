# Workstation fuzz concurrency

## Goal

Let the fuzz matrix use available workstation runners instead of queuing behind
the fixed two-job cap. Deliver through normal merge and launch the updated run.

## State

The organization runner API shows eight online workstation listeners. The
active legacy campaign has only two fuzz jobs running while other workstation
listeners are idle. The routing helper explicitly supplies parallel=2.

## Decisions and why

Set legacy mode's existing parallel output to GitHub's 256-job matrix limit.
This admits every matrix entry and lets actual runner availability determine
concurrency without new settings or runner-discovery APIs. Preserve the dedicated
pool's separate bounded configuration and the two Go workers per target.

Update the existing routing regression and the documentation of the old cap.
The replacement campaign will preserve the original full comparison selection.
Do not cancel the independently running dedicated native-only campaign.

## Surprises

A separate change added native-only campaigns on master during this task. This
branch starts from that merged master and preserves both campaign selections.

## Next step

Run the focused routing test file, publish the independent PR, watch required
checks, merge normally, then replace only this task's old workstation campaign
and verify assignments beyond two concurrent fuzz jobs.
