# fix/task-kill-resume-perms

## Goal
Q 2026-11-04: "let's build those conformance-tests first?" and "don't forget
to update the spec too". Two pre-existing Barn divergences from Toast found
while checking PR #604 (task state in the store):
1. resume() of a task waiting in read() must be E_INVARG.
2. kill_task()/resume() permission must use the programmer Toast checks, not
   the task owner.

## Toast source (read)
- tasks.cc do_resume (2783-2826): searches waiting_tasks for TASK_SUSPENDED
  (perm vs progr_of_cur_verb) and active_tqueues first_bg for TASK_SUSPENDED
  (perm vs tq->player); else E_INVARG. So read() tasks, unstarted forks,
  exec tasks and the current task are E_INVARG.
- tasks.cc kill_task (2673-2758): waiting forked -> forked.a.progr; waiting
  suspended -> progr_of_cur_verb; reading -> tq->player; bg queue -> tq->player.
- eval_vm.cc:66 progr_of_cur_verb = top activation's progr.
- tasks.cc:1300-1317 suspend: indefinite suspend goes to waiting_tasks.

- execute.cc bf_task_stack (3747-3768): find_suspended_task (tasks.cc
  2610-2647: waiting/bg TASK_SUSPENDED, reading, external) else E_INVARG;
  perm vs progr_of_cur_verb. Unstarted forks (TASK_FORKED) are E_INVARG.

## State
- Conformance: moo-conformance-tests PR #170 (branch tests/kill-resume-perms,
  worktree ../mct-task-perms), 4 tests in builtins/task_management.yaml.
  Stock WSL Toast: 4/4 + capability_admission pass. Barn master (with #604):
  3 fail (two perm cases give E_PERM; resume() wakes the read() task), the
  converse perm case passes. The commit also drops a stray CR at the end of
  one existing comment line (the edit tool normalized it).
- PR #170 now 6 tests: + task_stack_of_unstarted_fork_is_invarg and
  task_stack_perm_follows_suspended_verb_programmer. Toast 6/6.
- Spec: spec/tasks.md 6.2 and 7.1, spec/builtins/tasks.md 2.2 (also fixed
  its wrong argument names: 2nd = line numbers, 3rd = variables), 3.2, 3.3.
- Barn fix (this branch), 6/6 green locally against the #170 branch:
  - engine/task_rows.go ownEndRow: suspended row Owner = Context.Programmer
    (top frame's programmer); read() -> Running row owned by ReadingPlayer;
    read_http()/exec() -> Running.
  - engine/task_manager.go registrationRow: same rules; fork -> t.Programmer.
  - engine/task_factory.go: fork child row Owner = forking programmer.
  - task/task.go: `started` flag, ForkNotStarted().
  - builtins/tasks.go task_stack: E_INVARG for unstarted fork; perm vs top
    frame's programmer.
  - Regression test engine/task_rows_owner_test.go.
- Remaining divergence (no test): kill_task() of a read_http() task should
  check the connection (tq->player); Barn records no connection on the task,
  so its row owner is the suspending programmer.
- Merge order: Barn CI runs conformance from main HEAD, so merge #170, then
  push the Barn PR so its CI includes the new tests, then merge it.

## Surprises
- First version of the converse test passed 0 on Toast: a fork made inside
  another principal's verb sits in that principal's queue and had not run
  when the parent came back from suspend(0), so Toast checked
  forked.a.progr. Tests now wait until task_stack(t) is a LIST.

- Barn's task_stack() returned a stack for an unstarted fork, so the tests'
  wait loop exited at once on Barn and the kill hit the unstarted fork (row
  owner = forking programmer): the perm results looked inverted until
  task_stack was fixed.

## Next step
Merge #170 when green, push this branch, PR, watch CI, merge.
