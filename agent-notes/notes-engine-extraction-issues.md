# Engine extraction issue filing checkpoint

## Current findings

- `engine.Runtime` and `task.Manager` currently maintain separate task membership maps.
- The clean first package extraction is the narrow scheduler: task ready-time queue, queue sequencing, batch planning, worker dispatch/join, and result ordering.
- `engine/access_footprint.go` is dormant because every task footprint is currently unknown; deletion belongs in the scheduler extraction rather than a separate issue.
- Execution leases, VM-start/sweep barriers, context provenance, deferred WAIF/anonymous collection, and shutdown-root publication form one finalization invariant, but need consolidation inside `engine` before a package move.
- Open #173 owns WAIF liveness/statistics semantics and #170 owns durable WAIF identity; the architecture campaign must preserve those boundaries.

## GitHub state

- #175 is the open umbrella issue.
- #176 is the open sole-task-catalog child.
- #177 is the open private-scheduler child, blocked by #176.
- #178 is the open coordinator-consolidation child, blocked by #176 and #173.
- #179 is the open private-finalization-package child, blocked by #178 and #173.
- GitHub `addSubIssue` relationships were added from #175 to all four children and read back successfully through GraphQL.
- Each child body links to #175, states its dependency/constraint relationships, requires focused/race/full Go verification plus managed conformance, and requires a literal `Closes #N` PR phrase.

## Observations

- The hierarchy deliberately does not create a separate footprint-deletion issue.
- #64 remains a separate task-locking defect and is explicitly excluded from the architecture issues.
- The old broad `scheduler` package is not being restored; #177 creates only `engine/internal/scheduler`.

## Blocker

- None for issue filing. Implementation order is constrained by the dependencies recorded above.

## Next action

- Perform a final live read-back of #175-#179 and report the linked issue set to the user.
