# Fix: input_command_resumed_false_context_and_fallback flake

## Goal

Find and fix the cause of `server/input_callback_context.yaml::input_command_resumed_false_context_and_fallback`
failing once on Barn in CI (barn PR 1032, run 44888846177, job 136805922401):
`expected value [1, 1, 0, 1], but got [1, 1, 1, 1]` at the step starting
`h = {handler}; for attempt in ...`. Log saved at `C:/Users/Q/code/barn/.tmp/pr1032-conf.log` lines 553-632.

## State

Done: cause found from source, reproduced by a deterministic Go test, fixed in the engine, committed locally on fix/input-callback-resumed-false-flake. Not pushed.

## Findings

### The test (suite `src/moo_conformance/_tests/server/input_callback_context.yaml`)

Row 19: kind `command`, hook `do_command`, login 1, text `cb_probe one two`,
outcome `resumed_false`, tail `suspend(0); return 0;`, native 0. Setup (lines 26-55)
makes a listener `h` whose `do_command` records `this.events`, `this.frame`, then
runs the tail; the logged-in user `u` has a native command verb `cb_probe` that
increments `h.native`. The client sends `cb_probe one two`; the check (lines 73-88)
waits for `h.events`, then 50 ms more, and returns:

1. `h.events == {...}`: do_command was called once with the expected verb/this/caller/player/args/argstr/... frame.
2. `h.frame == {...}`: `callers()[1]` seen from inside do_command matches.
3. `h.native`: how many times the server fell back to native command dispatch and ran `u:cb_probe`. Expected 0, got 1.
4. the logged-in user is connected iff login was 1.

So on the failing run Barn treated a `do_command` that suspended (and later returned 0)
as "not handled" and ran the native parser, which ran `u:cb_probe`.

### Toast (upstream `git show origin/master:src/tasks.cc`)

`do_command_task` (tasks.cc:812-870): `if (run_server_task_setting_id(... "do_command" ...) != OUTCOME_DONE || is_true(result)) { /* handled */ } else { native find_verb_on ... }`.
A hook that reaches `suspend(0)` returns OUTCOME_BLOCKED from the synchronous
interpreter run; Toast is single-threaded, so the suspended task cannot resume until
the main loop dispatches it, which is after `do_command_task` has already decided.
Toast therefore guarantees native = 0 for this row; no timing dependence.

### Barn

- `server/input_login.go:160-186` `callDoCommand`: `FlowSuspend` or `FlowException` -> handled; otherwise `result.Val.Truthy()`. Matches Toast's rule.
- The result it inspects comes from `engine/task_factory.go:146-150` `runServerVerbTask`: `s.runTaskAdmitted(t, ...)`, then `s.settleTaskResult(t, nil)`, then `return t.Result`.
- On suspend, `engine/task_runtime.go:677-680` calls `handOffSuspended`, which `RequeueYield`s the task (task_runtime.go:936-943); the deferred lifecycle block (task_runtime.go:208-226) then calls `releaseTaskExecution` (runtime.go:307-324), which clears the execution lease and `NotifyScheduleChange()`. From that moment a scheduler worker on another goroutine may resume the task (suspend(0) is ready at once). The resumed slice writes `t.Result = result` (task_runtime.go:500/504) with `{FlowReturn, 0}`.
- `runServerVerbTask` reads `t.Result` only after that release, plus `settleTaskResult` -> `flushTaskOutput` (runtime.go:595-601). If the resumed slice finishes first, `callDoCommand` sees a normal return of 0 and runs the native parser: native = 1. Under CI load the input goroutine is easily descheduled in this window.
- Same read pattern affects every `runServerVerbTask` caller: `do_blank_command`, `do_out_of_band_command`, `handle_uncaught_error` (task_runtime.go:998-1006), `handle_task_timeout` (task_runtime.go:1612-1617).

Classification: (a), a Barn ordering bug. Toast decides on the first slice's outcome by construction; Barn reads a shared field after another goroutine may have overwritten it (also an unsynchronised data race on `t.Result`).

### Reproducer (Go, deterministic)

`engine/do_command_suspend_test.go` `TestServerVerbTaskReportsSuspendWhenResumedBeforeCallerReads`:
`#0:do_command` is `suspend(0); return 0;`. The runtime's task output flusher is
called by `settleTaskResult` after the lease release and before `runServerVerbTask`
reads its result, so the test drains the scheduler from inside the flusher, finishing
the resumed slice first. Without the fix it fails with
`result = {Val:0 Flow:1 ...}, want FlowSuspend` (a normal return of 0, exactly what
made `callDoCommand` fall through to the native parser in CI). With the fix it passes,
also under `-race -count=3`.

### Fix

`engine/task_runtime.go`: `taskCompletion` gains `result`; the lifecycle defer in
`runTaskSliceAdmitted` copies `t.Result` into it immediately before
`releaseTaskExecution`, while this invocation still owns the task, and
`runTaskAdmitted` now returns that slice result. `engine/task_factory.go`
`runServerVerbTask` returns the slice result instead of reading `t.Result` after the
release. Other `runTaskAdmitted` callers discard the new return value (they never
read `t.Result` afterwards, or only on terminal completion).

### Managed harness runs (WSL Debian, Linux cross-builds with CGO_ENABLED=0)

Script: `uv run --project <suite worktree> --frozen moo-conformance src/moo_conformance/_tests/server/input_callback_context.yaml -m "admission or conformance" --server-command "<bin> serve --db {db} --listen tcp://127.0.0.1:{port} --config=profiles/barn/outbound-on.conf --profile-id=barn-linux-testdb-outbound-on --profile-manifest={manifest} --log-dir=" --server-db=.../_db/Test.db --server-db-dir=.../_db/startup --moo-host=127.0.0.1 --oracle-profile-manifest=profiles/toast/stock-wsl-testdb.json --fail-on-unexpected-skip --strict-markers -q -k "capability_admission or (...)"`.

- Unfixed binary (origin/master production code), whole suite collection, `-k capability_admission or input_callback_context`: 19 passed (load 16.12).
- Unfixed, `-k ... input_command_resumed_false`: 2 passed (load 6.30), and again narrowed to the file: 2 passed (load 17.51).
- Unfixed, 8 iterations of both resumed_false rows: 8 x 3 passed; load (1-min) per batch 23.23, 19.46, 18.25, 16.20, 13.81, 11.33, 10.24, 9.41. Not reproduced: the window is the few microseconds between the lease release and the `t.Result` read, so the harness rarely hits it.
- Fixed binary, 3 iterations of the whole file: 19 passed each (load 10.01, 11.51, 11.55).

## Decisions and why

- Rule Zero was met by reading Toast's `do_command_task` source; no managed Toast run, since the Toast outcome is decided synchronously by construction and the suite already passes on Toast.
- Fix in the engine's shared `runServerVerbTask` path, not in `callDoCommand`, because every server hook caller (`do_command`, `do_blank_command`, `do_out_of_band_command`, `handle_uncaught_error`, `handle_task_timeout`) had the same read.
- No suite change: the test asserts what Toast guarantees.

## Surprises

- A Windows cross-build needs `CGO_ENABLED=0` (cgo with the MinGW gcc fails on Linux headers).
- Passing the YAML file path as a positional selector cuts a run from ~7.5 minutes to ~25 seconds.

## Next step

Lead publishes the Barn branch. Nothing in the suite worktree changed.
