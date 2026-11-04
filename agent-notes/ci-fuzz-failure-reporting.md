# Nightly fuzz failure reporting

## Goal
Open issues automatically for nightly fuzz failures, then repair scheduling and the saved failures.

## State
Reporting implemented; PR CI pending. Issue #593 records both existing snapshot failures with their run and artifact links. Existing workflow actions are pinned to their verified tag commits and checkout credentials are not retained.

## Decisions and why
The smallest reporting change is about 35 lines in the existing workflow: grant the fuzz job issue-write permission, identify a failed fuzz step, and create one issue per target with run and artifact links. Repeated failures append their run links to the same open issue rather than creating duplicates. Setup failures get their own issue title so they are not described as fuzz discoveries. Keep the existing artifact upload.
The compiler finding from the first manual campaign already has its input committed and was repaired by merged PR #576; do not file it again as an unresolved defect.
Scheduling investigation and snapshot repair proceed independently in separate ownership boundaries.

## Surprises
The latest scheduled run was created at 11 AM Mountain; runner jobs started immediately. The schedule delay requires investigation at the trigger rather than the runner.

## Next step
Implement reporting, create the issue for existing snapshot failures, inspect PR CI, and merge before the scheduling repair.
