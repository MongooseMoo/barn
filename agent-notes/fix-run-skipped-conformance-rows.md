# Run skipped conformance rows on Barn (issue #1112, stage 1)

## Goal

Make Barn's managed profile declare every `requires` feature name that Barn
really implements (`index_extensions`, `anonymous`, `fork`, `exec_support`,
`command_dispatch`, `toastcore_fixture`, `tasks`, `forking`, `server_options`),
derived from what the build and configuration provide, run the rows on Barn,
and report which pass and which fail. Barn only; the suite is not edited.

## State

Derivations written in `profile/manifest.go` (`suiteFeatures`). Diagnostic
full run on Barn in progress with every name declared (toastcore_fixture
temporarily hard-coded true in the diagnostic build only, not committed).

## Decisions and why

- Where to declare: the profile manifest, not `server_version("features")`.
  Toast's `server_version("features")` group pushes `VERSION_FEATURES(_FDEF)`
  only `#ifdef VERSION_FEATURES` (toaststunt `origin/master:src/version.cc:141-146`),
  and `git grep VERSION_FEATURES origin/master` finds only those lines, so the
  list is empty on Toast. Barn's MOO-visible list (`config/options.go`
  `FeatureNames`) is left alone. The suite reads `feature.X` from the target
  manifest first (suite `src/moo_conformance/test_conformance.py` `_has_feature`,
  manifest branch before the `server_version` fallback). The oracle/target
  comparison (`src/moo_conformance/profile_gate.py` `validate_profile_pair`)
  compares only `option.OUTBOUND_NETWORK`, `option.PROMOTE_NUMBERS` and three
  top-level fields, so new Barn features do not break the stock WSL oracle
  pairing.
- What each name means in the suite. The suite's Toast lanes assert all of them
  true for every build (`.github/workflows/toast-conformance-core.yml`, the
  "Record the exact Toast capability profile" step), i.e. they are compiled-in
  Toast capabilities, except `toastcore_fixture`, which the matrix sets true on
  one lane only ("ToastCore suites start the shipped _db/startup/toastcore.db
  fixture; one lane runs them", commit "Declare the ToastCore fixture feature on
  the 64-bit outbound Toast lane"). The names came from cow_py with the suite's
  initial release. From the gated rows:
  - `index_extensions`: `^`/`$` inside index and range expressions
    (`language/index_and_range.yaml`, description "Index and range extensions
    with ^ (first) and $ (last) operators"). Barn: parser accepts them
    (`parser/lexer.go` `^`/`$` tokens). Derived by parsing
    `x = {1}; return {x[^], x[$], x[^..$]};` with Barn's parser.
  - `anonymous`: anonymous objects, made by `create(parent, 1)`
    (`language/anonymous*.yaml`). Derived from `registry.Has("create")`.
  - `fork` and `forking`: the fork statement (`builtins/task_local.yaml` uses
    `fork`; `server/exec.yaml` uses `forking` and its rows run `fork go (0)`).
    Both derived from parsing `fork (0) endfork fork t (0) endfork`.
  - `tasks`: task inspection and control (rows call task_id, queued_tasks,
    kill_task, suspend, resume). Derived from the registry holding all five.
  - `exec_support`: `exec()` with the harness's `executables/` directory (the
    harness installs `_exec_fixtures` into `<server_dir>/executables` and starts
    the server with cwd `<server_dir>`, suite `server.py` `_install_exec_fixtures`
    and the `cwd=self._temp_dir` launch; Barn resolves `executables/<name>`
    relative to cwd, `builtins/system.go` `validateAndResolvePath`). Derived from
    `registry.Has("exec")`.
  - `server_options`: the server reads `$server_options` (rows set `player_huh`,
    `fg_ticks` etc. and call `load_server_options()`). Derived from
    `registry.Has("load_server_options")`.
  - `command_dispatch`: typed commands reach verbs (rows type `zip` and expect
    the huh verb). Derived from the server having at least one listener
    (`internal/app/app.go`, `CommandDispatch: len(specs) > 0`), since only a
    listener's connections dispatch commands.
  - `toastcore_fixture`: on the suite's Toast lanes this is a lane choice (one
    lane of three runs the ToastCore rows), not a build capability. What a Barn
    profile needs for those rows is to load the shipped
    `_db/startup/toastcore/toastcore.db`, a Format Version 17 database. Derived
    from Barn's database reader: `db/format/reader.go` `HeaderVersion` (the
    header switch the reader already used, now a function the reader calls)
    accepting the Format Version 17 header.
- Held back: `fork` and `forking`. Holding `fork` keeps
  `audit/task_scheduling_toast_oracle.yaml` (3 failing rows) from running;
  holding `forking` keeps `server/exec.yaml` (1 failing row) from running.
  Both names mean the fork statement, so they are held together. Holding
  `exec_support` or `tasks` instead would have hidden other passing rows
  (`builtins/exec_call_shapes.yaml`) without keeping `task_scheduling` from
  running. The derivation for them (parsing `fork (0) endfork`) is removed, not
  left as dead code; add it back with the fixes.
- `tasks` is declared (derived, no failing row of its own), but every row it
  gates also needs `fork` or `forking`, so it changes nothing yet.

## Results

Runs (managed harness, CI command form without `--log-dir`/admission-evidence
flags, `-rfEs --junitxml`, WSL Debian, root, suite worktree at suite
`origin/main` "Merge pull request #258", submodule initialised):

1. Diagnostic build, all nine names declared (toastcore_fixture hard-coded in
   that build only). Start load average 26.94 (8 CPUs; the CI runners were
   busy), end 2.85. 14217 rows: 14158 passed, 4 failed, 55 skipped, 1463 s.
2. Focused rerun of the 4 failing rows plus `capability_admission`, same
   build, start load 1.20: 2 failed, 3 passed.
3. Final build (this branch): see "Final run" below.

Rows newly running under the names (diagnostic run), per file:

| File | Gate | Passed | Failed |
|---|---|---|---|
| language/index_and_range.yaml | index_extensions | 130 | 0 |
| language/anonymous.yaml | anonymous | 56 | 0 |
| language/anonymous_value_authority.yaml | anonymous | 2 | 0 |
| audit/task_scheduling_toast_oracle.yaml | fork | 30 | 3 |
| builtins/task_local.yaml | fork, tasks | 20 | 0 |
| builtins/exec_call_shapes.yaml | exec_support | 1 | 0 |
| server/exec.yaml | exec_support, forking, tasks | 16 | 1 |
| server/signals_children.yaml | exec_support, forking, tasks | 4 | 0 |
| server/command_parsing.yaml | command_dispatch, server_options | 4 | 0 |
| server/toastcore_interactive_lifecycle.yaml | toastcore_fixture | 5 | 0 |
| total | | 268 | 4 |

That is 272 rows, not the 267 the audit and issue #1112 give (see Surprises).

Skipped in the diagnostic run (55): 30 Incompatible with feature: 64bit; 19
Requires config 'named_log_file'; 4 Incompatible with option:
OUTBOUND_NETWORK; 2 Requires config 'unprivileged_posix' (local only: the WSL
harness runs as root; Barn CI's runner does not skip these, per the audit).

### Failing rows

The suite's Toast CI lanes assert all of these names and pass every one of
these rows (audit, suite run 44958382522). The stock WSL oracle manifest
`profiles/toast/stock-wsl-testdb.json` declares neither `fork` nor `forking`
(its features are only `option.OUTBOUND_NETWORK`, `option.PROMOTE_NUMBERS`,
`runtime.arch_bits`), so the managed stock WSL lane skips these rows and could
not run them; I did not edit it. No Toast run of my own backs the rows below;
the Toast evidence is the suite CI lanes and the Toast source.

1. `audit/task_scheduling_toast_oracle.yaml::audit_delayed_fork_preserves_identity_locals_and_schedule_across_restart`
   (fails at idle load too). Message: `step 'for queued in (queued_tasks(1)...'
   expected value [1, 1, 1, 1, 1, 1, 1, 1, 1, 1], but got [1, 1, 1, 1, 1, 0, 1, 1, 1, 1]`:
   element 6 is `queued[8] == 1`, the line number queued_tasks() reports for a
   delayed fork after a checkpoint and restart. Toast writes the fork's
   current line (6) as the task header's first line (`src/tasks.cc:1906-1910`
   `write_forked_task`), and on reload sets `program->first_lineno` to it
   (`src/tasks.cc:2069`), but `queued_tasks()` asks `find_line_number(ft.program,
   ft.f_index, 0)` (`src/tasks.cc:2381`), and `find_line_number` first returns the
   program's line cache when pc and vector match (`src/decompile.cc:1066-1067`);
   `new_program` seeds that cache with line 1, pc 0, MAIN_VECTOR
   (`src/program.cc:33-36`), and a reloaded fork is MAIN_VECTOR at pc 0, so Toast
   reports 1. Barn compiles the saved body and renumbers it from the saved first
   line (`engine/task_load.go:165` `prog.WithFirstLine(saved.FirstLine)`) and
   reports `t.Program.LineForIP(0)` (`task/snapshot.go:346`), which is 6.
2. `server/exec.yaml::multiple_simultaneous_execs_work` (fails at idle load
   too). Message: `expected value 1, but got 0`. The row kills two of four
   exec-waiting forks and, in the same task, expects `queued_tasks()` no longer
   to list them. Toast's `kill_task` removes the task at once, external queues
   included (`src/tasks.cc:2679-2772`, the exec queue through
   `killing_closure` at 2759), so the next `queued_tasks()` omits it. Barn's
   `kill_task` only stages the task row as killed for commit
   (`builtins/tasks.go:116-117`), while `queued_tasks()` lists the task
   manager's live tasks (`builtins/tasks.go:35` `mgr.GetQueuedTasks()`), which
   do not see the staged kill until the calling task commits. Which conjunct
   of the row's return expression is false was read from source, not isolated
   with a probe; the staged-kill reading predicts `!(task_ids[1] in seen_after)`.
3. `audit/task_scheduling_toast_oracle.yaml::audit_suspended_vm_preserves_frames_locals_continuations_and_resume_value`
   (failed at load 26.9, passed at load 1.2). Message: `step 'result =
   {suspended_obj}.audit...' expected success but got error: MooError.E_RANGE`.
   Not diagnosed: the step indexes the suspended task's recorded result after
   a restart; at high load the result list was shorter than the step indexes.
   Whether Barn or the row's timing is at fault is unknown.
4. `audit/task_scheduling_toast_oracle.yaml::audit_waif_values_in_suspended_vm_survive_restart`
   (failed at load 26.9, passed at load 1.2). Message: `step 'result =
   {waif_class}.audit_re...' expected success but got error: MooError.E_RANGE`.
   Same shape as 3; not diagnosed.

Rows 3 and 4 would run on Barn's loaded CI runners, so they count as failing
for the hold-back decision.

### Names held back

- `fork`: rows 1, 3, 4 above. Also keeps the 20 passing
  `builtins/task_local.yaml` rows and the 30 passing
  `audit/task_scheduling_toast_oracle.yaml` rows from running.
- `forking`: row 2 above. Also keeps the 16 passing `server/exec.yaml` rows and
  the 4 passing `server/signals_children.yaml` rows from running.

### On this branch

Of the 272 gated rows, 198 run and pass (index_and_range 130, anonymous 56,
anonymous_value_authority 2, exec_call_shapes 1, command_parsing 4,
toastcore_interactive_lifecycle 5); 74 still do not run (task_scheduling 33,
task_local 20, exec 17, signals_children 4) because `fork`/`forking` are held
back.

### Final run

(pending)

## Not in this stage

- 19 `named_log_file` rows (`server/signals_log_rotation.yaml` 7,
  `server/signals_user_hooks.yaml` 12). The suite sets `named_log_file` only when
  the managed server command contains `{log}` (suite `src/moo_conformance/plugin.py`
  `moo_config`, `"named_log_file": "1" if ... "{log}" in
  managed_server.command_template`); the harness substitutes
  `<server_dir>/server.log` (suite `server.py`, `log=Path(self._temp_dir,
  "server.log")`). The rows rename `server.log`, send SIGUSR1, and expect
  "LOGFILE: Closing due to remote request signal." in the old file and
  "LOGFILE: Reopening due to remote request signal." in the new one, plus
  `server_log()` lines in each (Toast `src/server.cc:844,850`). Barn would need:
  a `barn serve --log FILE` flag writing the Toast-format text log to FILE
  (today only `--log-dir`, the JSON sink, `cmd/barn/main.go:55`); the SIGUSR1
  default in `server/signals.go:26` (now "Without a named log file, the USR1
  default has no effect") to close and reopen FILE with those two lines; and
  `--log {log}` in the CI command (`.github/workflows/ci.yml:196`). The
  USR2/user-hook rows depend on the same log file for their assertions.
- 4 outbound-off rows (`builtins/network_matrix.yaml` 2,
  `builtins/open_network_connection_call_shapes.yaml` 1,
  `builtins/open_network_connection_options.yaml` 1, all `skip_if:
  "option.OUTBOUND_NETWORK"`). They need a CI lane running
  `--config=profiles/barn/outbound-off.conf
  --profile-id=barn-linux-testdb-outbound-off` (profile already in
  `profiles/barn/profiles.json`). The profile gate requires the oracle and
  target `option.OUTBOUND_NETWORK` to match (suite `profile_gate.py`
  `REQUIRED_FEATURE_KEYS`), and every Toast manifest in `profiles/toast/` has
  it true, so that lane also needs an outbound-off stock Toast oracle manifest.
  It would be a second conformance step or matrix entry next to
  `.github/workflows/ci.yml:192-206`, with the full-suite cost again (about 24
  min locally).

## Surprises

- The gated row count is 272, not 267: the audit's 267 omits the 5
  `toastcore_fixture` rows from its own six-name sum (130+58+53+22+4 = 267;
  +5 = 272).
- Rows 3 and 4 fail only under load; at load ~27 on 8 CPUs they fail, idle
  they pass.
- The stock WSL oracle manifest declares none of these names, so the managed
  stock WSL Toast lane cannot run any of the 272 rows.

## Next step

Lead files one issue per failing row (1, 2, and 3+4 as the load-sensitive
pair), each fix then adds `fork`/`forking` back in `profile/manifest.go`
`suiteFeatures`. Then stage 2: named log file and outbound-off lane.

## Barn PR body

Title: Declare the conformance feature names Barn provides in its profile manifest

272 conformance rows never ran on Barn because they require feature names
Barn's profile manifest never declared (issue #1112). This declares, in the
manifest the managed harness reads, each name whose rows all pass on Barn,
each derived from what the build provides:

- `index_extensions`: Barn's parser accepts `^`/`$` in index and range
  expressions.
- `anonymous`: the registry has `create`.
- `tasks`: the registry has task_id, queued_tasks, kill_task, suspend, resume.
- `exec_support`: the registry has `exec`.
- `server_options`: the registry has `load_server_options`.
- `command_dispatch`: the server has a listener.
- `toastcore_fixture`: the database reader loads Format Version 17, the
  ToastCore fixture's format (`db/format.HeaderVersion`, the reader's header
  switch as a function).

`server_version("features")` is unchanged: Toast's is empty
(`src/version.cc:141-146`, no `VERSION_FEATURES`).

`fork` and `forking` are held back: four rows they gate fail on Barn
(`audit_delayed_fork_preserves_identity_locals_and_schedule_across_restart`,
`multiple_simultaneous_execs_work`, and two suspended-VM restart rows that fail
under load). Each becomes its own issue.

198 of the 272 rows now run and pass; 74 wait on `fork`/`forking`.

Part of #1112.
