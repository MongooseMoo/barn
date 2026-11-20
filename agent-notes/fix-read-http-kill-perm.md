# kill_task() of a read_http() task checks the connection's player

## Goal
Last divergence left from the task-permission work (Barn #605, conformance
#170; see agent-notes/mongoose-workload-gap.md): Toast's kill_task() checks a
task waiting in read_http() against the connection's player (tasks.cc
kill_task, the idle_tqueues/active_tqueues loops, `owner != tq->player`;
make_http_task sets `tq->reading_vm`, so read_http() is the connection's
reading task exactly like read()). Barn staged that task's row with the
programmer of the verb that called read_http().

## State
- Conformance (branch tests/read-http-kill-perm, worktree ../mct-read-http-kill):
  two tests in builtins/http.yaml,
  kill_task_of_read_http_task_follows_connection_player (expects 0) and
  kill_task_of_read_http_task_refuses_reading_verb_programmer (expects
  E_PERM). Both pass on managed WSL Toast; both failed on unchanged Barn
  (0 vs E_PERM and E_PERM vs 0).
- Barn (this branch): task.State gains HTTPReadConnection, set by read_http()
  beside IsHTTPReadSuspended and carried through the retry snapshot;
  registrationRow and ownEndRow use it as the row owner. Spec tasks.md §6.2,
  §7.1 and builtins/tasks.md §2.2, §3.2, §3.3 name read_http() alongside read().

## Decisions and why
- Kept the IsHTTPReadSuspended flag and added the connection beside it rather
  than replacing the flag with an ObjID: a zero ObjID is #0, a real object, and
  some tasks are built as struct literals that would never initialise it
  (engine/call_verb.go Task literal).
- The tests use a second connection logged in as Test.db's Programmer (#4,
  owns itself, not a wizard). The converse test sets #4.owner to a fresh
  non-wizard p so p may call read_http() on that connection, and restores it.

## Next step
Run the Barn conformance selection, open both PRs, merge Barn first, then
conformance; remove worktrees and branches.
