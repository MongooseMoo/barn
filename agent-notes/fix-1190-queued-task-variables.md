# Queued task variables

## Goal

Fix field 11 of queued_tasks(1) for suspended and reading tasks and open pull requests for the implementation and its conformance coverage.

## State

The VM captures the top frame's locals at yield. The old queue row used ForkInfo.Variables alone. The investigation reproduced an empty map for foreground suspend and read, and a stale creation-time value for a fork that changed a local before suspending. In all three cases task_stack returned the current value. The queue row now reads RuntimeVariableMap from the selected activation when its locals are available.

## Decisions and why

Use the activation already selected for the queue row, including its slice-start view when running. Keep the creation environment for a fork before its activation has captured locals. The separate field 2 mismatch for read waiters is outside this issue.

## Surprises

The managed stock profile skipped both first-draft cases because they used the fork feature. The conformance fixture supports a second input connection, so the regression can observe a waiting foreground task on both engines without depending on fork support.

## Next step

The companion conformance test reads the same task's marker first in read() and again after input moves it into suspend(). The test and admission passed on stock Toast and fixed Barn. Baseline Barn failed the first lookup with E_RANGE. The Go regression runs a fork that changes a marker before suspending. Land the Barn fix before the companion suite.
