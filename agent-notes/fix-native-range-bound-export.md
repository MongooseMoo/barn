# Fix: native export of a task suspended in a range loop's upper bound

## Goal

Barn must write, in ToastStunt's form, a task suspended while a range loop's
upper bound is being evaluated (`for n in [1..suspend()]`, or a verb call in the
bound whose callee suspends), and must import Toast's form of that state.

Reported failure (fuzz lane, Barn PR 892): `frame 0 (#0:): no held value for
native operand kind 6 from *ir.RangeLoopStmt` from
`compiler/native_export.go:251` (kind 6 = `NativeRangeNext`).

## State

Fix, producer, fixture and tests are committed locally on
`fix/native-range-bound-export`. Not pushed, no PR (the team lead does that).

### The defect

Toast (stock WSL source, `/root/src/toaststunt/src`):

- `code_gen.cc:1035-1038` (`case STMT_RANGE`): `generate_expr(from)`,
  `generate_expr(to)`, then `OP_FOR_RANGE`. While `to` is being evaluated,
  `from`'s value is an ordinary operand on the run-time stack.
- `execute.cc:1020-1063` (`OP_FOR_RANGE`): reads `to = TOP_RT_VALUE`,
  `from = NEXT_TOP_RT_VALUE` and keeps the next value and the limit in those
  two slots for the life of the loop.

Barn's description of Toast's stack (`compiler/native_layout_stmt.go:96-101`)
relabels the evaluated start `{NativeRangeNext, loop}` as soon as `n.Start` is
done, so a continuation for a call inside `n.End` wants that origin.

Barn's description of its own stack at the same call
(`compiler/native_barn_layout.go`, the `enter` closure) recorded the start value
as `{NativeExpression, n.Start}`; `NativeRangeNext`/`NativeRangeLimit` were
attached only to the hidden locals after the loop head
(`compiler/lower_control.go`, `compileRangeLoop`). Export
(`native_export.go` `nativeOperand`) and import (`native_import.go` `resolve`)
both match the two descriptions by origin, so both failed:

- export: `no held value for native operand kind 6 from *ir.RangeLoopStmt`;
- import: `native continuation lacks 0 value from *ir.IdentifierExpr`.

The lower bound was never affected (nothing of the loop is on either stack
yet), nor the body (both sides use the range names there).

### The fix

- `compiler/lower_control.go` (`compileRangeLoop`): after compiling `n.Start`,
  when an import layout is observing, call `markRangeStart`.
- `compiler/native_barn_layout.go` (`markRangeStart`): relabel the top of the
  observed Barn stack `{NativeRangeNext, loop}`.

No emitted bytecode changes; the import layout only observes. One relabel fixes
export and import.

### Evidence

Toast first. Producer
`tests/mongoose-conformance/server/suspended_range_bound_checkpoint.yaml`: two
forked tasks, `flat` (`low = 2; for n in [low..this:limit(4, "flat")]`) and
`nested` (`for a in [1..2] for b in [10 * a..this:limit(10 * a + 1, "nested")]`);
`limit` suspends 20 s the first time per key. After `dump_database()` and
`restart_server` it asserts `seen == {2, 3, 4}`,
`pairs == {{1, 10}, {1, 11}, {2, 20}, {2, 21}}`, each task finished once.

All managed runs were inside WSL Debian, candidate mode
(`--candidate-root`, `--moo-suite-root=tests/mongoose-conformance`,
`--moo-suite-path=server/suspended_range_bound_checkpoint.yaml`), canonical
admission in the same session, Test.db SHA-256
1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e. Driver
scripts are untracked scratch: `.tmp/range-bound/run_producer.sh`,
`.tmp/range-bound/toast_capture.sh`.

- Stock WSL Toast (`/root/src/toaststunt/build-release/moo`, binary SHA-256
  61dd1799d82f75e33cb9c42fe48bd1966199ad83fda2f6298e3a7f872cc613dd, built from
  the ref the fixture's `.md` names; wrapper copies `{db}` before exec):
  `2 passed, 1 warning in 41.12s`.
- Barn without the fix (`bin/range-bound-barn-before`): `2 passed in 34.87s`,
  and the Barn log has, for both tasks at each checkpoint,
  `Toast could not load this checkpoint: task kept in Barn's frame form ...
  reason="frame 1 (#9:flat): no held value for native operand kind 6 from
  *ir.RangeLoopStmt"` (and `#9:nested`).
- Barn with the fix (`bin/range-bound-barn-after`): `2 passed in 33.68s`, no
  such warning in the log.

Fixture: `engine/testdata/toast-suspended-range-bound.db` (+ `.md`), SHA-256
8454828352c2833042c79789516d9e0f8ca32c26e07cebef40a534991bc4baff. The `flat`
frame is written with 1 rt_stack slot holding 2; `nested` with 3 slots
`2, 2, 10`.

Go tests (`go test ./engine -run 'TestToastSuspendedRangeBoundImportRunsTheLoop$|TestBarnSuspendedTaskShapesCheckpointAsToastFrames$' -count=1`):

- `engine/toast_suspended_range_bound_import_test.go`: imports the fixture,
  re-exports the imported tasks and requires every frame's pc, error pc and
  operands to equal what Toast wrote, then resumes to Toast's outcome. Before
  the fix: `native frame 1: native operand 0: native continuation lacks 0 value
  from *ir.IdentifierExpr` (task 1372396205) and `... *ir.BinaryExpr` (task
  1579663818); Barn refused both tasks, it did not convert them wrongly.
- `engine/toast_task_export_test.go`, three new `toastExportShapes`: a builtin
  in an upper bound, the same nested in an outer range loop, and a verb call in
  an upper bound below a computed lower bound. Before the fix each failed with
  `frame 0 was not written in Toast's form`. Each shape exports, round-trips
  the file, imports, re-exports identically and resumes to the same result.
- After the fix both tests pass (all 18 shapes). `gofmt -l` on the four Go
  files prints nothing; `go vet ./compiler ./engine` exits 0.

### Loop-form survey (step 6)

Run as temporary shapes through `TestBarnSuspendedTaskShapesCheckpointAsToastFrames`
(export, file round trip, import, identical re-export, resume), on unfixed and
on fixed compiler code, then removed. All ten export on both:

| Paused at | Result |
| --- | --- |
| `for n in [suspend(100)..2]` (builtin, lower bound) | exports |
| `for n in [#0:pause()..2]` (verb call, lower bound) | exports |
| `for x in ({5, suspend(100), 7})` (builtin in list container) | exports |
| `for x in (#0:pause())` (verb call as list container) | exports |
| same, `for x, i in (...)` inside an outer range loop | exports |
| `for v, k in (["a" -> 1, "b" -> suspend(100)])` (builtin in map container) | exports |
| `for v, k in (#0:pause())` (verb call returning a map) | exports |
| `while (#0.n < 1 && !suspend(100))` (builtin in while condition) | exports |
| `while (#0:pause())` (verb call as while condition) | exports |
| `while w (#0:pause())` named, inside an outer range loop | exports |

Only the range upper bound had the defect.

## Decisions and why

- Fix on the Barn-side label, not in `nativeOperand`/`resolve`: the Toast-side
  layout already names the slot `NativeRangeNext` at `n.Start`'s end; making
  Barn's observed stack use the same name is one relabel and repairs both
  directions, where a special case in export would have left import broken.
- The nested builtin shape uses `suspend(100 * (2 - n))`, not the bare
  `suspend()` of the fuzz finding: the inner bound is evaluated once per outer
  pass, so the literal program suspends twice and cannot finish after one
  resume. The first pass pauses exactly as the finding does.
- The survey probes were not kept as tests: they pass on unfixed code and were
  not asked for.

## Surprises

- The producer passes on unfixed Barn. Barn keeps the unexportable task in its
  own frame form and reads its own file back, so a Barn-only restart cannot go
  red on this defect; only the log warning differs. The red-before evidence is
  the two Go tests.
- CI does not run this producer. `.github/workflows/ci.yml` "Run local
  conformance regressions" passes four explicit `--moo-suite-path` files
  (`builtins/move_false_accept_owner.yaml`, `root/empty_notifications.yaml`,
  `builtins/clear_property_metadata.yaml`,
  `builtins/recycle_unrelated_tasks.yaml`); nothing under `server/` is
  selected, including the existing `suspended_eval_checkpoint.yaml`.

## Next step

Team lead: push, open the PR, and do the cross-engine check that a real Toast
loads and resumes a checkpoint Barn writes for this state. That check is NOT
done. Also unverified: `staticcheck` and `golangci-lint` (CI runs them), the
full Go suite and full conformance suite (CI), and the MAXINT/MAXOBJ edge where
Toast decrements the limit instead of advancing the next value (not touched by
this change, not tested here).
