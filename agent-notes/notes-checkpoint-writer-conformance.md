# Checkpoint writer conformance handoff

## Authority and repository state

- Task: audit and bring Barn's checkpoint/database writer into complete conformance with current WSL Toast source.
- Barn control surfaces read: `AGENTS.md` and `plans/barn-toast-mongoose-convergence-workstreams.md`.
- Barn branch and HEAD before edits: `master`, `e79140c2d533253f25b0f7e086e127dc62ef2404`.
- Barn tracked and staged state before edits: clean.
- The checkout contains extensive pre-existing untracked noise; none of it is to be staged.
- WSL Toast source checkout: `/root/src/toaststunt`.
- Verified Toast HEAD: `aecc51e9449c6e7c95272f0f044b5ba38948459e`.
- Toast tracked state is clean; its listed artifacts are untracked only.

## Current verified findings

- Toast database version output is driven by `current_db_version` and its database file order is:
  header, users, values pending finalization, task queue, active connections,
  permanent/recycled objects in expanding object-number batches, zero object
  batch terminator, then verb programs.
- Toast object output includes name, flags, owner, location, last move,
  contents, parents, children, verb definitions, property definitions, and all
  property values. Programs are emitted separately by permanent object number
  and zero-based verb index.
- Toast value output covers clear, none, string, object, error, integer, catch,
  finally, float, map, list, iterator collapse, anonymous object, waif, and
  boolean. Float formatting uses `DBL_DIG + 4` significant digits.
- Toast task output writes:
  - forked tasks from waiting and active background queues, with line, rounded
    wake time, task id, activation-as-program-info, runtime environment, and
    the exact fork program;
  - suspended tasks from waiting and active background queues, with rounded
    wake time, task id, resume value, task-local value, VM header, and every
    activation;
  - interrupted tasks for every external queue and every idle queue blocked in
    `read()`/`read_http()`, with status text and full VM. On load these resume
    as suspended tasks carrying `E_INTRPT`.
- Toast checkpoint replacement writes a generation-suffixed temporary file,
  flushes and `fsync`s it, closes it, removes the destination database, then
  renames the temporary file to the destination. A failed checkpoint dump
  removes the temporary file and abandons that attempt.
- The existing convergence plan's two-case `dump_persistence` green gate is not
  evidence of complete writer coverage; this assignment requires the broader
  source audit.

## State and blocker

- Committed writer-only slice `3c0f839 fix checkpoint queued task header order`.
  The pre-fix regression wrote `0 9 5 123`; the corrected writer emits Toast's
  `0 9 123 5` (`start_time` before `task_id`). Focused `db/format`, complete
  `db/format`, and `go test ./... -count=1` passed.
- A proposed checkpoint replacement mismatch was invalid. Two consecutive
  `WriteCheckpoint` calls to the same target already pass on current Windows
  Barn, so the temporary test was removed and no source change or commit was
  made.
- Current uncommitted writer-only TDD slice: Toast's activation program-info
  sentinel is typed integer `-111`; Barn wrote typed integer `0`. This is
  committed as `32f0903 fix checkpoint activation sentinel`.
- Committed `25fd3eb fix checkpoint activation header sentinels`: Barn now
  writes Toast's historical `-7 -8 -9 -10` fields instead of zeros.
- Committed `91c4e71 preserve stored verb name in task checkpoints`: the
  invoked verb and stored verb name no longer collapse to one string.
- Committed `c33ee70 reject lossy queued task checkpoints`: a queued task with
  no serializable fork program now fails the checkpoint rather than being
  silently omitted.
- Current uncommitted deletion-first slice: the regression
  `TestWriteSuspendedTasksRejectsLossyCheckpoint` failed because a nonempty
  suspended-task list was serialized as `0 suspended tasks`. `writer_task.go`
  now rejects nonempty suspended snapshots, and the unreachable placeholder VM
  serializers/raw-value helpers were deleted. This is committed as
  `d115a21 reject lossy suspended task checkpoints`.
- Committed `2ba9425 round checkpoint fork times like Toast`: a fork scheduled
  at 123.6 seconds now serializes as 124 rather than the floored 123.
- Current uncommitted variable-order slice: Toast writes runtime environments
  in program `var_names` order. The unchanged behavioral test failed on
  iteration 5 when Barn emitted `a,z` for a program declaring `z,a`.
  `task.ForkSnapshot` now captures bytecode `VarNames`, and `writer_task.go`
  requires and emits that exact order instead of ranging the map. This is
  committed as `c086a68 preserve task variable order in checkpoints`.
- Committed `7f8b804 preserve typed this in task checkpoints`: queued fork
  activation `_this` now preserves the full typed value (the regression used
  ANON `#44`) instead of forcing OBJ.
- Committed `065130f write anonymous object verb programs`: writer program
  enumeration now includes `AnonymousObjects`; the pre-fix focused output
  incorrectly contained zero programs.
- Committed `86a4aa4 write waif properties in class order`: Toast writes sparse
  WAIF properties in ascending class propdef index order; Barn no longer emits
  them in nondeterministic Go map iteration order.
- Committed `9adde5a invalidate waifs with missing classes on dump`: WAIFs whose
  class is missing, recycled, or otherwise invalid now dump with class `#-1`
  and no property overrides, matching Toast.
- Committed `7471352 preserve fork first line in checkpoints`: queued fork
  snapshots capture the fork body first line from bytecode line information;
  the writer no longer substitutes the current activation line and fails
  closed if the required source position is unavailable.
- The documented managed persistence command was run from the repository root:
  `.\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb
  .\Test_conf.db -RunDb .\Test_run.db -Port 7788 -K dump_persistence
  -ExtraPytestArgs "-rs"`. Barn built, the managed server started, and the
  harness connected, but all 11 selected rows were skipped because they
  require config `managed_server` supplied with `--server-command`. The exact
  result is in `reports/runs/20260728_183305`; this is not a passing persistence
  restart gate.
- The documented managed `task_scheduling_toast_oracle` selector completed in
  `reports/runs/20260728_183433`: 21 passed and the two restart-dependent rows
  were skipped for the same missing `managed_server` configuration. The passing
  rows verify live task scheduling behavior, but do not verify checkpoint
  restart of suspended or pending forked tasks.
- Expanded ownership is now explicitly authorized across the Barn reader,
  task/VM snapshot, scheduler, server, active-connection, and store metadata
  surfaces, plus the managed harness in
  `C:\Users\Q\code\moo-conformance-tests` when required.
- Committed `4c42f6a fix queued checkpoint header reload`: the unchanged
  writer-reader round-trip regression proved that the reader still interpreted
  Toast's queued-task header as `<task_id> <start_time>`, reversing the corrected
  writer's `<start_time> <task_id>` fields. Focused `db/format`, complete
  `db/format`, `go test ./... -count=1`, and `git diff --check` passed.
- `moo-conformance-tests` is on `main` at `origin/main` with no tracked changes
  and extensive pre-existing untracked artifacts. Barn remains on `master`,
  now 14 commits ahead of `origin/master`, with no tracked changes.
- The conformance suite's managed server already supports a copied disposable
  database, `restart_server`, genuine process-down `down_ms`, preservation of
  the working database across restart, and adoption of common Toast checkpoint
  outputs (`.out`, `.new`, `.out.db`, `.new.db`). The pytest plugin exposes that
  machinery only when the invocation supplies `--server-command`; Barn's
  documented wrapper currently starts its own server externally and therefore
  causes all restart rows to skip.
- Existing durable restart coverage is narrow: two world-value rows in
  `dump_persistence.yaml` and two delayed-fork rows in
  `task_scheduling_toast_oracle.yaml`. There is no existing end-to-end coverage
  for a suspended VM, nested frames/locals/task-local/wake value, interrupted
  `read()` or external waits, active/former connections, `last_move`, or
  checkpoint hook ordering.
- Reverified stock Toast source at
  `aecc51e9449c6e7c95272f0f044b5ba38948459e`: queued forks persist rounded
  start time, id, activation identity, ordered runtime environment, and fork
  program; suspended tasks persist typed wake value plus full VM; external and
  `read()`/`read_http()` waits persist as interrupted VMs and reload as
  suspended with `E_INTRPT`; full VM state includes task-local, stack-depth
  metadata, every program/environment/operand stack/activation/temp/PC/error
  PC/builtin continuation; active connections persist player/listener pairs;
  every object persists typed `last_move`; checkpoint hooks run
  `checkpoint_started` before flush and `checkpoint_finished` after completion.
- Windows `uv` could not use the conformance checkout's `.venv` because uv
  failed to remove `.venv\lib64` with access denied. The user explicitly
  authorized the tracked checkout from inside WSL with a WSL-native isolated
  uv environment; the Windows `.venv` was not repaired, recreated, or used.
- The tracked conformance checkout resolves to
  `/mnt/c/Users/Q/code/moo-conformance-tests`. Using
  `UV_PROJECT_ENVIRONMENT=/tmp/barn-checkpoint-conformance-uv` and the managed
  server command
  `/root/src/toaststunt/build-release/moo {db} {db}.out -p {port}`, stock Toast
  passed the two existing task restart rows 2/2 and the complete existing
  `dump_persistence` selector 11/11. The harness owned the disposable DB copies,
  server lifecycle, checkpoint adoption, and restart.
- Conformance commit `7a06160 document checkpoint conformance coverage` adds
  the required source/runtime and harness capability matrix at
  `reports/checkpoint-roundtrip-coverage-matrix-20260728.md`. It identifies
  existing coverage, every requested observable checkpoint state, exact
  capability gaps, and the ordered rows to add.
- The unchanged documented Barn wrapper plus
  `--fail-on-unexpected-skip` failed both selected restart rows because pytest
  had no managed server. Current uncommitted deletion-first wrapper slice
  removes the separate PowerShell server lifecycle and gives ownership to
  pytest with `--server-command` and `--server-db`. The unchanged red selector
  now executes and passes 2/2 with zero skips in run
  `reports/runs/20260728_191120`.
- The previously blocked reader/scheduler/server/store ownership boundary is
  now explicitly authorized. No blocker is active.
- Committed `2a6822a run conformance through managed server`: the documented
  Barn wrapper now delegates the disposable database, server lifecycle,
  checkpoint-output adoption, and restart to pytest via `--server-command` and
  `--server-db`. The focused two restart rows passed 2/2 in
  `reports/runs/20260728_191120`; the full `task_scheduling_toast_oracle`
  selector passed 23/23 in `reports/runs/20260728_191247`; and
  `dump_persistence` passed 11/11 in `reports/runs/20260728_191330`.
  Script syntax, `go test ./... -count=1`, and `git diff --check` also passed.
- The first matrix-pinned conformance expansion is an uncommitted row named
  `audit_delayed_fork_preserves_identity_locals_and_schedule_across_restart`
  in `task_scheduling_toast_oracle.yaml`. It creates a delayed fork with
  deliberately ordered locals, captures the public queue metadata, checkpoints,
  restarts, verifies persisted metadata and locals, and then verifies the
  resumed runtime identity and values.
- Managed stock WSL Toast passed that exact finalized row 1/1. The observed
  Toast contract is that scheduled start, task id, programmer, verb location,
  stored verb name, receiver, ordered locals, `task_perms()`, `player`, `this`,
  `caller`, and runtime local values persist. Because the fork reloads as its
  own program, the public `queued_tasks()[8]` line normalizes from enclosing
  verb line 6 before checkpoint to fork-program line 1 after restart.
- Barn run `reports/runs/20260728_192500` did not establish a checkpoint
  failure. Its eight-element mismatch proves the row failed at the
  pre-checkpoint queue assertion. The row used Toast's `queued_tasks(1)`
  include-variables form, but Barn currently interprets argument `1` as an
  owner-`#1` filter and hid the wizard-owned fork. No Barn change was made for
  that initial invalid checkpoint claim.
- Current state: the conformance checkout has exactly the intended YAML row as
  a tracked uncommitted change, while its earlier coverage-matrix commit remains
  one commit ahead of `origin/main`. Barn remains clean in tracked source after
  its committed wrapper slice; this notes file and pre-existing report/noise
  artifacts remain untracked. No push has been performed.
- Conformance commit `f354e18 test delayed fork checkpoint identity` now freezes
  the first strengthened Toast-proven row as an atomic one-file YAML commit.
  The exact managed Toast row passed, `git diff --check` passed, the new test
  name occurs exactly once, and the offline unit/schema suite passed 71/71
  (11,567 live conformance cases deselected).
- The conformance repository's documented duplicate lint is baseline-red before
  this row can be treated as a clean global gate: it reports 45 existing
  duplicate-name groups and 28 existing duplicate-content groups, with no
  occurrence of the new row. Global Ruff is also baseline-red with 55 existing
  Python findings in untouched files. Neither baseline failure authorizes
  unrelated cleanup.
- The earlier claim that Barn was checkpoint-red on this row was wrong.
  Replacing its four `queued_tasks(1)` lookups with `queued_tasks()` was then
  proven invalid on managed Toast: the public no-argument form has only ten
  fields, so reading ordered locals from field 11 raises `E_RANGE`.
  `queued_tasks(1)` is the exact Toast include-variables surface that exposes
  the checkpoint row's required locals evidence.
- The controlling correction is Option A: restore the checkpoint row's four
  `queued_tasks(1)` calls and amend its unpushed commit so the durable row is
  truthful. Then add a dedicated queued-task semantics row as a separate
  conformance commit, prove Toast green and unchanged Barn red, fix Barn's
  argument semantics in its own Barn commit, and only then rerun the unchanged
  checkpoint row Toast first and Barn second.
- Authoritative Toast `src/tasks.cc:bf_queued_tasks` establishes the exact API:
  argument 1 truthy sets `include_variables`; wizard authority independently
  sets `show_all = is_wizard(progr)`; argument 2 truthy returns the count.
  Conformance commit `0c2ef86` has valid behavioral assertions but must be
  amended to replace its false show-all argument name and description.
- The exact Barn target is not an owner filter: a wizard sees all queued tasks;
  a nonwizard sees only tasks whose current programmer matches the caller;
  argument 1 controls the extended runtime-environment field; and argument 2
  controls count mode.
- The truthful queued-task conformance slice is finalized as commit
  `bc50c27 test queued task visibility and variables`. Its row
  `audit_queued_tasks_wizard_include_variables_exposes_other_programmer`
  captures `task_perms()` for the other programmer, schedules a delayed task as
  that programmer, and requires a wizard's `queued_tasks(1)` to expose field 5,
  the eleven-field extended shape, and the inherited marker local. Managed WSL
  Toast passes the exact committed row 1/1.
- Unchanged Barn's exact pre-fix result is preserved in
  `reports/runs/20260728_194025`: expected `[1, 1, 1]`, actual `[]`. Barn
  misparsed argument 1 as owner `#1`, so the wizard could not see the other
  programmer's task.
- A focused Go regression
  `TestQueuedTasksUsesToastVisibilityAndArgumentSemantics` was red at the same
  wizard `queued_tasks(1)` visibility failure. The first implementation pass
  now passes that unit test by making wizard visibility independent of
  arguments, filtering nonwizards by field-5 programmer, using argument 1 for
  the optional variables map, and argument 2 for visible count mode.
- The first post-fix managed Barn run improved from `[]` to `[0, 1, 1]`: wizard
  visibility and the extended variables field are fixed, but queued field 5 is
  still not the task's live `task_perms()`. After correcting the row to capture
  `task_perms()` and re-proving it on Toast, Barn still returns `[0, 1, 1]` in
  `reports/runs/20260728_194430`.
- The remaining exact cause is the fork creation handoff:
  `Scheduler.CreateForkedTask` stamps both `t.Programmer` and the activation
  frame from `parent.Programmer`, while the current permission authority used
  by `task_perms()` is `parent.Context.Programmer`. The Barn source slice is
  still uncommitted; no commit or push has been made for it.
- Conformance commit `bc50c27 test queued task visibility and variables` and
  Barn commit `e3d51fa fix queued task visibility and variables` now complete
  the separate API slice atomically. Barn uses wizard authority independently
  of arguments, filters nonwizards by the reported current programmer,
  appends the variables map only for truthy argument 1, uses argument 2 for
  count mode, and stamps forked tasks from the live task-context programmer
  with a nil-context fallback.
- Post-fix Barn passes the exact dedicated row in
  `reports/runs/20260728_194713`. The broader `queued_tasks` selector finishes
  with 19 passes and one established declared skip in
  `reports/runs/20260728_194742`. Both focused Go regressions, the prior
  stack-limit regression in `scheduler/task_factory_test.go`, the previously
  failed eval-fork regression, `git diff --check`, and `go test ./... -count=1`
  pass. The controlling convergence plan was reread after the full-suite pass.
- The restored delayed-fork checkpoint row has now been rerun after the API
  commits in the required order. Managed stock WSL Toast passes the exact
  committed row 1/1, including the extended locals surface. The Barn half has
  not yet been run. No push has been performed.
- The first truthful committed-Barn checkpoint baseline is
  `reports/runs/20260728_195107`: expected eight ones before the checkpoint and
  got `[1, 1, 1, 1, 0, 1, 1, 1]`. The sole mismatch was public queued-task line
  1 versus Toast's enclosing-source line 6; identity, programmer, verb
  location/name, receiver, and ordered locals already matched.
- Barn commit `763900c fix queued fork source line` derives the activation line
  from the extracted fork program instead of hardcoding 1. Its focused
  regression was red at 1 versus 6, then green. The complete delayed-fork
  checkpoint row passes in `reports/runs/20260728_195229`; the full
  `task_scheduling_toast_oracle` family passes 25/25 in
  `reports/runs/20260728_195255`; and `go test ./... -count=1` plus
  `git diff --check` pass. The plan was reread after the full-suite pass.
- The committed coverage matrix now selects ordered target 3: the complete
  suspended-VM row. Toast source `execute.cc::make_stack_list` confirms
  `task_stack(id, 1, 1)` returns hottest-to-oldest frames with seven fields:
  typed receiver, invoked verb, programmer, defining location, player, positive
  line, and an ordered runtime-variable map. This is the pre-resume observable
  for nested frame topology and locals.
- Conformance commit
  `1ca5800 test complete suspended VM checkpoint round trip` adds
  `audit_suspended_vm_preserves_frames_locals_continuations_and_resume_value`.
  Its nested `inner` -> `outer` -> fork-body task proves all three extended
  `task_stack(id, 1, 1)` frames before the checkpoint, then resumes with a map
  containing a list and error value. After restart it proves stable task
  identity and authority, frame-specific locals, task-local state, indexed
  operand continuation, caller/receiver/verb identity, line topology, and an
  exception handler still active after the suspension.
- Managed stock WSL Toast passes that exact row 1/1. The conformance offline
  schema/transport/profile/option/managed-server/file-selection unit set passes
  58/58, and `git diff --check` is clean.
- Unchanged Barn is red in `reports/runs/20260728_200156`. The failure is before
  checkpoint writing: evaluating the expected three extended frames raises
  `E_RANGE`. Source inspection identifies the exact cause in
  `builtins/tasks.go`: Barn accepts the third `task_stack` flag but explicitly
  ignores it, returning six fields even when Toast returns a seventh
  runtime-variable map. Barn's VM already owns each frame's ordered
  `Program.VarNames` and `Locals`, but its suspended-task synchronization only
  copies line numbers into `task.CallStack`.
- Barn commit `63ccd2c expose suspended frame variables in task_stack`
  completes that independent API slice. Its focused red regression proves the
  third flag independently appends a map, with and without line numbers. At
  every VM yield Barn now copies bound locals from each matching
  `Program.VarNames`/`Locals` pair into its activation frame. Focused
  builtins/VM/task tests and `go test ./... -count=1` pass, and the coverage
  matrix was reread after the full-suite gate.
- With that commit built, the unchanged row reaches restart and then fails in
  `reports/runs/20260728_200721` with `E_INVIND` while indexing the still-empty
  `audit_result`. The pre-checkpoint three-frame topology and locals therefore
  pass.
- The exact persistence cause is now proven. `resume()` changes the live VM
  task from `TaskSuspended` to `TaskQueued` while retaining `BytecodeVM` and
  its typed `WakeValue`. `Scheduler.TaskSnapshots` classifies solely by task
  state, so it sends this ready VM to `writeQueuedTasks`. Because the task
  originated as a fork, Barn serializes only its original fork program and
  locals. Restart begins that fork body again, reaches a new indefinite
  `suspend()`, and never executes the saved indexed continuation.
- Barn currently has no full suspended-VM persistence path:
  `writeSuspendedTasks` rejects all suspended snapshots,
  `readSuspendedTasks` only skips Toast VM records, `SuspendedTask` stores only
  id/time, and server startup loads only queued tasks. The live Barn VM state
  that must be carried includes every compiled source program, IP, ordered
  locals, operand stack/base pointers, activation identity, loop/exception
  continuation state, task-local value, typed wake value, and saved context.
- The full-VM slice is now present but wholly uncommitted and not yet compiled.
  `task.VMSnapshot`/`VMFrameSnapshot` define a package-neutral persistence DTO;
  `vm.PersistenceVMSnapshot` clones compiled programs, per-frame operand
  segments, locals, activation identity, exception handlers, pending errors,
  flags, and saved context; and `vm.RestoreVMSnapshot` reconstructs a yielded
  machine.
- The writer now emits suspended-task records with Toast's outer shape
  (rounded task header, typed wake value, task-local value, VM header, source
  program, ordered bound environment, per-frame operand stack, activation
  identity, temporary value, and PC). The standard temporary-value slot carries
  a typed Barn metadata list containing the exact compiled code/constants,
  line table, handler state, and saved context needed to resume Barn bytecode
  without recompiling an extracted fork body incorrectly.
- The reader now retains rather than skips suspended records, reconstructs the
  DTO and call stack, and preserves whether the saved VM was already ready.
  Scheduler classification sends every task with `BytecodeVM` through this
  suspended-VM path even when its state is `TaskQueued`; server startup now
  calls `LoadSuspendedTasks`.
- This is still an active source slice. No compile, test, managed run, commit,
  or push has occurred since these edits. LoopStack has not been encoded
  because current Barn execution never appends to it; builtin continuation
  data remains the explicitly later matrix target.
- The focused writer/reader round-trip now passes and preserves ready state,
  typed wake/task-local values, exact compiled program metadata, unbound
  locals, operand stack, exception handler, and saved context. Existing stock
  Toast suspended-record parsing initially failed because those records lack
  Barn's metadata marker; the reader now retains them as non-restorable
  source/environment snapshots, preserving its prior load behavior, and the
  existing multi-activation reader regression passes.
- The first managed rerun still returned `E_INVIND` because Barn's
  `dump_database()` only queued an asynchronous checkpoint and returned before
  `{db}.new` was durable. The dump callback now executes the checkpoint
  synchronously. With that exact durability boundary, the complete row passes
  in `reports/runs/20260728_202053`, and `go test ./... -count=1` passes.
- The first broad 26-row scheduling run
  `reports/runs/20260728_202136` found that a fresh delayed fork also owns a
  preconfigured VM. It was incorrectly classified as a suspended
  continuation, causing post-restart field 11 to disappear. VM persistence is
  now restricted to machines whose `IsYielded()` state is true. The two-row
  rerun `reports/runs/20260728_202234` proves the complete VM row still green
  and reduces the delayed-fork failure to only its saved schedule comparison.
- Toast `tasks.cc::list_for_forked_task` reports field 2 with the same
  `ROUND(&ft.start_tv)` used by `write_forked_task`. Barn's writer already
  rounded but `ToQueuedTaskInfo` truncated with `Unix()`, so synchronous dump
  made the fractional mismatch observable. The uncommitted correction now
  rounds the public queued timestamp with `Add(500ms).Unix()` too. It has not
  yet been rerun.
- Barn commit `7c72fe5 persist complete suspended VM continuations` contains
  the finalized 14-file full-VM slice. Its final gates were the delayed plus
  complete-VM rows 2/2 in `reports/runs/20260728_202356`, all scheduling rows
  26/26 in `reports/runs/20260728_202426`, and the full Go suite.
- Conformance commit `793db7b test suspended VM restart timing` adds separate
  remaining-time and process-down-overdue rows. Both pass managed stock WSL
  Toast 2/2. Unchanged Barn run `reports/runs/20260728_202807` passes the
  overdue row but reports `[1, 0, 1]` for the remaining row: the task remains
  pending and in the future, but its public saved schedule changes.
- Two focused red regressions prove both parts of the exact timestamp defect.
  Suspended writer records truncated `WakeTime` while Toast rounds it; and
  pre-checkpoint `queued_tasks()` reported the task's original `StartTime`
  instead of its timed `WakeTime`. The active Barn timing slice rounds the
  suspended header and uses the rounded wake time for suspended public task
  metadata.
- Post-fix timing rows pass 2/2 in `reports/runs/20260728_203014`; the complete
  scheduling family passes 28/28 in `reports/runs/20260728_203045`; focused
  writer/task regressions, `git diff --check`, and `go test ./... -count=1`
  pass. The matrix was reread after the full suite. The Barn timing changes
  are not yet committed; no push has been performed.

## Next action

Stage only `db/format/writer_vm.go`, its writer regression, `task/task.go`, and
the task timestamp regression; commit the verified timing slice. Then continue
ordered target 3 with one focused ANON-in-suspended-task Toast-first row.
## 2026-07-28 ANON suspended-VM row in progress

- Active matrix row after the completed suspended-VM timing rows: ANON values preserved through a suspended local, task-local value, and resume value across checkpoint/restart.
- Added the focused row `audit_anonymous_values_in_suspended_vm_survive_restart` to `src/moo_conformance/_tests/audit/task_scheduling_toast_oracle.yaml` in the conformance repository.
- The first two managed stock Toast invocations selected zero tests, so they are not oracle results.
- Direct isolated collection proved the row is not loading because line 1179 uses unsupported YAML tag `!ref`: `could not determine a constructor for the tag '!ref'`.
- No Barn run or Barn source diagnosis has occurred for this row.
- Current blocker is only the conformance assertion syntax within the active uncommitted test slice.
- Next action: replace `!ref` with the repository's established capture-reference expectation form, collect the row, then run the exact managed stock WSL Toast command before Barn.
## 2026-07-28 ANON oracle and Barn baseline

- Conformance commit `7d81063 test anonymous task values across checkpoints` contains the single focused ANON suspended-VM row.
- The exact managed stock WSL Toast oracle passes 1/1 after the assertion syntax correction.
- The isolated WSL offline conformance gate passes 58/58 and `git diff --check` was clean before commit.
- Unchanged committed Barn is red in managed run `reports/runs/20260728_203914`.
- Barn preserves the anonymous value's type, validity, identity, task id, and queue removal, but both restored aliases read inherited `class-marker` instead of the anonymous object's local override `anonymous-marker`.
- Source inspection proves the load-path mismatch: `Database.resolvePropertyNames` in `db/format/reader_helpers.go` resolves inherited positional property names only for `database.Objects`; `database.AnonymousObjs` retain `_inherited_N` placeholder keys, so lookup falls through to the class value after reload.
- This is now a Barn source slice. No Barn source has been edited yet.
- Next action: add a focused failing anonymous inherited-override dump/reload regression, run it red, extend the existing property-name resolution pass to anonymous builders without introducing a helper/interface/adapter, then run focused and full gates plus the managed ANON row.
## 2026-07-28 ANON slice complete

- Conformance commit: `7d81063 test anonymous task values across checkpoints`.
- Stock WSL Toast authority: exact ANON row passes 1/1.
- Unchanged Barn red: `reports/runs/20260728_203914`; anonymous type, validity, and identity survived, but restored property lookup returned the inherited class value.
- Focused Go regression `TestRoundTripPreservesAnonymousInheritedOverride` was red because the anonymous override slot was absent after reload.
- Barn commit: `e549962 preserve anonymous property overrides on reload`.
- The fix extends the existing `Database.resolvePropertyNames` two-pass resolution over both regular and anonymous builders, keyed by builder identity; no new helper/interface/adapter was introduced.
- Focused regression and all `db/format` tests pass.
- Post-fix managed exact ANON row passes in `reports/runs/20260728_204318`.
- Full `task_scheduling_toast_oracle` family passes 29/29 in `reports/runs/20260728_204330`.
- Full `go test ./... -count=1` and `git diff --check` pass.
- No push.
- Next action after rereading the committed coverage matrix: execute the WAIF suspended-value row Toast-first.
## 2026-07-28 WAIF row complete with no Barn defect

- Conformance commit: `78adae4 test waif task values across checkpoints`.
- The row proves WAIF class, pre-checkpoint property data, local/task-local/resume type and alias identity, post-restart shared mutation, stable task identity, and task removal.
- The first Toast attempt was not a persistence result: an invalid cross-task `task_local(task_id)` pre-check returned `E_ARGS`. That unsupported call was removed; the resumed task itself remains the task-local authority.
- Corrected managed stock WSL Toast row passes 1/1.
- Unchanged committed Barn also passes 1/1 in `reports/runs/20260728_204746`.
- The earlier premise that WAIF might require a Barn source fix was wrong; no Barn source change was made.
- The conformance offline gate passes 58/58 and `git diff --check` was clean before commit.
- No push.
- Next ordered matrix target after reread: builtin continuation, using only a source-backed candidate whose exact Toast induction and observable result pass first.
## 2026-07-28 builtin-continuation candidate rejected by Toast-first gate

- The matrix permits a builtin-continuation row only if an exact source-backed Toast induction and observable result pass first.
- Stock Toast source shows `call_function()` has registered read/write state and can wrap `create()`'s registered continuation while `create()` waits for the new object's `initialize` verb.
- The experimental row successfully suspended inside `initialize`; pre-checkpoint `task_stack` showed the live initializer frame.
- After resume/checkpoint/restart, stock Toast resumed `initialize` and persisted its child mutation (`marker == "continued-marker"`), but the outer continuation returned an invalid value: `typeof(created) == 64`, rendered `">>Unknown value<<"`, and the recorded restored `task_id()` was corrupted (`714995232` in the observed run).
- This exact Toast induction therefore does not satisfy the matrix's pass-first gate. It is not Barn evidence and must not become a conformance assertion.
- The entire uncommitted builtin row plus its temporary `call_function`/`children` requirements were restored from Git. The conformance worktree is tracked-clean at committed WAIF coverage `78adae4`.
- No Barn run and no Barn source change occurred for builtin continuation.
- Next ordered target: add the source-mandated blocking `read()` restart row Toast-first, then packaged `exec()`.
## 2026-07-28 blocked read restart slice in progress

- Conformance commit: `69ae29e test blocked read restart interruption`.
- The managed row opens a disposable secondary connection, blocks a wizard-owned fork in `read(negative_connection)`, proves the live `block` frame, checkpoints with the connection alive, performs a managed restart, and expects the restored VM to catch `E_INTRPT`.
- Stock WSL Toast passes the exact row 1/1.
- The conformance offline gate passes 58/58 and `git diff --check` was clean before commit.
- Unchanged committed Barn is red in `reports/runs/20260728_205837`; post-restart `audit_result` is empty and indexing raises `E_RANGE`.
- Source inspection shows the exact format gap:
  - Barn tasks carry `ReadingPlayer` and full VM snapshots.
  - `Writer.writeInterruptedTasks()` hardcodes `0 interrupted tasks`.
  - `Database.readInterruptedTasks()` parses the count but skips every interrupted VM instead of restoring it.
  - Toast's contract writes a header such as `<id> interrupted reading task`, followed by task-local state and the complete VM, then reloads it as a ready task whose resume value is `E_INTRPT`.
- This is a checkpoint writer/reader/load slice, not a connection workaround and not a generic suspended-task substitute.
- Next action: add focused red writer/reader tests for an interrupted reading VM, implement literal interrupted-section serialization and restoration using the existing VM frame format, load it as ready with `E_INTRPT`, then run exact/family/full gates and commit.
## 2026-07-28 blocked read writer/reader implementation

- Focused regression `TestInterruptedReadingTaskWriterReaderQueuesEIntrptContinuation` first failed because the reading snapshot was emitted under `1 suspended tasks` and the interrupted section remained `0 interrupted tasks`.
- The active source slice now separates snapshots with `ReadingPlayer != #-1` into the literal interrupted-task section.
- The writer emits `<id> interrupted reading task`, task-local state, the four-field VM header, and the complete existing VM frame representation.
- The reader now parses that literal section into a ready snapshot with the original task id, task-local value, complete VM, and `E_INTRPT` wake value; the old skip-only path was removed.
- The focused regression now passes.
- The source slice is still uncommitted. Exact managed Barn, scheduling-family, full Go, and diff gates have not yet been run.
- Next action: run the broader `db/format` package gate, build `barn.exe`, run the exact managed blocked-read row, then the complete scheduling family and full Go/diff gates. Commit only if all pass.
## 2026-07-28 blocked read runtime semantics correction

- `go test ./db/format -count=1` passed after the interrupted-section implementation.
- First rebuilt managed Barn run `reports/runs/20260728_210742` resumed the stable task but returned `[1, "returned", "E_NONE", 0]` instead of entering the saved catch block with `E_INTRPT`.
- This proves serialization/load was working but Barn treated a direct typed error wake value as an ordinary builtin return value.
- Toast source authority is explicit: `resume_from_previous_vm()` calls the interpreter's error-resume path whenever the resume value has `TYPE_ERR`; other typed values are pushed as ordinary return values.
- Focused VM regression `TestBytecodeErrorResumeRaisesIntoSavedExcept` first returned `"returned"` and is now green.
- The VM's existing `SetResumeValue`/`Resume` path now records a direct typed error and raises it into the restored exception stack on resume, while preserving ordinary value replacement.
- The matrix was reread after the focused green gates; `read()` remains the current target before packaged `exec()`.
- Source slice remains uncommitted. Next action: rebuild and rerun the exact managed blocked-read row, followed by the complete scheduling family, full Go, diff check, and atomic commit if all pass.
## 2026-07-28 blocked read slice verified, pending commit

- Rebuilt exact managed blocked-read row passes 1/1 in `reports/runs/20260728_211115`.
- Complete `task_scheduling_toast_oracle` family passes 31/31 in `reports/runs/20260728_211134`.
- The first full Go run found only the exhaustive VM pool dirty-state guard missing the new `resumeError` field. The guard was updated to dirty and explicitly verify reset of that field.
- `go test ./vm -count=1` and the subsequent `go test ./... -count=1` pass.
- `git diff --check` passes.
- The matrix was reread after the exact row, family, VM package, and full-suite passes. The next ordered target remains packaged `exec()` after this read slice is committed.
- No push.
- Next action: stage only the seven read-slice source/test files, commit the atomic kept slice, report the safe boundary, then begin packaged `exec()` Toast-first.
## 2026-07-28 interrupted read committed; packaged exec Toast-first in progress

- Barn commit `8eab9bd persist interrupted read continuations` is the completed interrupted-read slice. Both tracked worktrees were clean after commit; no push.
- Added an uncommitted packaged-`exec()` row using the managed server's packaged `executables/sleep` fixture with a 30-second child.
- The row proves the forked task is live in `task_stack()` and `queued_tasks()` with command `executables/sleep` before checkpoint.
- First Toast attempt reached post-restart observation before the durable result existed and raised `E_INVIND`; a diagnostic run then showed the eventual exact result `{same task id, "error", E_INTRPT}` and no queued task.
- A later attempt showed the newly created holder invalid after restart, proving `dump_database()` return alone was not a completed-flush authority for this row.
- The row now uses the suite's established 1500 ms checkpoint-flush interval before managed restart and guarded result observation.
- With that correction, the exact managed stock WSL Toast row passes twice consecutively, each 1/1.
- The isolated offline conformance gate passes 58/58 and `git diff --check` passes.
- The conformance row is still uncommitted. Before committing, narrow its availability requirement so adding one exec row does not make every existing scheduling row depend on suite-wide exec support; then rerun the exact Toast row and offline/diff gates.
- No Barn run or Barn source diagnosis has occurred for packaged `exec()`.
## 2026-07-28 packaged exec Barn slice in progress

- Conformance commit `5ee5cea test interrupted exec restart` contains the focused row. Availability is row-local with `skip_if: missing builtin.exec`; no suite-wide capability dependency was added and the managed fixture installer required no code change.
- Final managed stock WSL Toast row passes 1/1 after the requirement narrowing; the earlier corrected form also passed twice consecutively. Offline conformance remains 58/58 and diff-clean.
- Unchanged committed Barn is red in `reports/runs/20260728_212106`: holder valid, durable result absent, and the exec task still queued after restart.
- Exact source gap: `task.Task` has `IsExecSuspended` and `ExecCommandName`, but `task.Snapshot` omitted both, so checkpoint serialization treated the exec VM as an ordinary suspended task.
- Focused exec writer/reader regression first failed to compile because those persistence fields did not exist. The active slice now copies them into snapshots, classifies exec waiters into the interrupted section, writes the command status header, and accepts Toast's arbitrary external-queue status string while restoring every interrupted VM ready with `E_INTRPT`.
- Focused read/exec writer-reader tests and the task snapshot test pass.
- Rebuilt exact managed Barn exec row passes 1/1 in `reports/runs/20260728_212408`.
- The source slice is uncommitted. Next action: reread the matrix, run the complete scheduling family, full Go, and diff gates, then commit the atomic exec slice if green.
## 2026-07-28 packaged exec slice verified, pending commit

- Complete `task_scheduling_toast_oracle` family passes 32/32 in `reports/runs/20260728_212439`, including both interrupted read and interrupted exec rows.
- Full `go test ./... -count=1` passes.
- `git diff --check` passes.
- The matrix was reread after the exact row, family, and full-suite passes. Target 4's required read and packaged exec rows are now implemented; optional `read_http()` and background worker remain gated on deterministic Toast-first induction.
- The active Barn diff is limited to five files: snapshot fields/tests plus interrupted writer/reader classification/tests.
- No push.
- Next action: stage only those five files and commit the atomic packaged-exec slice, then evaluate the optional deterministic Toast-first inductions before ordered target 5.
## 2026-07-28 packaged exec committed; read_http optional row in progress

- Barn commit `58ca95c persist interrupted exec continuations` completes the packaged-exec source slice. Conformance commit remains `5ee5cea`.
- Deterministic managed `read_http()` induction exists in the repository through the secondary-connection fixture; no external service is required.
- Conformance commit `6010dd6 test interrupted read http restart` adds the focused row. Stock WSL Toast passes 1/1; offline conformance passes 58/58 and the diff gate passed before commit.
- Unchanged committed Barn is red in `reports/runs/20260728_212859`: holder valid, result absent, and the HTTP parser task still queued after restart.
- Barn's `read_http()` uses its own in-process waiter queue and does not set `ReadingPlayer`, so the earlier interrupted-read classification does not cover it.
- Focused classification regression first failed to compile because neither `task.Task` nor `task.Snapshot` had an HTTP-read suspension marker.
- Active uncommitted source slice adds the specific `IsHTTPReadSuspended` task/snapshot state, sets it when `read_http()` blocks, clears it on resume/kill, and classifies that VM into the existing interrupted section.
- Next action: finish snapshot regression coverage, run focused tests, rebuild, and run the exact managed read_http row before family/full/diff gates and commit.
## 2026-07-28 read_http slice verified, pending commit

- Focused HTTP classification, interrupted-reader, snapshot, and HTTP builtin tests pass.
- Rebuilt exact managed Barn read_http row passes 1/1 in `reports/runs/20260728_213146`.
- Complete `task_scheduling_toast_oracle` family passes 33/33 in `reports/runs/20260728_213209`.
- Full `go test ./... -count=1` and `git diff --check` pass.
- The matrix was reread after exact, family, and full-suite passes.
- Active diff is limited to six read_http marker/classification source and test files.
- No push.
- Next action: commit the atomic read_http slice, then attempt the deterministic stock background worker row Toast-first as the final optional target 4 item.
## 2026-07-28 read_http committed; background worker rejected

- Barn commit `ffcb21d persist interrupted http reads` completes the read_http source slice. Conformance commit remains `6010dd6`.
- A focused background-worker restart row was attempted with the source-backed `background_test("done", 30)` induction and managed checkpoint/restart flow.
- The authoritative stock WSL Toast binary collected the exact row but skipped it because builtin `background_test` is absent. The source implementation is conditional on `BACKGROUND_TEST`; it is not a stock runtime target in this oracle build.
- This fails the matrix's deterministic Toast-first gate. The entire uncommitted background row was restored from Git.
- Conformance tracked worktree is clean at `6010dd6`; no Barn run and no Barn source change occurred for background workers.
- Target 4 is complete to its stated gate: required read/exec and deterministic read_http are covered; unavailable background worker was attempted and rejected.
- Next ordered target: former-connection startup ordering, then `last_move`, then checkpoint hook ordering/status.
## 2026-07-28 former-connection startup row in progress

- Conformance commit `11ddac6 test checkpointed connection startup order` contains the focused row.
- Stock WSL Toast passes 1/1: the captured unlogged negative connection and its listener `#0` are delivered to `user_disconnected`, and that event precedes `server_started`.
- Offline conformance passes 58/58 and the diff gate passed before commit.
- Unchanged committed Barn is red in `reports/runs/20260728_213947`: `server_started` is recorded, but no former-connection event exists.
- Source inspection confirms both missing halves: `Writer.WriteDatabase()` hardcodes `0 active connections with listeners`; `readActiveConnections()` discards every pair. Server startup has no loaded former-connection state to replay before `callServerStarted()`.
- Checkpoint ownership is `Server.checkpoint()` -> `dbformat.WriteCheckpoint()`. The connection manager owns the live player/listener pairs. For unlogged connections the persisted player must be the public negative connection id `-conn.ID`, not the connection object's internal pre-login placeholder.
- Next action: add focused format and server-order regressions, thread exact active-connection snapshots through checkpoint writer/reader, replay loaded pairs via `user_disconnected` after listener bind and before `server_started`, then run exact/family/full/diff gates and commit.
## 2026-07-28 former-connection Barn slice in progress

- The focused format regression now passes: writer output preserves Toast's `N active connections with listeners` header and each player/listener pair; reader restores those pairs and also retains compatibility with the older player-only section.
- `WriteCheckpoint` now requires the active-connection snapshot and installs it on the writer. Its existing unit call site is updated.
- A focused connection-manager regression is red at the intended boundary, then implementation added a deterministic creation-order snapshot. Logged-in connections persist their player; unlogged connections persist public `-conn.ID`; both preserve the listener object.
- `Server.LoadDatabase` now retains loaded former connections and `Server.checkpoint()` supplies the manager snapshot, but startup replay is not implemented yet.
- Current tracked slice is intentionally uncommitted. No conformance rerun or full gate has occurred after these source edits.
- Next action: add the lifecycle-order regression, implement `user_disconnected` replay before `server_started`, run focused tests, then rebuild and run the exact managed Barn row, lifecycle family, full Go, and diff gates before committing.
## 2026-07-28 former-connection slice verified, pending commit

- Focused format, connection-manager, and startup-order regressions pass.
- Rebuilt exact managed Barn row passes 1/1 in `reports/runs/20260728_214704`.
- The complete `connection_lifecycle_toast_oracle` family passes 24/24 in `reports/runs/20260728_214719`.
- Full `go test ./... -count=1` and `git diff --check` pass.
- The matrix was reread after the exact row, lifecycle family, and full-suite passes. Ordered target 5 now proceeds to `last_move`.
- The source diff contains the active-connection format model/writer/reader, checkpoint plumbing, deterministic live snapshot, loaded former-connection state, and pre-`server_started` replay plus focused tests. The task notes remain uncommitted.
- One control-surface read immediately after the exact row used a Barn-relative path and failed; PowerShell continued into the already-selected lifecycle family. The matrix was then read from the conformance repository while that unchanged family ran, and again after it passed.
- Next action: stage only the eleven intended source/test files and commit this atomic slice, report the safe boundary, then start the `last_move` Toast-first row.
## 2026-07-28 former connections committed; last_move slice in progress

- Barn commit `82d09da persist checkpointed connections` completes the former-connection slice. No push.
- Conformance commit `bb203d8 test last move checkpoint persistence` adds one world-metadata row. It creates source/destination/object, captures the exact `last_move["time"]` and `last_move["source"]`, verifies bounds and location, adopts a managed checkpoint, and compares the same entries after restart.
- The exact managed stock WSL Toast row passes 1/1. Offline conformance passes 58/58 and the diff gate passed before commit.
- Unchanged committed Barn is red in `reports/runs/20260728_215154`: reading `last_move["time"]` raises `E_RANGE` before checkpoint, proving Barn still exposes an empty map after movement.
- Source inspection confirms the full narrow gap: `Store` has no last-move field/accessor; coarse and transaction move paths do not record it; VM property lookup fabricates an empty map; the v17 reader discards the typed slot; and the writer always emits an empty map.
- Focused store and format regressions are red at the intended API boundary because `Store.LastMove` and `StoreTxn.LastMove` do not exist.
- Next action: carry `last_move` as immutable object relationship metadata through builder, snapshot, reader/writer, coarse and transaction move paths, and VM builtin-property reads; then run the focused tests and unchanged exact row before world-metadata/full/diff gates and commit.
## 2026-07-28 last_move slice verified, pending commit

- Toast's exact behavior is a map updated by every move with `"time"` equal to the move epoch second and `"source"` equal to the object's prior location; the map and current location survive checkpoint adoption unchanged.
- Barn now carries that map as immutable object relationship metadata. Both coarse and decentralized transaction move paths update it, transaction reads see staged state, VM `.last_move` returns stored state, and v17 reader/writer plus snapshots preserve it.
- Focused store/format/VM tests pass, including pre-commit transaction visibility and a writer/reader round trip.
- Rebuilt exact managed Barn row passes 1/1 in `reports/runs/20260728_215608`; the complete one-row `world_metadata_toast_oracle` family passes in `reports/runs/20260728_215632`.
- Full `go test ./... -count=1` and `git diff --check` pass. The matrix was reread after the exact row, family, and full-suite passes.
- Ordered target 5 is not closed yet: the matrix still requires checkpoint hook ordering/status after `last_move`.
- Next action: stage only the eleven intended source/test files, commit this atomic slice, report the safe boundary, then add the checkpoint hook ordering/status row Toast-first.
## 2026-07-28 last_move committed; managed-adoption defect found

- Barn commit `fec44e7 persist last move metadata` completes the last-move slice. No push.
- The first checkpoint-hook row reached stock Toast and observed `{started, finished-success}` both before and after restart, contradicting the matrix's expected checkpoint boundary.
- Source and harness inspection proved this was not requested-checkpoint behavior: `ManagedServer.restart()` terminated Toast before adoption; Toast handles SIGTERM as a graceful shutdown and `db_shutdown()` writes a later shutdown dump to the same `{db}.out`; only afterward did the harness copy that overwritten output into the input DB.
- Therefore existing managed restart rows could adopt a shutdown dump rather than the output of their explicit `dump_database()`, violating the matrix's mandatory genuine-adoption condition.
- The uncommitted hook row was removed before starting the harness source slice.
- A focused managed-server regression simulated an explicit checkpoint output followed by a shutdown overwrite and failed with the shutdown dump adopted.
- The harness now synchronizes the requested checkpoint output before terminating the server. The focused managed-server suite passes 12/12; the complete offline conformance gate passes 59/59; `git diff --check` passes.
- Active conformance diff is exactly `src/moo_conformance/server.py` and `tests/test_managed_server.py`. It is uncommitted.
- Next action: commit this atomic harness correction, then re-add the hook row unchanged to the matrix's original boundary expectation and verify it on stock Toast. After that, rerun representative previously green restart rows under genuine adoption before Barn diagnosis.
## 2026-07-28 genuine checkpoint adoption repaired and revalidated

- Conformance commit `12c4c40 adopt checkpoint before managed shutdown` copies the requested output into the working input before terminating the server.
- A full restart-row audit then revealed another contamination path: Toast's graceful shutdown leaves a later output beside the newly adopted input, so an immediate next test can mistake that stale shutdown output for its own not-yet-finished checkpoint.
- Conformance commit `e9bb030 discard managed shutdown checkpoints` centralizes known output candidates and removes leftovers before each new server start. The regression proves the requested output is adopted and the shutdown output is absent afterward.
- Both managed-server tests (12/12) and the complete offline gate (59/59) passed before each harness commit.
- The first genuine-adoption rerun selected 12 prior rows: 7 passed and 5 failed. All five failures were rows that restarted immediately after Toast's asynchronous `dump_database()`; they had been false-green through shutdown-output adoption.
- The active test-only correction adds the established 1500 ms checkpoint-flush wait to the six affected task restart rows and the persistent walking-skeleton row. Every restart row in the audited task file now has that interval.
- Under the corrected harness and waits, stock WSL Toast passes all 14 selected restart authorities: connection, walking skeleton, suspended/delayed/timing/ANON/WAIF/read/read_http/exec tasks, and `last_move`.
- The complete offline gate passes 59/59 and `git diff --check` passes. The matrix was reread after the 14-row Toast pass.
- Active conformance diff is exactly the seven wait insertions in two YAML files. No hook row is currently present.
- Next action: commit this test-only genuine-adoption timing correction, report the safe boundary, then re-add and verify the checkpoint hook row on stock Toast before unchanged Barn.
## 2026-07-28 checkpoint hook row and full campaign complete

- Conformance commit `5761aea wait for managed checkpoint outputs` contains the seven missing flush waits.
- Conformance commit `aa312b9 test checkpoint hook persistence boundary` contains the final ordered target-5 row.
- Exact stock Toast behavior under genuine adoption is:
  - `checkpoint_started` runs first.
  - `checkpoint_finished` runs afterward with success argument `1`.
  - Before restart the live event list is `{{"started"}, {"finished", 1}}`.
  - The adopted requested checkpoint contains only `{{"started"}}`, proving the post-flush finished-hook mutation is outside that checkpoint.
- The corrected exact Toast hook row passes 1/1.
- The earlier premise that Barn needed a checkpoint-hook source fix was wrong. Unchanged Barn already matches the exact row in `reports/runs/20260728_221343`; no Barn source change or commit was made for it.
- Under genuine adoption, Barn passes all 15 checkpoint/restart authorities together in `reports/runs/20260728_221414`.
- The complete affected Barn families pass 60/60 in `reports/runs/20260728_221550`: task scheduling, connection lifecycle, world metadata, checkpoint lifecycle, and persistent walking skeleton.
- Final `go test ./... -count=1` passes. Barn and conformance tracked worktrees are clean; both diff gates pass. This notes file remains untracked and uncommitted. No push.
- Ordered matrix targets 1-6 are complete. Optional background-worker coverage was attempted and rejected at the stock-Toast availability gate; no unavailable row remains staged.
