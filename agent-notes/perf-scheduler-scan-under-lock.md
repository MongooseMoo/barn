# Scheduler scans under the scheduler lock

## Goal

Q 2026-11-20: "Please diagnose and fix the goodput issue". The Goodput series
job went red: at 16 players commands took up to the 10 s completion timeout and
throughput fell to about 200 a second.

## Cause

`engine/scheduler.go` `NextWake`, `blockedReadiersLocked` and `readyLocked`
each walked the ready-time heap, the pending list and the whole catalog while
holding the scheduler's mutex, and took every visited task's lock. When a world
queues several hundred delayed forks, every dispatch and every wake computation
paid for all of them. Measured on the pinned dump with a mutex profile: the
mutex was held 2.3 to 3 s of every 5 s, goroutines waited 57.6 s per 20 s on it,
and the eight cores sat about 37 percent idle.

## Fix

- A task records its heap keys (order and due time) in an atomic mark when it
  enters the ready-time heap, and the mark is cleared when it leaves.
- Heap walks prune by key: a subtree whose root is later than the cutoff is not
  visited.
- Catalog loops pass over a task that is in the heap and not yet due without
  taking its lock.
- `NextWake` returns at once when every worker is busy.

No scheduling rule changed. The fix relies on: a task's heap keys do not change
while it is in the heap; a task's ready deadline is zero or no earlier than its
order key; a heap task that is not yet due has never been dispatched.

## State

- Test: `TestSchedulingScansDoNotLockDelayedForksThatAreNotDue` in
  `engine/scheduler_scan_test.go` fails without the fix (timeout) and passes
  with it.
- Local harness, quiet machine, alternating builds of the same tree, 30 s
  measure: 16 players 423, 436, 433 a second with the fix against 221, 223, 203
  without; 1 player 79, 73, 73 against 48, 46, 49; slowest command 4.6 to 6.1 s
  against 7.7 to 8.5 s. No failed command in any of the six runs.
- With the fix the harness uses 87 percent of its cores (61 before) and mutex
  waiting is 22 s per 20 s (63 before); the rest is charged to Dispatch.

## Surprises

- Two earlier explanations were wrong and were disproved by measurement: that a
  specific merge caused it, and that the dump's simulation ran on the real
  clock.
- The burst of delayed forks does not happen in every run; a run that measures
  before it shows about 600 a second.

## Not done, not known

- CI measured 575 to 764 a second at 16 players on the morning of 2026-11-19.
  The fix reaches 423 to 546 locally. That gap is unexplained.
- Killed tasks stay in the heap until their start time passes. They cost no
  scan time now but hold memory.
- Whether the Goodput series job is green with the fix is known only once it
  runs on master.

## Next step

Watch the first Goodput series run on master after the merge.
