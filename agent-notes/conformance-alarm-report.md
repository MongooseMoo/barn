# Conformance alarm report

## Goal
Explain a weekly runtime alarm from the exact sampled jobs and their JUnit artifacts. Test the report against issue #1180 and deploy through the merge queue.

## State
Implemented runtime attribution in the alarm and exercised its issue-building path using the exact fourteen #1180 jobs and archived XML reports. The emitted report recovered +144.58 seconds per run, including +115.58 for newly seen cases, +24.52 for previously seen cases and +4.48 outside case timers. The focused alarm tests passed, including unequal weeks, removals, duplicate case IDs and unavailable reports.

## Decisions and why
Enrich runtime alarms when reporting them. Show links, runner names, durations and artifact coverage. Attribute the difference between weekly means to new cases, previously seen cases and time outside the case timers. An unavailable report cannot stand in for a zero-duration run.

## Surprises
The existing issue remains open, so normal duplicate suppression applies to the deployment run.

## Next step
Publish the tested change through Barn CI and verify the alarm workflow on master.
