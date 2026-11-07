# test/fuzz-source-oracles

## Goal

Plan item C4 of `agent-notes/fuzz-pipeline-expansion.md`: stronger source fuzz oracles.

## State

Built and verified locally; PR open against master (see Next step).
FuzzSourceExecution now resumes suspends, runs fork children through the
shared `vm.NewForkVM`, requires every task to end, and checks the toliteral
round trip. No fuzz.yml change: no new target.

## Decisions and why

- Moved `forkBodyProgram` and `Runtime.newForkVM` from `engine/task_factory.go`
  into `vm/control.go` as `vm.ForkBodyProgram` and `vm.NewForkVM` (body
  unchanged; `configureVMStackLimit` inlined as its one line). Engine calls
  them at its two sites. Why: the brief requires the fork continuation to use
  the path real tasks use, not a test copy, and vm cannot import engine.
- Fork children and suspend resumes share one 100-tick budget with the root,
  as the brief says. Children start with the ticks the earlier tasks left; a
  child with none left is not started. suspend(n) is resumed at once: the delay
  is scheduler timing, not VM behavior.
- Round trip compares with the production `equal()` builtin (case-sensitive
  MOO equality), called through `Session.CallByNameWithExecution`, and builds
  the literal with the production `toliteral()` the same way.

## Verification so far

- Seed corpus passes with the new oracles (`go test ./vm -run
  '^FuzzSourceExecution$' -count=1`: ok).
- Oracle fires: with `publicLiteral` temporarily appending `, 0` to lists, seeds
  #9, #10, #11, #77, #78 failed with "toliteral round trip changed ..."
  (reverted).
- Continuations run: a temporary log showed seed #87 resume one suspend, and
  seed #88 resume a suspend, queue a fork, and run child task 2 through its own
  suspend (log removed).
- gofmt, go vet, staticcheck, golangci-lint on vm, engine, internal/fuzzseed:
  clean.
- Bounded campaign `timeout --kill-after=5s 300s go test ./vm -run '^$' -fuzz
  '^FuzzSourceExecution$' -fuzztime=4m -parallel=2`: PASS, 15635 execs, 37 new
  interesting inputs, no failure. No wrong-value or panic finding, so nothing
  saved under vm/testdata/fuzz and no production fix.

- Hang found by fuzz-grammar (C6) and confirmed here: `n = 1; while o (n)
  endwhile` charges only unchecked ticks (`compiler/lower_control.go:126-130`,
  Toast EOP_WHILE_ID decrements without testing, `execute.cc:2286-2288`), so
  only the seconds check at a tick checkpoint stops it (`vm/vm.go:481-491`).
  `task.NewState(..., 1)` without a deadline keeps `SecondsLeft` at 1
  (`task/task.go:814-821`). The saved input
  `vm/testdata/fuzz/FuzzSourceExecution/named-while-unchecked-ticks` timed out
  at 60 s before the fix (exit 124) and passes in 0.11 s after
  `attachSourceFuzzTask` sets a 100 ms execution deadline. Harness defect, not
  a Barn production bug; it predates this branch (the old target had the same
  NewState call). fuzz-grammar carries the same fix and input on
  test/fuzz-structured-source; whoever merges second rebases vm/fuzz_test.go.

## Surprises

- read() cannot suspend in this host (no connection manager), so only
  suspend() and fork reach the continuation paths here.
- The campaign showed stretches of 0 execs/sec of up to about 20 s between new
  interesting inputs. Not examined; the cause is unknown (Go's minimization of
  new inputs is one candidate, not checked).
- Only 2 of 97 seeds yield (the new suspend/fork seed and the existing fork
  seed); the Test.db verb seeds error before reaching suspend() in an empty
  store.

## Findings

- Current target `vm/fuzz_test.go` FuzzSourceExecution: compiles with
  `registry.Compiler().CompileMOO`, runs `machine.Run(program)` once with 100
  ticks, ignores the result. No suspend resume, no fork.
- Real resume path for suspended tasks is engine-owned: `engine/task_runtime.go`
  launchSlice reuses the saved VM and calls `vm.Resume()` (`vm/vm.go:670`);
  `vm.SetResumeValue` (`vm/vm.go:715`) delivers read() values.
- Real fork path: `engine/task_runtime.go:1285` drainForks calls
  `Runtime.CreateForkedTask` (`engine/task_factory.go:291`), which builds the
  child with `Runtime.newForkVM` (`engine/task_factory.go:261`, a Runtime method
  using `bytecode.ForkBody.ExtractProgram`, `vm.PrepareVerbFrame`,
  `vm.SetLocalByName`), then `vm.SetForkResult` + `vm.Resume()`.
- vm cannot import engine (engine imports vm), so a vm-package target can only
  reuse the VM-level resume API, not engine's child construction.
- In the fuzz host (`builtins.Host{TaskManager: emptyTaskHost{}}`), suspend()
  already yields FlowSuspend (`builtins/tasks.go:134-172` needs only a task and
  a task manager, not CanSuspend). read() cannot suspend there: no ConnManager,
  so it returns E_INVARG (`builtins/connection.go:36`). The VM stays
  `Resumable=false`, so threaded builtins run inline (`builtins/threaded.go:31`)
  and no transaction renewal happens (`vm/op_property.go:189`).
- Toast toliteral (`list.cc:1189` bf_toliteral -> `list.cc:449` unparse_value):
  INT `%PRIdN`, OBJ `#%d`, ERR error_name, STR quoted with only `"` and `\`
  escaped, LIST `{a, b}`, MAP `[k -> v]`, BOOL true/false, re-readable.
  FLOAT is `%.*g` with DBL_DIG (15 digits) (`list.cc:460-465`), so it is lossy
  (0.1+0.2 prints 0.3) and cannot round-trip by equality. ANON prints
  `*anonymous*` and WAIF `[[class = #c, owner = #o]]`, neither is a value
  expression. Barn's publicLiteral (`builtins/types.go:214`) and formatFloat
  (`types/value.go:200`, 'g' 15) follow the same spellings.
- Toast charges one tick for OP_FORK, OP_FORK_WITH_ID and OP_BI_FUNC_CALL
  (`src/include/opcode.h:44-50`, `COUNT_TICK` at `opcode.h:129`, applied at
  `execute.cc:977`). So every yield (a fork, or suspend() as a builtin call)
  costs at least one tick; a yield with no tick charged is a defect (the
  zero-tick loop class of #610).

## Next step

Watch the PR's CI to green; the lead reviews and merges.
