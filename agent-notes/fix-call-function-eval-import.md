# fix-call-function-eval-import

## Goal

Resolve MongooseMoo/barn issue #1028: Barn refuses to boot from a ToastStunt
checkpoint holding a task suspended inside `call_function("eval", ...)`
("native builtin continuation requires conversion"). Barn must load it and
resume the task to Toast's result.

Done (lead's words): a Toast-written checkpoint with a task suspended inside
`call_function("eval", ...)` loads in Barn and resumes to Toast's result,
pinned by a test that passes Toast unchanged; the reverse direction is checked
and recorded; the generated lane has the kind; this note holds each run's
command and result and what is not verified.

Not included: push, PR, any GitHub post (the lead publishes).

## State

Done, committed locally (not pushed): the suite test on
`test/call-function-eval-import` in
`C:/Users/Q/code/moo-conformance-tests-call-function-eval`; the Barn fix,
engine test, Toast fixture, fixture doc and this note on
`fix/call-function-eval-import` in
`C:/Users/Q/code/barn-call-function-eval-import`.

## Source readings

Toast upstream `origin/master` = 2152ad4f6b57e82989c4fa110fc1ce70278f0cf7
(`git show origin/master:src/...` in `C:/Users/Q/src/toaststunt`). The oracle
binary is built from 03af4fa2; the lines below did not need to differ for the
result, and the recorded checkpoint confirms what the oracle writes.

- `verbs.cc:604-650` bf_eval: next==1 compiles, `setup_activ_for_eval` pushes
  the eval activation (`execute.cc:3420-3458`), returns `make_call_pack(2,
  nullptr)` (628); next==2 returns `{1, value}` (643-650).
- `execute.cc:3468-3500` bf_call_function: next==1 deletes the name and calls
  the named builtin with next 1 (3483-3484); on a BI_CALL package it wraps the
  callee's data in `cf_state{fnum, data}` (3493-3498). next>1 calls the saved
  builtin with the same next and the inner data (3486-3491).
- `execute.cc:2285-2290` OP_BI_FUNC_CALL BI_CALL: stores `bi_func_id` (here
  call_function's id), `bi_func_data` (the cf_state) and `bi_func_pc` (2) on
  RUN_ACTIV, which after bf_eval pushed is the eval activation.
- `execute.cc:3502-3510` bf_call_function_write: prints
  `bf_call_function data: fname = eval`, then `write_bi_func_data(nullptr,
  eval)`; eval registers no writer (`verbs.cc:678` plain `register_function`),
  so nothing follows.
- `execute.cc:310-336` unwind_stack FIN_RETURN of the eval activation: takes
  its bi_func_pc/id/data, pops it, and calls `call_bi_func(call_function,
  value, 2, ...)` on the caller; bf_call_function calls bf_eval(next=2), which
  returns `{1, value}`; that BI_RETURN is pushed on the caller's stack (334-336).
  So the `{1, value}` wrap is bf_eval's, reached through bf_call_function.
- `execute.cc:494-499` callers() names a builtin frame by
  `name_func_by_num(stack[i].bi_func_id)`: under call_function that is
  `call_function`.
- Barn `db/format/reader_task.go:296-310` already reads this as
  `Builtin{Name: call_function, State: "bf_call_function data: fname = eval",
  Inner: {Name: eval}}`. `vm/persistence.go:279-289`
  withoutEvalReturnContinuation drops only a plain `eval` mark, so the
  wrapper reaches ConvertNativeFrame, which refuses it (the issue's error).
- Generated lane: `internal/fuzzseed/suspended_stack.go:21-23,355-359` already
  has eval variant 2 = `call_function("eval", ...)`; no new kind needed.

## Runs

Toast identity: `/root/src/toaststunt/build-release/moo` SHA-256
61dd1799d82f75e33cb9c42fe48bd1966199ad83fda2f6298e3a7f872cc613dd; Test.db
SHA-256 1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e.
Runner: `.tmp/cfe/run.sh LABEL FIRST RESTART` (not committed), run as
`wsl -d Debian -u root bash -lc 'bash .../.tmp/cfe/run.sh ...'`; it runs
`UV_PROJECT_ENVIRONMENT=/root/.cache/barn-cfe-conformance-linux uv run
--project <suite worktree> --frozen moo-conformance
src/moo_conformance/_tests/server/suspended_call_function_eval_checkpoint.yaml
-m "admission or kept_db_restart" --server-command <first>
[--restart-server-command <second>] [--target-profile-manifest=<oracle> when
Toast is first] --server-db=<suite>/_db/Test.db --server-db-dir=<suite>/_db/startup
--moo-host=127.0.0.1 --oracle-profile-manifest=profiles/toast/stock-wsl-testdb.json
--fail-on-unexpected-skip --strict-markers -p no:cacheprovider -v`. Toast
command: `.tmp/cfe/capture_toast.sh {db} {port} <dir>` (copies each boot's
input db, then execs Toast `{db} {db}.new -p {port}`). Barn command: the
canonical `bin/barn-linux serve ... --log-dir=<out>/logs`.

1. Toast writes, Toast resumes (new suite test, unchanged): `2 passed in
   33.18s`. Checkpoint the restart booted from: 5461 bytes. The eval
   activation is saved as pc line `47 2 45`, `call_function`,
   `bf_call_function data: fname = eval`, nothing after. Its `stack`
   (callers() names inside the eval'd program before suspending) is
   `{"call_function", "start", ""}`. Saved as
   `engine/testdata/toast-suspended-call-function-eval.db` (SHA-256
   ad1bd19d6162be3e0fb0de43700e8bc5c5e5c9d5f9d13189db4ed20b7e256259).
2. Toast writes, Barn resumes, Barn linux build of unchanged origin/master code
   (`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/barn-linux
   ./cmd/barn`, SHA-256 f6624ebe...): `1 failed, 1 passed in 18.19s`; Barn
   exits at load: `level=ERROR msg="Barn failed" err="load database: restore
   suspended tasks: suspended task 2088686: native frame 2: native builtin
   continuation requires conversion"` (the issue's error).
3. `go test ./engine -run 'TestToastSuspendedCallFunctionEvalImportReturnsEvalResult$|TestToastSuspendedEvalImportReturnsEvalResult$' -count=1`
   without the fix: FAIL `suspended task 1150946788: native frame 2: native
   builtin continuation requires conversion`. With the fix (plus the move
   import tests): 5 PASS, `ok github.com/MongooseMoo/barn/engine 0.684s`.
4. Fixed Barn linux build (SHA-256 382b97f4...), new suite test:
   - Toast writes, Barn resumes: `2 passed in 32.41s`.
   - Barn writes, Toast resumes: `2 passed in 32.50s`.
   - Barn writes, Barn resumes: `2 passed in 33.93s`.
   Barn logs of all three: 0 `Toast could not load`, 0 `failed to restore`,
   0 ERROR records. Barn's checkpoint (the one Toast booted from) writes the
   eval activation as `47 2 45` / `eval` (plain eval mark, no call_function
   wrapper).
5. callers() probe: a scratch copy of the test (deleted, never committed)
   whose last step returns `{before, after, result, finished, stack,
   resumed_stack}` and expects "probe", so the failure prints the value.
   `resumed_stack` is callers() names in the eval'd program after it resumes.
   - Toast to Toast: `[1, 1, [1, 73], 1, ['call_function', 'start', ''], ['call_function', 'start', '']]`
   - Barn to Barn: `[1, 1, [1, 73], 1, ['eval', 'eval'], ['start', '']]`
   - Toast to Barn: `[1, 1, [1, 73], 1, ['call_function', 'start', ''], ['start', '']]`
   - Barn to Toast: `[1, 1, [1, 73], 1, ['eval', 'eval'], ['eval', 'start', '']]`
   Barn without a server (`bin/moo` on Minimal.db, a `#0:start` verb storing
   `{eval(P), call_function("eval", P)}` where P collects callers() names):
   `=> {{1, {"eval", "eval"}}, {1, {"eval", "eval"}}}`.
6. Generated lane: `printf '\x00\x00\x01\x02\x00' | bin/moo-fuzz-source
   --generator suspended-stack` prints `r1 = call_function("eval", "suspend();
   v = 0; return {1, v};");`: the issue's choices `0000010200` are the
   existing seed `eval/2` of `suspendedStackSeeds` (`vm/suspended_stack_test.go:16-33`).
   `go test ./vm -run 'TestSuspendedStackLayersMarkResult$|TestSuspendedStackKindsReachCheckpoint$|TestCheckpointReloadsEvalCalledThroughCallFunction$|TestCheckpointExportsSuspendedEvalWithToastContinuation$|FuzzSuspendedStackCheckpoint$' -count=1 -v`:
   all PASS incl. `TestSuspendedStackLayersMarkResult/eval/2`,
   `ok github.com/MongooseMoo/barn/vm 8.090s`.
7. `go test ./engine -run 'Toast|Native|Checkpoint' -count=1`: ok 3.579s.
   `go test ./vm -run 'Native|Persist|Export|Import' -count=1`: ok 2.039s.
   `gofmt -l vm engine`: no output. `go vet ./vm ./engine`: clean.

## Decisions and why

- Fix: `vm/persistence.go` withoutEvalReturnContinuation unwraps a
  `call_function` mark whose line is exactly
  `bf_call_function data: fname = eval` to its inner eval mark, then applies
  the existing eval checks (pc 2, no state, no inner). Same shape as
  withoutMoveContinuation's call_function unwrap. Toast at pc 2 under
  call_function only calls bf_eval(next=2), so the result is the same `{1,
  value}`.
- Durable test lives in the suite (`server/suspended_call_function_eval_checkpoint.yaml`,
  kept_db_restart), so `scripts/cross-engine-checkpoint.sh`'s suite sessions
  run it in both directions once the suite change lands. It records callers()
  names in `stack`/`resumed_stack` without asserting them (as the
  call_function-move producer does), because the brief says not to fix
  callers() here.
- No new fuzzseed kind or seed: eval variant 2 is `call_function("eval",
  ...)` and seed `eval/2` is exactly the issue's program (run 6).
- Export left as is (plain `eval` mark for the call_function case): Toast
  resumes it to the same result (run 4, Barn writes, Toast resumes). Writing
  Toast's call_function wrapper would only change what callers() names after
  Toast resumes, which is the callers() finding below.

## Surprises

- The vm lane's seed eval/2 already passed before the fix: it round-trips
  through Barn's own export, which writes the plain eval mark, so it never
  reads Toast's call_function wrapper. Only a Toast-written checkpoint (the
  engine fixture, the suite test, the #975 cross-engine lane) exercises the
  import.

## Findings

1. Fixed: Toast's form of the eval activation under call_function (pc 2,
   builtin call_function, line `bf_call_function data: fname = eval`) was
   refused by Barn's import, failing the whole database load.
2. Not fixed (callers(), per brief). Inside the eval'd program under
   call_function("eval", ...), Toast's callers() names are
   `{"call_function", "start", ""}` before and after a restart. Barn's live
   callers() gives `{"eval", "eval"}` (two entries, not three: no `start`,
   and the builtin frame named `eval`); after Barn resumes (from either
   engine's checkpoint) `{"start", ""}` (the builtin frame is gone); after
   Toast resumes Barn's checkpoint `{"eval", "start", ""}` (Toast names the
   frame from Barn's plain eval mark). Barn's live `{"eval", "eval"}` is the
   same for a direct eval() (bin/moo, run 5), so it is not specific to
   call_function; Toast's direct-eval value was not run.

## Not verified

- Toast's callers() for a direct eval() (only the call_function case ran on
  Toast).
- Whole test suites, `-race`, CI, `scripts/cross-engine-checkpoint.sh` as a
  whole, and the #975 generated cross-engine lane on this branch.
- Mongoose's database: how many of its tasks are suspended in this form.

## Next step

None on this branch: the lead publishes. Open a callers() issue from Finding 2.
