# Fix: import and export Toast's move() builtin continuation

## Goal

Barn refuses to boot a ToastStunt checkpoint holding a task suspended inside a
hook verb that `move()` called (`where:accept` pc 2, `oldloc:exitfunc` pc 3,
`where:enterfunc` pc 4). Convert Toast's saved `bf_move` continuation into
Barn's existing move continuation model on import, and write it back in
Toast's form on export. Model: Barn PR 886 (eval continuation). Rows 1-3 of
`C:/Users/Q/code/barn/agent-notes/native-continuation-refusals-inventory.md`.

## State

Done and committed locally on `fix/native-move-continuation` (not pushed, no
PR): producers, fixtures, regression tests, import fix, export mirror.

### What changed

- `vm/persistence.go` `convertNativeSnapshot`: when the frame below is paused
  at a builtin call, `withoutMoveContinuation` recognises builtin `move`, pc
  2-4 and a well-formed `bf_move data` line on the frame, removes the mark and
  returns a `task.MoveContinuationSnapshot`; the converted frame then gets
  `IsVerbCall`, `DiscardReturn`, `MoveContinuation`, `BuiltinCaller = "move"`
  and `BuiltinPC`, as `pushMoveLifecycleVerb` leaves a live hook frame. Any
  other builtin mark still reaches `ConvertNativeFrame`'s refusal.
- `vm/persistence.go` `ExportNativeSnapshot`: a frame with a
  `MoveContinuation` is exported with builtin `move`, pc `Stage + 1` and the
  `bf_move data` line. A continuation whose what or where is not a plain
  object number is refused ("move continuation has no Toast form") and the
  task stays in Barn's frame form.
- `engine/task_load.go` `loadSuspendedTask`: after the VM is restored, each
  frame's `BuiltinCaller`/`BuiltinPC` is copied onto the task's activation,
  which is what `callers()` reads.
- No change in `compiler/`. `ExportNativeFrame` still refuses a move
  continuation it is handed; `ExportNativeSnapshot` removes the continuation
  before the call, as the import removes Toast's mark before
  `ConvertNativeFrame`.

### Producers (Toast-green, Barn-green)

`tests/mongoose-conformance/server/suspended_move_{accept,exitfunc,enterfunc}_checkpoint.yaml`.
Each builds a non-wizard `owner`, a driver `o`, `thing` inside `origin`, and
`dest`; `dest:accept`, `origin:exitfunc`, `dest:enterfunc` each append their
verb name to `o.log`; the hook under test also calls `suspend(20)`, then
appends `"<verb> resumed"` and stores the verb names of `callers()` in
`o.stack`. `o:start` runs `this.result = {move(this.thing, this.dest)};
this.finished = this.finished + 1;`. All verbs are owned by the non-wizard
`owner`, so at pc 2 the value `accept` returns after the restart decides the
move (`objects.cc:153-155`).

Identities: stock Toast HEAD `03af4fa2b53b418999f03566b646ee42bc7ea647` (the
manifest's `implementation_ref`); binary `/root/src/toaststunt/build-release/moo`
SHA-256 `61dd1799d82f75e33cb9c42fe48bd1966199ad83fda2f6298e3a7f872cc613dd`;
conformance checkout `C:/Users/Q/code/moo-conformance-tests` at `dcda6c19`;
Test.db SHA-256
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.

Command, inside WSL Debian as root (`.tmp/native-move/run_producer.sh`, not
committed), with `repo=/mnt/c/Users/Q/code/barn-native-move`:

```
UV_PROJECT_ENVIRONMENT=/root/.cache/barn-native-move-conformance-linux \
uv run --project /mnt/c/Users/Q/code/moo-conformance-tests --frozen moo-conformance \
  --candidate-root=$repo --moo-suite-root=$repo/tests/mongoose-conformance \
  --moo-suite-path=server/suspended_move_<hook>_checkpoint.yaml \
  --server-command "<server>" \
  --server-db=$repo/.tmp/conformance/db/Test.db --server-db-dir=$repo/.tmp/conformance/db/startup \
  --moo-host=127.0.0.1 --oracle-profile-manifest=$repo/profiles/toast/stock-wsl-testdb.json \
  --fail-on-unexpected-skip --strict-markers --admission-evidence-output=<out>/admission.json \
  --admission-evidence-context=native-move:<engine>:<label> --junitxml=<out>/results.xml -v
```

- Toast `<server>`: `bash $repo/.tmp/native-move/capture_toast.sh {db} {port}
  <capture-dir>`, which copies `{db}` to the capture directory and then execs
  `/root/src/toaststunt/build-release/moo {db} {db}.new {port}`; plus
  `--target-profile-manifest=$repo/profiles/toast/stock-wsl-testdb.json`. One
  session per file.
- Barn `<server>`: `$repo/bin/<binary> serve --db {db} --listen
  tcp://127.0.0.1:{port} --config=$repo/profiles/barn/outbound-on.conf
  --profile-id=barn-linux-testdb-outbound-on --profile-manifest={manifest}
  --log-dir=<out>/logs`. One session, all three files.

Raw results (each session includes `test_capability_admission`; the warning is
pytest's record_property/xunit2 notice):

| Engine | Result |
|---|---|
| Toast, accept | `2 passed, 1 warning in 36.69s` |
| Toast, exitfunc | `2 passed, 1 warning in 33.02s` |
| Toast, enterfunc | `2 passed, 1 warning in 42.21s` |
| Barn before the fix (`origin/master` code, linux build) | `4 passed, 1 warning in 81.76s` |
| Barn with the fix | `4 passed, 1 warning in 76.33s` |

Barn before the fix passes because it reads back its own Barn-form frames: its
logs hold 6 WARN `Toast could not load this checkpoint: task kept in Barn's
frame form ... builtin continuation has no Toast form` records (two
checkpoints per producer). With the fix that count is 0, restore failures 0,
ERROR records 0, so the Barn-after run boots from Toast-form move frames that
Barn wrote and imported.

### Fixtures

The database each Toast restart booted from. Described in
`engine/testdata/toast-suspended-move.md`.

- `engine/testdata/toast-suspended-move-accept.db` SHA-256
  `430c730bfc4c722b2eb26e8b98a5f85683e7bcabfa72aa2e71c7851cc80f59ea`, hook frame
  ends `22 2 20`, `move`, `bf_move data: what = 11, where = 13, position = 0`
- `engine/testdata/toast-suspended-move-exitfunc.db` SHA-256
  `80fa40bafb261708520efa6647b2241af1cf6b8bcbf3ac247badf3de938ea205`, `22 3 20`
- `engine/testdata/toast-suspended-move-enterfunc.db` SHA-256
  `4e3a934cefa184bec8c86968993d2ba2b20a539590a0a041aca2bd5af9675d17`, `22 4 20`

### Go regression tests

`engine/toast_suspended_move_import_test.go`, run with `go test ./engine -run
'TestToastSuspendedMove' -count=1 -v`.

- `TestToastSuspendedMoveImportFinishesMove` (accept, exitfunc, enterfunc):
  imports each fixture, drives the task, asserts Toast's outcome (log, the
  resumed hook's `callers()`, final location, `{0}` result, finished).
  On the unfixed tree, all three:
  `suspended task <id>: native frame 2: native builtin continuation requires conversion`.
  With only the import conversion and not the `task_load.go` change, all three
  failed on `stack={"start", ""}`, want `{"move", "start", ""}`.
- `TestToastSuspendedMoveExportsToastContinuation`: imports each fixture,
  takes the checkpoint snapshots, and requires every frame's pc, builtin pc,
  error pc, builtin name and data, and operands to equal the Toast fixture's;
  writes the checkpoint and requires the file to hold the hook frame's three
  closing lines exactly as Toast wrote them; reloads the file and finishes the
  move. With the export change disabled, all three:
  `frame 0 was not written in Toast's form`.
- With the fix: both tests pass, 6 subtests.
- `gofmt -l` on the three Go files: clean. `go vet ./vm ./engine`: clean.

### Added acceptance condition: hook frame as a middle frame

The lead added: the export must work when the move hook frame is a middle
frame at any depth, because PR 891's `fuzzseed.SuspendedStack` kinds
`move-accept`, `move-exitfunc`, `move-enterfunc` suspend several frames above
the hook, and PR 892's fuzz lanes fail on any `ExportNativeSnapshot` error.

`TestBarnMoveHookMiddleFrameCheckpointsAsToastContinuation` (same test file):
a Barn-started task `program -> start (move) -> hook -> deep (for loop, try
body) -> pause (suspend)`, for each hook. The checkpoint must hold all five
frames in Toast's form, with builtin `move`, pc 2/3/4 and `bf_move data: what
= 3, where = 5, position = 0` on frame 2 only; the written file is reloaded,
imported and run to the end (hook log, finally count, final location, move's
0 result). It passed with no production change beyond the one already made:
3 subtests PASS. My first draft of it failed because `start` was `#0.log =
{@#0.log, move(...)}`, which reads the log before the hooks append to it; that
was the test's error.

PR 891 was still OPEN (not merged) when I checked, twice, so I did not merge
master or run `TestSuspendedStackKindsReachCheckpoint`. The combined check
with 891 and 892 has not been run.

### Lead's correction on CI

CI's "Run local conformance regressions" step selects four explicit files and
nothing under `server/`; a separate cross-engine job will run the `server/`
producers, and CI is not to be changed here. The producers pass on unfixed
Barn in a Barn-only run (results table above), so the red-before evidence is
the Go tests on the Toast-written fixtures plus the `Toast could not load this
checkpoint` log records before the fix (6) and their absence after (0).

## Decisions and why

- The move mark is accepted when the frame below is paused at any builtin
  call other than eval() or pass(), not only at a direct `move(...)` call.
  Barn's `call_function("move", ...)` calls `builtinMove` directly, so its
  hook frame is the same move continuation under a `call_function` call site.
  The export writes that as a plain `move` mark; if the import demanded a
  direct move() call site, Barn could no longer reload such a task from its
  own checkpoint, which it can today in Barn's frame form. Toast itself writes
  that case as `call_function` wrapping `move` (inventory row 9), which is
  still refused on import and is the call_function work that follows this.
- `Decentralized = true` on import: `builtinMove` computes it as
  `!ctx.LiveStoreMutated` (`builtins/objects_movement.go:69`), and a task just
  restored has not mutated the live store. It is read only at stage 1
  (`ApplyMoveLifecycle`).
- `OldLocation` is not converted either way. Toast's `oldloc` is a C local
  reset on every entry (`objects.cc:121`), and Barn reads `OldLocation` only
  in the stage 1 code that sets it (`vm/move_lifecycle.go:94-102`).
- The state line is accepted only if printing the parsed numbers reproduces
  it exactly, so a malformed line is refused instead of half-read.
- Hook verbs are owned by a non-wizard so the pc 2 producer depends on the
  value `accept` returns after the restart; under a wizard programmer Toast
  ignores it.
- One YAML file per pc, each Toast run in its own managed session, so each
  fixture is a fresh Test.db plus one producer's objects.
- The task's activations get the builtin caller in `loadSuspendedTask`
  because the conversion runs in `vm.RestoreVMSnapshot`, which does not hold
  the task. For a Barn-form frame the reader already built both from the same
  metadata, so the copy changes nothing there.

## Surprises

- First Toast run of the accept producer failed in setup with E_INVARG: I had
  named driver properties `t`, `a`, `c`, and `a` is a builtin property on
  Toast. The test was wrong; renamed to `thing`, `origin`, `dest`. Barn was not
  run on the wrong draft.
- `.github/workflows/ci.yml:239-245` runs `tests/mongoose-conformance` with an
  explicit `--moo-suite-path` list of four `builtins/` and `root/` files. No
  `server/*.yaml` file (including PR 886's `suspended_eval_checkpoint.yaml`) is
  in that list, so CI does not run these producers unless the list is extended.
  I have not changed CI.
- Separate Barn bug, found while settling inventory U4, not fixed here. When
  `where:accept` raises under a wizard programmer, Toast still moves the
  object and the error propagates: `unwind_stack` re-enters `bf_move` with 0
  (`execute.cc:372-404`) and `do_move` case 2 moves it (`objects.cc:153-171`).
  Barn calls `resumeMoveLifecycle` only from `vm.Return`
  (`vm/stack.go:179-192`), so an error leaving the hook frame skips it and
  nothing moves. Evidence, managed harness, uncommitted probe
  `.tmp/native-move/suite/builtins/move_accept_error_probe.yaml` (`dest:accept`
  is `raise(E_PERM);`, wizard-owned; driver catches the error and returns
  `{code, thing.location == dest}`; expectation `["E_PERM", 1]`): Toast `2
  passed, 1 warning in 12.49s`; Barn `expected value ['E_PERM', 1], but got
  ['E_PERM', 0]`. By the same Toast code the exitfunc and enterfunc cases do
  not differ in location (the move has already happened), but Toast then
  pushes and immediately pops the remaining hooks; I did not probe those. A
  converted continuation is the same Barn continuation, so it has the same
  difference as a live one; the conversion needs nothing extra for it.

## Unverified

- Cross-engine: Toast loading and resuming a checkpoint that Barn wrote with a
  move continuation has NOT been run. The export is proven only in-process
  against the Toast fixtures (same pc line, name and data line) and by Barn
  re-importing its own output.
- A Barn-written plain `move` mark under a `call_function("move", ...)` call
  site: from `execute.cc:326-336` Toast calls `bf_move` from the callee's
  `bi_func_id` and pushes its result on the caller's stack, so I expect it to
  resume correctly, with `callers()` naming `move` where a Toast-written file
  would name `call_function`. Not run on Toast or Barn.
- `move()` with a nonzero position, and a move whose what/where is recycled
  while the hook is suspended, were not produced; the fixtures all have
  position 0.
- The full Go suite, staticcheck, golangci-lint and the full conformance suite
  were not run locally; CI runs them.
- The conformance checkout used is the workstation's main checkout at
  `dcda6c19`, not necessarily the ref CI resolves from `main`.

## Next step

The lead pushes the branch, opens the PR, and runs the cross-engine check
(Barn writes, Toast resumes) with the harness feature being built for it.
Whether to add the three producers to the CI `--moo-suite-path` list is the
lead's call.
