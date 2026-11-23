# queued_tasks() omits tasks the caller has killed

## Goal

Match ToastStunt: after kill_task(t), queued_tasks() in the same task no longer
lists t (tasks.cc kill_task dequeues before returning).

## State

builtinQueuedTasks skips any task whose row the caller's transaction has staged
as not live; StoreTxn.StagedTaskRow reads only staged writes. Regression test
engine/queued_tasks_killed_test.go failed before the fix and passes after.
Conformance moo-conformance-tests#268 adds the two YAML cases; both pass on
ToastStunt and on this branch. server/exec.yaml::multiple_simultaneous_execs_work
also passes on this branch (3 of 3 runs with an engine profile that runs it).

## Decisions and why

StagedTaskRow instead of ReadTaskRow: ReadTaskRow records a read version for
every published row it falls back to, which would add a commit conflict on each
task queued_tasks() lists. Only the caller's own staged kills matter here.

## Surprises

Barn's CI never ran the exec test: server/exec.yaml is gated on exec_support,
forking and tasks, which Barn's profile manifest does not declare. Fixed in a
separate PR.

## Next step

Merge after CI.
