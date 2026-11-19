# fix/queued-fork-export-varnf

## Goal

Find and fix why a queued forked task that Barn wrote to a checkpoint raises
E_VARNF when Toast loads and runs it
(`server/fork_line_numbers_checkpoint_restart.yaml::queued_fork_keeps_verb_line_numbers_after_checkpoint_restart`,
Barn writes, Toast resumes: expected `[6, 10]`, got `['E_VARNF', 10]`).

## State

Cause found by reading both sides; fix in progress.

Cause: the queued fork's activation line ends with the debug flag, and Barn
always writes 0 there.

- Toast writes the fork's activation with its debug flag
  (`execute.cc` write_activ_as_pi: `"... -10 %d\n", ..., a.debug`), reads it
  back (read_activ_as_pi: `&a->debug`), and runs the fork with it
  (do_forked_task: `RUN_ACTIV = a`). The flag is the forking verb's `d` bit,
  copied when the fork is queued (OP_FORK passes RUN_ACTIV by value to
  enqueue_forked_task2).
- Barn: `db/format/writer_task.go` writeActivationAsPI has `debug := 0` and
  writes it for every queued fork. `db/format/reader_task.go`
  readQueuedTaskActivation parses the field and discards it (`_ = debug`).
- The test's verb is `xd`. Toast loads Barn's fork with debug 0, so
  `y = 1 + "a"` yields E_TYPE as a value instead of raising, the except arm
  never binds `caught`, and reading `caught` yields E_VARNF as a value:
  `{E_VARNF, 10}`. The variable environment and body text are not at fault.
- Barn-to-Barn and Toast-to-Barn pass because Barn never uses the saved flag:
  `engine/task_load.go` loadQueuedTask leaves the frame's VerbDebug at its
  default (true).

Fix built; Go tests green (`go test ./engine ./vm ./db/format ./task ./types`).
`engine.TestQueuedForkCheckpointsItsVerbDebugFlag` was red before the fix (the
`d` verb's fork was written `0 -7 -8 2 -9 2 0 -10 0`) and is green after.

Cross-engine result for the one test (Barn writes, stock WSL Toast
`/root/src/toaststunt/build-release/moo` resumes; suite at origin/main; the
"suite tests" session of `scripts/cross-engine-checkpoint.sh` from the
ci/cross-engine-checkpoint branch, narrowed with
`-k "capability_admission or queued_fork_keeps_verb_line_numbers_after_checkpoint_restart"`):

- Barn built without the fix: `1 failed, 1 passed`, `expected value [6, 10],
  but got ['E_VARNF', 10]`.
- Barn built with the fix: `2 passed, 14048 deselected`.

The run used copies under the Linux home (`/root/qfe-varnf`: Linux Barn
binaries cross-built on Windows, `profiles/`, a fresh clone of the suite), not
/mnt/c. The scratch copies were removed afterwards.

What changed:

- `types/fork.go`: `ForkInfo.Debug`.
- `vm/control.go`: the fork statement records the forking frame's VerbDebug;
  NewForkVM runs the child with it (it used to look the verb up again by name
  on `this` when the child started).
- `task/snapshot.go`: `ForkSnapshot.Debug`.
- `db/format/writer_task.go`: writes the flag. `db/format/reader.go`,
  `reader_task.go`: `QueuedTask.Debug` read from the line and kept on re-write.
- `engine/task_load.go`: a loaded fork runs with the saved flag.
- Fixtures that build a queued fork by hand and expect a raise now say
  `Debug: true` (`engine/fork_descriptor_test.go`,
  `engine/restored_fork_environment_test.go`); the vm fuzz harness's hand-built
  restore carries `saved.Debug` (`vm/checkpoint_resume_test.go`).

## Decisions and why

- Carry the flag from the fork statement to the file in one field
  (`types.ForkInfo.Debug`, set from the forking frame in `vm/control.go`),
  through `task.ForkSnapshot`, the writer, the reader (`QueuedTask.Debug`) and
  `loadQueuedTask`. Reading it back is needed so a fork Barn loaded and
  checkpoints again without running it (Toast to Barn to Toast) keeps its flag.

- NewForkVM takes the flag from the fork record instead of looking the verb up
  when the child starts, so one field decides the flag for running and for
  writing. Toast decides it when the fork is queued (the activation is copied
  then), and a fork inside `eval()` gets the eval activation's flag
  (setup_activ_for_eval sets debug 1; Barn's eval frame defaults VerbDebug
  true).

## Surprises

- The symptom names a variable (E_VARNF) but no variable was missing from the
  file: with debug 0 the except arm never ran, so `caught` was never assigned.
- Before this change a fork loaded from any checkpoint ran in Barn with debug
  on whatever the file said, and a fork started in Barn took the flag the verb
  had when the child started, not when it was queued.

## Unverified

- No conformance test yet pins the two behaviours that changed on the Barn
  side beyond the failing test: a fork from a verb without `d` loaded from a
  checkpoint (errors as values), and a verb whose `d` flag changes between the
  fork statement and the child's start. Both follow Toast's source
  (execute.cc read_activ_as_pi, do_forked_task, OP_FORK); neither was run on
  the Toast oracle.
- The other three directions of this test and the rest of the cross-engine
  job were not rerun locally; CI on the PR covers them.

## Next step

Push the branch and open the PR (the lead does this). Consider Toast-first
conformance tests for the two unverified behaviours above.
