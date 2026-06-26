# Fix F28 — queued_tasks() sort order

## Toast's TRUE order (source is authority; WSL oracle down)

`queued_tasks()` returns tasks **ascending by start time (earliest start time
first)**. The review's "ascending (oldest first)" expectation was **CORRECT**.

### Source citations (`~/src/toaststunt/src/tasks.cc`)

- `bf_queued_tasks` (tasks.cc:2496) builds the result list purely by iterating
  the task queues. It applies **no sort of its own**. Iteration order:
  1. idle_tqueues reading tasks (2546-2551)
  2. active_tqueues reading + first_bg ready tasks (2553-2569)
  3. **waiting_tasks** forked/suspended tasks (2571-2581)
  4. external_queues (2585-2586)
- The forked/suspended tasks that `queued_tasks()` reports come from
  `waiting_tasks`, which is kept **sorted ascending by start_tv** in
  `enqueue_waiting` (tasks.cc:1182-1205): a new task is inserted before the
  first existing task whose start time is strictly greater
  (`timercmp(start_tvp, GET_START_TIME(...), <)`, lines 1193 & 1200). Equal
  start times preserve insertion order (FIFO).

So the dominant queued-task case (forked/suspended waiting tasks) comes out
earliest-first = ascending by start time.

Per-task tuple shape (unchanged): `{task_id, start_time, x, y, programmer,
verb_loc, verb_name, line, this}` — Toast's `list_for_vm`/`list_for_*`
(tasks.cc:2398-2453). Not perturbed.

## The bug

`builtins/tasks.go` `builtinQueuedTasks` sorted with `StartTime.After()`
(descending, newest first) — inverted vs Toast.

## The change

- `builtins/tasks.go`: flipped comparator `StartTime.After` → `StartTime.Before`
  (ascending), with a Toast-source citation comment. `sort.SliceStable`
  preserves insertion order for equal start times, matching Toast's FIFO tie
  behavior. Tuple shape untouched.
- `builtins/review_io_test.go` `TestReview_IO_QueuedTasksSortOrder`: kept the
  ascending assertion (already Toast-true) and rewrote the header comment to
  cite the Toast source authority instead of the review.

Note: Barn's `Manager.GetQueuedTasks` iterates a Go map (random order), so the
explicit sort is the sole determinant of output order — the comparator flip is
the complete fix.

## Gate output

```
go test ./builtins/ -run 'QueuedTasks|queued_tasks|Tasks' -v
=== RUN   TestReview_IO_QueuedTasksSortOrder
--- PASS: TestReview_IO_QueuedTasksSortOrder (0.00s)
PASS
ok  	barn/builtins

go vet ./builtins/   # clean, no output
```

### Before vs after (full `go test ./builtins/...`)

- Before: `TestReview_IO_QueuedTasksSortOrder` FAILED (descending order).
- After: it PASSES.
- Unrelated pre-existing red tests still fail (NOT touched, different file
  `review_data_test.go`): `TestReview_Data_IsMemberStrCaseSensitiveBug`,
  `TestReview_Data_PcreMatchEmptySubject`. No NEW failures introduced.

## Commit

<filled after commit>
