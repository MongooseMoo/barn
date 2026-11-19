# Fix: import and export of a task suspended inside a `finally` body

## Goal

Barn boots a ToastStunt checkpoint that holds a task suspended inside a
`finally` handler body, for each of Toast's reasons (FIN_FALL_THRU, FIN_RAISE,
FIN_UNCAUGHT, FIN_RETURN, FIN_EXIT), resumes it with Toast's outcome, and writes
such a task back in Toast's form.

Shape of the work follows Barn PR 886 (suspended eval): Toast-passing producer
YAML, Toast-written fixture with `.md` and SHA-256, Go regression test red
before and green after, import fix, export mirror.

## State

- First agent: read Toast and Barn, ran the Toast oracle, built the fix, was stopped before committing.
- 2026-11-18 Resumed by a second agent. The first was stopped mid-work with
  nothing committed. Verifying the working tree against this note before
  trusting it.
- 2026-11-18 Branch fast-forwarded to origin/master with the work carried
  over; no conflicts. `go build ./...` and `go vet` clean, `gofmt -l` empty,
  `staticcheck` silent, `golangci-lint` 0 issues. `go test ./compiler ./task
  ./types ./db/... ./engine` ok. `go test ./vm` FAILS in two tests that
  existed before this work: `TestExceptionReasonPendingFinallyRetryAndCheckpoint`
  (`exception_reason_test.go:126: pending error = <nil>`) and
  `TestFinallyNestedPendingRootsCloneAndInvalidState`
  (`finally_control_persistence_test.go:176: malformed nested pending state
  accepted`). Both suspend in the finally body of an error nothing catches, so
  `forgetUncaughtErrors` now turns their pending error into an abort.
  Confirmed by commenting out the one call in `vm/op_misc.go`: both vm tests
  pass, and `TestUncaughtErrorSuspendedInFinallyEndsUnreported` fails with
  `stack [1 {E_PERM, "suspended", 0, {}}], want reason 2 and value 0`. The
  call is restored.
- 2026-11-18 Green with the tree as it stands: `go test ./engine -run
  'TestToastSuspendedFinally|TestBarnSuspendedTaskShapesCheckpointAsToastFrames$'
  -count=1` prints `ok github.com/MongooseMoo/barn/engine 2.200s`; the whole
  of `./engine`, `./compiler`, `./task`, `./types`, `./db/format`,
  `./db/store` ok. The export with frames above the finally frame is the
  shape "verb call in a finally body a return entered" in `toastExportShapes`
  and the called-verb fixture's write-back check; both pass.
- 2026-11-18 The lead chose A (below). Done and committed on
  `fix/native-finally-reasons`; not pushed, no PR (the lead does both).
  Final results on the committed tree:
  - `go test ./vm ./compiler -count=1`: `ok github.com/MongooseMoo/barn/vm
    28.873s`, `ok github.com/MongooseMoo/barn/compiler 2.279s`.
  - `go test ./engine -run
    'TestToastSuspendedFinally|TestBarnSuspendedTaskShapesCheckpointAsToastFrames$'
    -count=1`: `ok github.com/MongooseMoo/barn/engine 2.696s`.
  - `go test ./engine ./task ./types ./db/... -count=1`: all `ok`.
  - `gofmt -l` on the changed Go files prints nothing; `go vet ./compiler
    ./vm ./engine ./task ./types ./db/...` exits 0; `golangci-lint` 0 issues.
  - Producer on fixed Barn, managed harness (command under "Barn before the
    fix", run directory `.tmp/finally-runs/barn-final`, binary
    `bin/barn-linux-amd64` built from this tree, SHA-256
    `507a946e4e1e9260b240836967d8c7c76222b691724746e2651920b927d2276f`, input
    Test.db SHA-256 as above): `8 passed in 179.33s (0:02:59)`, the uncaught
    test among them. Before the fix it was `1 failed, 7 passed`.

## Decision on the two vm tests (lead, 2026-11-18: A)

Two vm tests written before this work suspended in the finally body of an
error nothing catches and pinned that the error survives the suspension and
that resuming returns it. Toast drops that error at the suspension (the
FIN_UNCAUGHT facts under "Toast run", from the managed oracle run recorded
there: the rest of the finally body runs, the statement after `endtry` does
not, `#0:handle_uncaught_error` is not called). With `forgetUncaughtErrors`
live they failed: `exception_reason_test.go:126: pending error = <nil>` and
`finally_control_persistence_test.go:176: malformed nested pending state
accepted`. What changed:

- `TestFinallyNestedPendingRootsCloneAndInvalidState`
  (`vm/finally_control_persistence_test.go`): the program gained an enclosing
  `try ... except (E_TYPE) endtry`, so its error is a handled one. No
  assertion changed: the clone, root-collection, five malformed-state and
  discard checks are as they were.
- `TestExceptionReasonPendingFinallyRetryAndCheckpoint`
  (`vm/exception_reason_test.go`): the program gained an enclosing `try ...
  except e (E_MAXREC) return e[1]; endtry`. The pending-error, retry-restore
  and checkpoint-reload assertions are unchanged. Its two
  `requireExceptionReason(t, <vm>.Resume(), ExceptionReasonFrameDepth)`
  assertions (live VM and VM restored from the checkpoint) became
  `requireCaughtMaxrec`: resume returns E_MAXREC from the except arm, which
  shows the pending error reached the handler after the finally body.
- New `TestUncaughtErrorDroppedWhenFinallySuspends` (same file) runs the
  first test's original program and asserts Toast's outcome: the suspended
  frame holds one `PendingAbort` with no error, and resuming, live and from
  the persisted snapshot, ends as an exception flow that is `Unreported`
  with no error code. That the handler is then not called is asserted at the
  engine level by `TestUncaughtErrorSuspendedInFinallyEndsUnreported` and by
  the producer's uncaught test.

The rejected alternative B was to drop the live change and treat only an
imported FIN_UNCAUGHT as an abort: no existing test would change, but Barn
would still call `#0:handle_uncaught_error` where Toast does not.

## Findings from reading

Toast, `/root/src/toaststunt/src/execute.cc` at the source revision named by
`implementation_ref` in `profiles/toast/stock-wsl-testdb.json`:

- Reasons enum, lines 107-112: FIN_FALL_THRU 0, FIN_RAISE 1, FIN_UNCAUGHT 2,
  FIN_RETURN 3, FIN_ABORT 4, FIN_EXIT 5. Comment says the numbers are stored
  in the DB.
- `unwind_stack` (246-410): on a TYPE_FINALLY marker it sets pc to the
  handler, replaces the marker with INT `why`, then pushes `value` (269-276).
- `raise_error` (536-566): FIN_RAISE value is the 4-list
  `{code, msg, value, stack list}`. For FIN_UNCAUGHT the 5-list goes into the
  static `handler_verb_args` with name `handle_uncaught_error` and the value
  pushed is 0 (561-564).
- `EOP_END_FINALLY` (2609-2620): pops the marker, pushes FIN_FALL_THRU and 0.
- `EOP_CONTINUE` (2623-2645): pops value and reason; FALL_THRU does nothing,
  the others call `unwind_stack(why, v)` again.
- `EOP_EXIT` (2659-2670): value is `{stack depth, target pc}`; unwind stops at
  `base_rt_stack + depth` and sets pc (264-265, 305-308).
- `run_interpreter` resets `handler_verb_name` to null on every entry
  (3117-3120) and calls the in-DB handler only if it is set after `run()`
  (3193). So a FIN_UNCAUGHT task that suspends in its finally body loses the
  handler call on resume. This is not specific to a restart: any suspend
  there does it, in a live Toast too. To be verified on the oracle.

Barn:

- Refusal on import: `compiler/native_import.go:260-263`. On export:
  `compiler/native_export.go:34-35` (frame holds PendingReasons) and `:94-98`
  (call site inside a finally handler).
- Layout: `compiler/native_layout_stmt.go:218-240` pushes
  `NativeFinallyReason`, `NativeFinallyValue` at the handler label and records
  a `NativeFinallyHandler` control.
- Runtime: `vm/control.go:397-448` `pendingReason` and `executeEndFinally`;
  entries are appended at `vm/stack.go:120` (return), `vm/vm.go:1593`
  (error), `vm/control.go:481` (jump). Snapshot form is
  `task.PendingReasonSnapshot` (`task/snapshot.go:140-`), validated at
  `vm/persistence.go:436-462`.

## Toast run (managed harness, stock WSL oracle)

Producer: `tests/mongoose-conformance/server/suspended_finally_checkpoint.yaml`,
seven tests: fall-through, called-verb (FIN_RETURN with the suspension in a
verb the finally body calls, so the finally frame is a middle frame), raise,
uncaught, return, break (list loop), continue (range loop).

Command, run inside WSL Debian as root, cwd
`/mnt/c/Users/Q/code/barn-native-finally/.tmp/finally-runs/toast-1`:

```
UV_PROJECT_ENVIRONMENT=/root/.cache/barn-native-finally-conformance-linux \
uv run --project /mnt/c/Users/Q/code/moo-conformance-tests --frozen moo-conformance \
  --candidate-root=/mnt/c/Users/Q/code/barn-native-finally \
  --moo-suite-root=/mnt/c/Users/Q/code/barn-native-finally/tests/mongoose-conformance \
  --moo-suite-path=server/suspended_finally_checkpoint.yaml \
  --server-command 'bash <scratchpad>/toast_capture.sh <rundir>/capture {db} {port}' \
  --server-db=<rundir>/db/Test.db --server-db-dir=<rundir>/db/startup \
  --moo-host=127.0.0.1 \
  --oracle-profile-manifest=<worktree>/profiles/toast/stock-wsl-testdb.json \
  --target-profile-manifest=<worktree>/profiles/toast/stock-wsl-testdb.json \
  --fail-on-unexpected-skip --strict-markers -v
```

`toast_capture.sh` copies `{db}` to `capture/boot-N.db` and then execs
`/root/src/toaststunt/build-release/moo {db} {db}.new -p {port}` (binary
SHA-256 `61dd1799d82f75e33cb9c42fe48bd1966199ad83fda2f6298e3a7f872cc613dd`,
source revision as in `profiles/toast/stock-wsl-testdb.json`). Input Test.db SHA-256
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.

Raw result: `8 passed in 170.47s (0:02:50)` (capability admission plus the
seven tests, each PASSED).

Toast facts this run established:

- FIN_UNCAUGHT: after the restart the task ran the rest of the finally body
  (`after` 1), did not run the statement after `endtry` (`finished` 0), and
  `#0:handle_uncaught_error` was not called for it. The same error raised in a
  task that did not suspend (`o:control`) did reach the handler. The test
  replaces the handler with a recorder and asserts the recorded messages are
  exactly `{"finally control"}` before and after the restart.
- Saved operands, read from the captured files: fall-through `0, 0`; return
  `3, 5`; uncaught `2, 0`; raise `1, {E_PERM, "finally raise", 7, {stack}}`
  above the outer try's codes, label and catch marker; break `5, {0, 77}`
  above the loop's list and cursor.

Fixtures (the database each restart booted from), `engine/testdata/`:

| File | SHA-256 |
|---|---|
| toast-suspended-finally-fall-through.db | 4b6a3a1564b4e230fa6659b3a88a2d52f52a974d204c57cdf3c983af35309c39 |
| toast-suspended-finally-called-verb.db | 679bc6b2f520fe46aa5b8d979e3326243b6035b6692966285f7c86e90f160bf2 |
| toast-suspended-finally-raise.db | 9044f35900904ba2da2c07766c6fd999a1145c83700a3098990b971bafce4a27 |
| toast-suspended-finally-uncaught.db | 247d0350bb0ca36dcd2f70b83ae6b148d133e4619fe741b14c65d46e80023c73 |
| toast-suspended-finally-return.db | fd12b742469e8fe607e7219f43b0a1c1501aebdd0018dc79051ca3c155abcff5 |
| toast-suspended-finally-break.db | d466f7040b8d55a8fefa79da5051407c68e55a626b096243f867bfd88b8cfaed |
| toast-suspended-finally-continue.db | 40439f212e59b770e95cce163e40546f0cbfbe2c7208d9d485826541807056f2 |

These are the hashes of the files on disk, computed 2026-11-18, and the ones
the Go test and `engine/testdata/toast-suspended-finally.md` pin. That record
names a later Toast run than the one above (`8 passed in 175.18s`); the table
this note first held was from the earlier run and matched no file on disk.

## Barn before the fix

Go test `TestToastSuspendedFinallyImportContinuesTheReason` (`go test ./engine
-run 'TestToastSuspendedFinallyImportContinuesTheReason$' -count=1`), all
seven subtests red with the refusal, for example:

```
--- FAIL: .../fall_through
    suspended task 1860448623: native frame 1: native pending finally reason requires conversion
--- FAIL: .../return,_suspended_in_a_called_verb
    suspended task 26631211: native frame 2: native pending finally reason requires conversion
```

Producers on unfixed Barn (same harness command, `--server-command
'<worktree>/bin/barn-linux-amd64 serve --db {db} --listen
tcp://127.0.0.1:{port} --config=<worktree>/profiles/barn/outbound-on.conf
--profile-id=barn-linux-testdb-outbound-on --profile-manifest={manifest}
--log-dir=<rundir>/logs'`, no `--target-profile-manifest`):
`1 failed, 7 passed in 176.09s`. Barn restarts from its own frame form, so six
producers pass without the import. The failure is the uncaught one:

```
expected value [1, 1, 0, 1, ['finally control']],
but got [1, 1, 0, 1, ['finally control', 'finally uncaught']]
```

That is a live difference, not an import one: Barn calls
`#0:handle_uncaught_error` for an uncaught error whose finally body
suspended; Toast does not.

## The fix (as built; line numbers are of the working tree)

Import (`compiler/native_finally.go importFinallyReasons`, called from
`compiler/native_import.go` where the refusal was): for each finally body the
call site is inside, read Toast's two operands and build Barn's pending
reason: 0 none; 3 `PendingReturn` with the value; 1 `PendingError` holding
Toast's `{code, message, value, stack}`; 5 `PendingJump`; 2 the new
`task.PendingAbort`.

- The finally body's Barn identity (its handler address) comes from the
  `OP_TRY_FINALLY_WIDE` the import layout now records per try statement
  (`native_barn_layout.go finallys`).
- A loop exit's `{stack depth, pc}` is matched to a break or continue
  statement: the Toast layout now lists each one's operand pair
  (`NativeVectorLayout.Exits`, filled in `native_layout_stmt.go exit`), and the
  import layout records each one's `OP_UNWIND_JUMP` (`exits`), whose target and
  handler depth are Barn's jump. The jump's saved stack is the frame's operands
  up to the target's verified height.

Export (`exportFinallyReasons`, called from `compiler/native_export.go`; both
refusals removed): the inverse, from the frame's pending reasons to the two
operands. A pending reason that matches no finally body at the site, or a
jump no break/continue makes, still refuses.

FIN_UNCAUGHT (`task.PendingAbort`, `vm/control.go`, `vm/vm.go`,
`vm/error.go unreportedAbort`, `types.Result.Unreported`,
`engine/task_runtime.go`): Toast saves no error for this reason, so there is
nothing to put in a `PendingError`. A finished finally body with a
`PendingAbort` re-raises `unreportedAbort`, which runs enclosing finally
bodies, is caught by nothing, is not turned into a value by a verb without the
d flag, and ends the task as killed with no handler call and no traceback.

Live Barn had the same difference from Toast without any checkpoint (the red
producer above), so the same kind is used live: an error entering a finally
body with no handler left in any frame is marked `Uncaught`
(`vm/error_observation.go uncaught`, Toast's `find_handler_activ`), and when
the task suspends the mark becomes a `PendingAbort` (`vm/control.go
forgetUncaughtErrors`, called at the builtin suspend yield in
`vm/op_misc.go`). Without a suspension nothing changes: the error is re-raised
and reported as before.

Go test after the fix: `go test ./engine -run
'TestToastSuspendedFinallyImportContinuesTheReason$' -count=1 -v`, all seven
subtests PASS. Each subtest also checks that the restored task checkpoints as
frames with Toast's pc, error pc and operands (the in-process export proof).

More Go coverage added with the fix:

- `TestUncaughtErrorSuspendedInFinallyEndsUnreported` (same file): a
  Barn-started task, no Toast file. Red with the `forgetUncaughtErrors` call
  removed: `frame written in Toast's form = true with stack [1 {E_PERM,
  "suspended", 0, {}}], want reason 2 and value 0`. Green with it.
- Five shapes added to `toastExportShapes` in
  `engine/toast_task_export_test.go` (fall-through, return through a called
  verb, handled error, break, continue): a Barn-started task paused in a
  finally body is written in Toast's form, reloads, imports and finishes.
  `TestBarnSuspendedTaskShapesCheckpointAsToastFrames` PASS, 20 subtests.
- `barnFormProgram` in that file was a task suspended in a finally body, used
  as the example of a shape Barn cannot write in Toast's form. It now can, and
  `TestCheckpointKeepsBarnFormTaskBesideToastFormTasks` failed with `task 7202
  written in Toast's form = true`. I replaced the example with the
  path-dependent push-clear shape that `TestExportRefusesPathDependentPushClear`
  already pins as unexportable, and its marker text `"finally resumed"` with
  `"resumed"`. The assertions are unchanged.

## Another Barn bug found, not fixed here

A verb without the `d` flag that holds a `try ... finally` swallows an error
raised in a verb it calls. `vm/vm.go` executeLoop turns any error returned by
`vm.Execute(op)` in a frame with `VerbDebug` false into a pushed value, and
`OP_END_FINALLY`'s re-raise of the pending error (`vm/control.go
executeEndFinally`) is such an error. Evidence, `bin/moo.exe` on the in-memory
database, unfixed and fixed code alike:

```
add_property(#0, "x", 0, {#3, "rw"});
add_verb(#0, {#3, "rxd", "boom"}, {"this", "none", "this"});
set_verb_code(#0, "boom", {"raise(E_PERM);"});
add_verb(#0, {#3, "rx", "nd"}, {"this", "none", "this"});
set_verb_code(#0, "nd", {"try", "this:boom();", "finally", "this.x = this.x + 1;", "endtry", "this.x = this.x + 10;", "return 5;"});
try r = #0:nd(); except e (ANY) r = {"caught", e[1]}; endtry
return {r, #0.x};
=> {5, 11}
```

By my reading of Toast (`unwind_stack` continues from `EOP_CONTINUE` whatever
the frame's debug flag) Toast gives `{{"caught", E_PERM}, 1}`. Not run on
Toast. It matters here only in that an imported FIN_RAISE in a frame without
`d` meets the same bug; the new abort does not go through that path.

## Decisions and why

- The managed run's database must sit inside `--candidate-root`; the harness
  refuses otherwise ("candidate primary database escapes candidate checkout
  root"). Runs use `<worktree>/.tmp/finally-runs/<name>` (`.tmp` is ignored).
- FIN_UNCAUGHT is a kind of its own (`task.PendingAbort`) because Toast saves
  no error with it: there is nothing to rebuild a `PendingError` from.
- A loop exit is matched by Toast's `{stack depth, pc}` pair against the
  break and continue statements of the program, not decoded from the pc: any
  statement with that pair leaves the same loop the same way.
- The branch was brought up to master by fast-forward with the uncommitted
  work carried across, so it holds no merge commit.

## Surprises

- `.github/workflows/ci.yml:237-256` runs only four named
  `--moo-suite-path` files from `tests/mongoose-conformance`. It does not run
  `server/suspended_eval_checkpoint.yaml`, and it will not run this producer
  unless a `--moo-suite-path` line is added. I did not change CI.

- The first agent's fixture table in this note did not match the files on
  disk; the fixtures had been regenerated by a later Toast run and only the
  Go test and the fixture record were updated.
- Two vm tests written before this work pin Barn's old behaviour for an
  uncaught error whose finally body suspends (see Decision on the two vm
  tests).

## Next step

The lead pushes the branch and opens the PR. Open after that:

- CI does not run the producer (see Surprises); adding its
  `--moo-suite-path` line to `.github/workflows/ci.yml` is not done here.
- The bug under "Another Barn bug found, not fixed here" is unfixed and has
  not been run on Toast.
