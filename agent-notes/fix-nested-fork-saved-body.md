# Fix nested fork saved body

## Goal

A fork nested in a fork written on one source line must leave a queued child
task whose saved code compiles, so a checkpoint holding that task can be
reloaded and run. Save the fork body the way Toast does (unparse the fork
body's program), not by cutting source text.

## State

Reading done for Toast and for the saved text's users; design being chosen.

### What Toast does (WSL Debian, /root/src/toaststunt/src)

- `execute.cc:2115-2135`: OP_FORK reads `f_index` and calls
  `enqueue_forked_task2(RUN_ACTIV, f_index, when, id)`.
- `tasks.cc:1209-1227` and `tasks.cc:1258-1291`: the queued task keeps the
  parent program and `f_index`; no text is cut.
- `tasks.cc:1904-1914` `write_forked_task`: writes the header with
  `find_line_number(ft.program, ft.f_index, 0)`, the activation, the runtime
  environment, then `dbio_write_forked_program(ft.program, ft.f_index)`.
- `db_io.cc:417-421` `dbio_write_forked_program`:
  `unparse_program(program, receiver, nullptr, 1, 0, f_index)` then `.`:
  fully parenthesized, not indented, one line per receiver call.
- `unparse.cc:856-866` `unparse_program`: `decompile_program(p, f_index)` and
  `list_prg` of that statement list. The saved code is the unparse of the fork
  vector's statements, never source text.

### What Barn does

- `vm/control.go:150` `executeFork` sets `ForkInfo.SourceLines` from
  `sourceLinesForFork` (`vm/control.go:190`), which picks source lines by the
  body's line table and, when the body is on one line, calls
  `oneLineForkBody` (`vm/control.go:259`): text from the first `)` after the
  first `fork` to the first `endfork`. `oneLineForkBody` has that one caller.
- `ForkInfo.SourceLines` is used only for persistence:
  `task/snapshot.go:377` copies it to `ForkSnapshot.SourceLines`,
  `db/format/writer_task.go:83` writes it as the queued task's code,
  `db/format/reader_task.go:151` reads it back, `engine/task_load.go:153`
  compiles it (`CompileMOO(saved.Code)`) and `engine/task_load.go:177` puts it
  back on the restored task. `vm/retry.go:98` clones it for a retried fork.
- `bytecode.Program` holds no IR, only `Source` and `Origin.Fork` (one-based
  code-order index of the fork body in the compiled program,
  `bytecode/origin.go:15`, stamped by `extractForkBody`,
  `bytecode/program.go:360`).

### The fix

- The compiler records each fork statement as it emits its instruction
  (`compiler/lower_control.go` `compileFork`, `lowerer.forks`) and, after
  lowering, formats each body with the parser's unparser, fully parenthesized
  and not indented (`compiler/compiler.go` `forkSources`), into
  `bytecode.Program.ForkSources`, one entry per fork instruction in code
  order. Both compile paths fill it: `compileUncached` and
  `compileNativeVector` (`compiler/native_import.go`).
- An extracted fork body shares its parent's table
  (`bytecode/program.go` `extractForkBody`), as does a restored frame's
  program (`vm/persistence.go` `cloneProgram`), so a fork made by a nested
  body or by a frame restored from a checkpoint finds its text.
- `bytecode.ForkBody.SourceLines` (`bytecode/fork.go`) returns the entry named
  by the extracted body's `Origin.Fork`. `vm/control.go` `executeFork` uses it.
- Deleted from `vm/control.go`: `sourceLinesForFork`, `blockDepth`,
  `oneLineForkBody`, `indexASCIIFold`. None had another caller.

### Tests

- `vm/fork_saved_body_test.go`
  `TestForkBodyOnOneLineSurvivesCheckpoint`: checkpoint, reload and run
  compared with the uninterrupted run for a nested one-line fork, a fork
  nested two deep, `endfork` in a string, `fork` in a string before the fork,
  a second fork on the line, a fork made by a restored frame, and a one-line
  fork nested in a body of several lines.
  `TestForkSavedCodeIsUnparsedBody` pins the saved lines.
- Before the fix: five of the cases fail. Four with
  `queued child 2 does not compile` (`Line 2:  syntax error`, or
  `Missing quote`). "second fork on the line" compiled and ran the wrong
  body: the second fork's child was saved with the first fork's text, and
  the property ended `{1, 1}` where the uninterrupted run gives `{1, 2}`.
- The two fuzz inputs are in `suspendedStackMixedSeeds`
  (`vm/suspended_stack_test.go`). Without the production change
  `FuzzSuspendedStackCheckpoint/seed#38` and `seed#39` fail with
  `queued child 2 does not compile: [Line 2:  syntax error]`; with it they
  pass.

## Decisions and why

- The text is made at compile time, not at fork time or checkpoint time.
  `bytecode.Program` keeps no IR, so a later unparse would parse the source
  again, on every fork or under the task lock at every checkpoint. The compile
  already holds the IR and is cached. The old path lexed source lines on every
  fork; the new one is a table lookup.
- Fork statements are recorded where their instruction is emitted, not by
  walking the IR, so the table's order is the code order `Origin.Fork` counts
  by construction.
- Floats in the saved body are spelled exactly (no `ToastFloats`), as in
  stored verb code. Toast's unparser prints `%.15g`; that would change a value
  across a checkpoint.
- A body the unparser cannot format is a compile error, not a missing entry.
  Parsed source cannot hold what the formatter rejects (NUL, a newline in a
  string), so this is not expected to fire.

## Surprises

- The text cut was also wrong without any nesting: with two forks on one
  line, every fork on the line was saved with the first fork's body.
- A multi-line body is now saved in unparse form too, so its line layout can
  differ from the source (one statement per line), as in Toast, whose reloaded
  forked task numbers lines from the unparsed text plus the saved first line.
- In fully parenthesized form Toast does not bracket an assignment's right
  side (`unparse.cc:739-743`): `y = x + (2 * 3);`.

## Verified

- `go test ./vm ./compiler ./engine ./db/... ./bytecode ./types ./task -count=1`:
  every package `ok`. No existing expectation was changed.
- `gofmt -l` on the changed Go files prints nothing; `go vet ./...` exits 0.
- `go test ./vm -run '^$' -fuzz 'FuzzSuspendedStackCheckpoint$' -fuzztime 40s`:
  2818 executions, PASS.

## Not verified

- No Toast run: Toast's behavior here is from reading its source only. The
  saved text was not loaded into a Toast server.
- The native path (`compileNativeVector`) fills the table through the same
  helper, but no test here forks from a frame imported from a Toast
  checkpoint and then checkpoints the queued child.
- Compile time and memory of the table were not measured.

## Next step

The lead pushes the branch and opens the pull request.
