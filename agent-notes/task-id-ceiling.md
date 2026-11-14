# Task id ceiling

## Goal

Every task id Barn hands out or writes to a checkpoint stays within 1..2147483647
and never collides with a live queued or suspended task, so a Barn checkpoint
stays loadable by ToastStunt. Nothing else.

## State

`Runtime.newTaskID` (`engine/runtime.go`) wraps its counter to 1 after
`math.MaxInt32` and skips ids the task catalog still holds. Test:
`engine/task_id_ceiling_test.go` (fails on the old allocator with ids
2147483648 and 2147483649; passes now). Pull request open.

## Decisions and why

- Ceiling: ToastStunt reads every saved task id with `%d` into a C `int`
  (`tasks.cc` `read_task_queue`) and draws ids from `lrand48` (`new_task_id`),
  so 2^31-1 is the largest value both sides agree on.
- Kept counting up rather than copying Toast's random ids: smallest change, and
  existing tests that pin counting from a restored id still hold.
- Collision check uses the catalog (`TaskManager.GetTask`), which holds every
  loaded queued and suspended task. `registerTask` replaces a same-id task
  silently, which is why the skip matters after a wrap.
- Direct tasks in `Runtime.leasedTasks` are not checked: their ids come from the
  same counter, so they can only collide after a full 2^31 wrap during one run,
  and taking `s.mu` inside `newTaskID` risks lock inversion.

## Surprises

- ToastStunt's `new_task_id` does not check live tasks; it only rejects 0.
- Barn's writer (`db/format/writer_task.go`) prints int64 ids unchecked.

## Next step

CI on the pull request.
