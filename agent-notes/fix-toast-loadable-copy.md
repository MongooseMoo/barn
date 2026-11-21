# fix-toast-loadable-copy (issue #1033)

## Goal

A Barn command that writes a copy of any Barn-written database that ToastStunt
can load: suspended tasks Barn kept in its own frame form are left out, one line
per omitted task is printed (id, verb, object, refusal reason), exit 0. Without
the flag nothing changes. Proof on Toast per refusal reason, a fuzz-cycle lane
check, and `docs/going-back-to-toast.md`.

## State

- Built and committed locally (not pushed): `barn offline-checkpoint
  --toast-loadable`; `Runtime.TaskSnapshotsWithRefusals`;
  `dbformat.WriteToastLoadableCheckpoint` (uses the existing
  `OmitBarnFrameTasks`/`Omitted`); test
  `internal/dbtool/toast_loadable_test.go`; restart wrapper
  `scripts/toast-loadable-restart.sh`; `docs/going-back-to-toast.md`; README line.
- Toast proof done (Runs below): emergency-mode load on stock and 0ff6fd7,
  managed-harness resume on stock.
- Fuzz-cycle lane: waits on PR #1032 (`fuzz/cross-engine-generated`, open,
  auto-merge on). Not built on its branch, per the brief.

## Decisions and why

- The command is a flag on `barn offline-checkpoint`, not `barn dump`:
  `barn dump` writes no tasks at all (`dbtool.go` `dumpDatabase`, no
  `SetTaskSnapshots`), so a flag there would drop every task Toast can load.
  `offline-checkpoint` already restores the file's tasks and re-exports each
  through `Runtime.TaskSnapshots`, which is how a Barn-written file's Barn-form
  frames are told apart (they are refused again, with the reason).
- Reasons: `TaskSnapshots` already computed the refusal error and only logged
  it. `TaskSnapshotsWithRefusals` returns it by task id; `TaskSnapshots` wraps
  it, so every existing caller is unchanged.
- Leaving tasks out uses the existing `Checkpoint.OmitBarnFrameTasks` through a
  new `WriteToastLoadableCheckpoint`; `writeCheckpoint` gained the flag. The
  live checkpoint paths pass false.
- No counts-only mode: nothing here runs on the Mongoose dump in CI.
- `BARN_TOAST_LOADABLE_KEEP` in the test copies the Barn-written file and the
  converted copy out, so the same files can be loaded in Toast (no Barn
  command writes a checkpoint after running MOO code on master; the #976
  branch adds `offline-checkpoint --run`).
- The restart wrapper removes the converted copy's `.new.waifids` (see
  Surprises).

## Source readings

- `db/format/checkpoint_stage.go:84-98`: with `OmitBarnFrameTasks`, Capture
  drops every suspended/interrupted snapshot whose `VM.Frames[0].Native == nil`
  and logs id, verb, verbloc; it has no refusal reason.
- `engine/task_factory.go` `TaskSnapshots`: each task with VM state goes through
  `vm.ExportNativeSnapshot`; on error the snapshot keeps Barn form and a WARN
  names id, verb, verbloc, this, reason. The reason is not returned.
- `internal/dbtool/dbtool.go` `dumpDatabase`: `barn dump` writes
  `NewWriter(f, store.Snapshot()).WriteDatabase()` without `SetTaskSnapshots`,
  so its output holds no tasks at all (and no WAIF sidecar). A flag there could
  not keep Toast-loadable tasks, so `barn dump` cannot be the command.
- `internal/dbtool/offline_checkpoint.go` `WriteOfflineCheckpoint`
  (`barn offline-checkpoint --db DB`): loads the file, restores its tasks into a
  never-started runtime, takes `runtime.TaskSnapshots()` (so every loaded task is
  re-exported, with its refusal reason), writes `DB.new` through
  `dbformat.WriteCheckpoint`. This is the path that can tell tasks apart on a
  Barn-written file: a frame read in Barn's form (`__barn_vm_frame` marker,
  `agent-notes/toast-task-frames.md`) is restored and refused again.
- Refusal reasons (`vm/persistence.go:362-383`, `compiler/native_export.go`):
  create/recreate/recycle caller frame (`ErrToastLosesObjectContinuation`),
  move continuation without Toast form, builtin continuation (`BuiltinPC != 0`,
  `MoveContinuation`, `RecycleContinuation`), WAIF setter result local, fork body
  mismatch, program not the tables' compile, IP not at a call, no continuation,
  operand stack mismatch, native stack over max_stack, loop columns, exception
  codes, `ErrPathDependentClear`.

## Runs

- Load before Go work: Windows CPU load 48%.
- `go test ./internal/dbtool -run TestOfflineCheckpointToastLoadable -count=1 -v`:
  PASS. Barn kept six of seven forked tasks in its own form, reasons as logged:
  `frame 0 (#-1:): which variables Toast holds unbound at the call depends on the path into its block`;
  `frame 1 (#4:initialize): create: stock Toast does not resume ...` (both
  `create(p)` and `call_function("create", p)`: the frame's BuiltinCaller is
  `create` either way); `recreate: ...`; `frame 1 (#6:recycle): recycle: ...`
  (both `recycle(o)` and `call_function("recycle", o)`).
- Probe (scratch test, deleted) of more shapes: suspend in a finally a raise or
  return entered, `eval("suspend();")`, `call_function("eval", ...)`,
  `call_function("suspend")`, a location's exitfunc during `move`, `pass()`,
  index assignment with a suspend in the index, catch expressions with ANY and
  a computed code list, `except (#0.codes)`: all exported in Toast's form. A
  WAIF `_set_index` that suspends (`w[1] = 2`) is refused:
  `frame 0 (#-1:): instruction position 17 is not paused at a call`.
- Final test `internal/dbtool/toast_loadable_test.go`: 13 forked tasks, 6
  Toast-form shapes (plain, finally-after-raise, eval, call_function eval, move
  exitfunc, pass) and 7 Barn-form (path-dependent clear, create,
  call_function create, recreate, recycle, call_function recycle, WAIF index
  set). PASS, plus the two existing offline-checkpoint tests.
- Command on the Barn-written file (kept with `BARN_TOAST_LOADABLE_KEEP`):
  `barn offline-checkpoint --db orig.db --toast-loadable --log-dir=`, exit 0:
  ```
  read: 0 queued, 13 suspended or interrupted tasks, 0 connections
  omitted task 16: #-1:, this #-1: frame 0 (#-1:): which variables Toast holds unbound at the call depends on the path into its block
  omitted task 18: #-1:, this #-1: frame 1 (#4:initialize): create: stock Toast does not resume a create, recreate or recycle continuation from its checkpoint
  omitted task 20: ... create: ...   (call_function("create"))
  omitted task 22: ... recreate: ...
  omitted task 24: #-1:, this #-1: frame 1 (#6:recycle): recycle: ...
  omitted task 26: #-1:, this #-1: frame 1 (#7:recycle): recycle: ...   (call_function("recycle"))
  omitted task 28: #-1:, this #-1: frame 0 (#-1:): instruction position 17 is not paused at a call
  wrote .../orig.db.new: 0 queued, 6 suspended or interrupted tasks, 0 connections, 7 tasks omitted for Toast
  ```
- Toast emergency mode (WSL, load average 13.65/21.64/18.37 on 8 CPUs,
  scratch `toast_load.sh`: `moo -e -p 0 in.db out.db` on a private copy, walk
  input, `quit`). Barn-written file sha256 `19facf80...79f8`, converted
  `e9f6b8b1...d94a`:
  - stock `/root/src/toaststunt/build-release/moo` (2.8.0_7, sha256 `61dd1799...13dd`)
    and Mongoose `/root/src/toaststunt-mongoose-0ff6fd7/build-release/moo`
    (2.8.0_1, sha256 `1c0e5128...5a53`), Barn-written file: both
    `READ_ACTIV: Bad PC for suspended task.` / `READ_TASK_QUEUE: Bad suspended task vm, count = 7` /
    `DB_LOAD: Cannot load database!`, exit 1.
  - both builds, converted copy: load, exit 0; `queued_tasks()` holds tasks
    4, 6, 8, 10, 12 (`#8:exitfunc`), 14 (`#9:pp`): the six Toast-form tasks;
    Toast's own out.db: `0 queued tasks`, `6 suspended tasks`, `0 interrupted tasks`.
  - world walk (each object: name, parents, location, contents, verbs,
    properties) printed by Toast on the converted copy is identical to
    `barn eval --file` on the same copy (`diff`: no difference). Barn reading
    the converted copy back: `read: 0 queued, 6 suspended or interrupted tasks`.

- Managed harness, Barn writes, the converter runs, Toast resumes
  (`scripts/toast-loadable-restart.sh BARN TOAST {db} {port}` as
  `--restart-server-command`; scratch runner in the scratchpad, suite = a
  detached worktree of moo-conformance-tests origin/main `5116a6b`, since
  `C:/Users/Q/code/moo-conformance-tests` is on an older main without
  `--restart-server-command`; stock Toast; Linux barn
  `CGO_ENABLED=0 GOOS=linux GOAMD64=v3 go build`):
  - scratch YAML (`.tmp/tl-suite/server/toast_loadable_checkpoint.yaml`, not
    committed): one task suspended in a verb call (Toast form), one in
    `create()`'s initialize (Barn form). Load 12.79/17.47/17.37. PASSED: after
    the restart the plain task finished (`173`), the create task is gone
    (`created` 0, `inits` 1), the parent object is valid.
  - control, same YAML with plain `toast {db} {db}.new -p {port}` restart:
    FAILED with `READ_ACTIV: Bad PC for suspended task.` /
    `DB_LOAD: Cannot load database!`.
  - every `tests/mongoose-conformance/server/*checkpoint*.yaml` (14 tests:
    call, call_function move accept, eval, 7 finally shapes, move accept,
    enterfunc, exitfunc, range bound) through the converter: 15 passed
    (admission included) in 348 s, load 9.82/14.98/16.51. The converter keeps
    every task Toast can load and Toast resumes them.

## Surprises

- First harness run of the 14 tests: second test failed,
  `Barn failed ... WAIF identity sidecar does not match database`. The wrapper
  had moved the converted copy's `.new.waifids` to `{db}.waifids`; the harness
  knows no sidecar, Toast neither reads nor rewrites one, so the next Barn start
  on Toast's file found a stale sidecar. The wrapper now removes it.
- `barn dump` writes no tasks at all (see source readings).
- Property names `r`, `w` are builtin properties: `add_property(#0, "r", ...)`
  raises E_INVARG (test fixture uses `rc`, `wv`).
- Toast clears (OP_PUSH_CLEAR) only slots below 32, so the path-dependent
  shape exports fine when forked from a program with many variables; each shape
  runs in its own eval in the test.

## Not verified

- Resuming a converted copy on the Mongoose 0ff6fd7 build (only its
  emergency-mode load was run; the harness runs used stock Toast).
- The command on the Mongoose dump.
- Refusal reasons no test produced: builtin continuation with no Toast form
  (`BuiltinPC`/`MoveContinuation`/`RecycleContinuation` outside the converted
  cases), WAIF setter result local (frame-level refusal; the `_set_index`
  shape gave "not paused at a call" first), operand stack mismatch, native
  stack over max_stack, loop columns, exception codes, fork body / program
  mismatch.
- Whether Barn's shutdown writes a final checkpoint (doc step 1 says so).
- The fuzz-cycle lane (waits on #1032).

## Next step

- When `gh pr view 1032 --json state` says MERGED: merge origin/master, add a
  converted Barn-first run to `scripts/fuzz_cross_engine.py` for every program
  whose Barn-first run failed at the restart step (Toast did not boot), with
  `scripts/toast-loadable-restart.sh` as the restart command through
  `scripts/cross-engine-session.sh`, and require the restart to succeed; add
  its unit test to `scripts/test_fuzz_cross_engine.py`; run it on the lane's
  findings 2 and 3 choice records.
