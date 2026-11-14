# Checkpoint task order

## Goal

The same world writes the same checkpoint bytes, so a round trip can be checked
by comparing files. Allowed change: make the order in which a checkpoint writes
queued, suspended and interrupted tasks deterministic, at the one place that
fixes every checkpoint. Nothing else.

## State

Done locally on branch `checkpoint-task-order`, not pushed, no PR.

- `engine/checkpoint_task_order_test.go`: 8 queued, 8 suspended, 8 interrupted
  tasks, each holding its own WAIF, all WAIFs also listed in `#0.kept`; five
  checkpoints of the same state must be byte-identical, sidecar included.
  Failed 30 of 30 runs before the fix (differs at the first queued task's id),
  passes 30 of 30 after.
- `engine/task_factory.go` `TaskSnapshots`: sorts the catalog by task id, then
  stable-sorts with the scheduler's own `taskQueue.Less` (ready time, then
  enqueue sequence).

## Decisions and why

- Where: `Runtime.TaskSnapshots` is the only production source of checkpoint
  task lists (`server/server.go` `dumpCheckpoint`, used for ordinary, panic and
  shutdown dumps). Sorting there also fixes `snapshotWithTaskRoots`
  (`db/format/checkpoint_stage.go`), which roots task values for anonymous
  objects in task order before the writer sees them. `TaskManager.Snapshot` is
  left alone because `queued_tasks()` and other scans use it.
- Order: scheduler order, not plain task id. Toast's reader puts every task
  through `enqueue_waiting`, which sorts by whole-second start time and keeps
  file order among equal times; Barn's loader enqueues in file order and the
  heap breaks equal ready times by enqueue sequence. So file order is run
  order for tasks due in the same second in both servers. Toast task ids are
  random, so id order would reshuffle those ties for a file that came from
  Toast. Id is only the last tie-break, for tasks whose keys are equal.

## Surprises

- Two checkpoints of the old code were sometimes equal by chance (Go's map
  iteration start is random, so the same rotation can repeat); the test takes
  five to fail reliably.

## Next step

Run the full CI gate on a PR when Q or the lead decides to publish this
branch.
