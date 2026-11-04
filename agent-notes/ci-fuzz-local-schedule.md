# Nightly fuzz scheduling

## Goal
Start the existing thirty-minute fuzz campaigns at night, after 3 AM Mountain.

## State
The Windows task is installed and inspected: daily offset-free 03:17 boundary, next run November 5 at 03:17, hidden PowerShell action, existing Q interactive principal, wake enabled and late catch-up disabled. Reporting is independent PR #594. Workflow actionlint passed; PR CI pending.

## Decisions and why
The smallest effective repair replaces GitHub cron with one Windows scheduled task that dispatches the existing workflow daily at 03:17. GitHub created the November 4 scheduled run at 18:00:27 UTC and its runners began three seconds later. Earlier scheduled runs also arrived many hours late. The valid timezone setting does not prevent GitHub schedule-event delays.
Use the same Q interactive principal as the eight existing Barn runner tasks and the existing GitHub CLI Windows-keyring login. No new credential or scheduler service is needed. Disable late catch-up; allow waking the workstation. The installer is about 25 lines and records the exact task configuration in the repository.

## Surprises
Windows is configured for Mountain time with daylight saving. The task's local-time trigger therefore follows America/Denver seasonal clock changes. Q must remain logged on, which is already required by the existing Barn runner tasks.

## Next step
Publish the independent PR, await CI and merge after reporting PR #594. The first actual nightly dispatch remains unobserved.
