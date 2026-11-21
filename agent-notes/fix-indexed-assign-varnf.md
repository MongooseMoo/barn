# Fix indexed assignment to an unassigned variable (barn #1106)

## Goal

Barn issue #1106: in a MOO program where `y` was never assigned, `y[1] = 1`,
`y[1..2] = 1` and `y[~""] = 1` raise E_VARNF on ToastStunt and E_TYPE on Barn.
Barn must do what Toast does, including the evaluation order of base, index and
right-hand side. Done = Toast-green conformance test committed on the suite
branch, Barn fix plus Go tests committed on the Barn branch, gates run, this
note complete. Not included: push, PRs, any GitHub post.

## State

Fix and tests committed locally on both branches (not pushed). Suite test is
Toast-green (24/24) and Barn-red on origin/master (15 of 24 fail). With the fix
all Barn gates pass; full managed conformance result is recorded under
"Gates".

## Fix

`compiler/lower_assignments.go` `prepareCollectionBase`: for a variable base,
emit `OP_GET_VAR slot; OP_POP` before any index, bound or value is compiled.
`OP_GET_VAR` raises E_VARNF with "Variable not found: <name>" for an unbound
slot (`vm/vm.go` `OP_GET_VAR` case), matching Toast's base `OP_PUSH`. Both
instructions are tick-free in `bytecode/opcodes.go` `tickTable`, as Toast's
`OP_PUSH` is, so tick accounting is unchanged. The assignment still updates
the slot in place (`OP_INDEX_SET`/`OP_RANGE_SET` operand), so the read only
checks. This one site covers flat index, flat range, nested index and nested
range assignments (all four lowerers call `prepareCollectionBase`). The
native-import layout tracks stack effects per instruction, and the pair is
stack-neutral, so suspended-frame conversion is unaffected (`go test ./...`
includes the native layout and checkpoint tests).

Go tests: `vm/unassigned_collection_assignment_test.go` (16 cases, mirrors the
suite rows). `vm/perf_bench_test.go` gains four BenchmarkVM workloads
(`list_index_set_1M`, `map_index_set_1M`, `list_range_set_1M`,
`nested_index_set_1M`); BenchmarkVM had no index or range assignment workload.

## Fuzz generators

No change. `internal/fuzzseed/structured.go` already produces indexed and
range assignment whose base can be unassigned at run time: `assignment`
(cases 3 and 4) takes the base from `collectionTarget`, which picks from
`g.scope`, and `g.scope` gains a name when the generator emits an assignment,
loop variable or fork variable for it (`bind`), whether or not that code runs
(untaken `if` arm, loop over an empty collection, assignment inside a catch
expression whose value raised). Issue #1106 itself was found by the
generated-program differential sample, which uses these programs.

## Toast source (origin/master, read with git show)

- `src/code_gen.cc:848-925` `EXPR_ASGN`, non-scatter branch: `push_lvalue(e, 0)`
  (line 893), then `generate_expr(rhs)` (894), then `OP_PUT_TEMP` and the
  `EOP_RANGESET`/`OP_INDEXSET` chain. So the order is base, then indexes or
  range bounds, then right-hand side.
- `src/code_gen.cc:565-603` `push_lvalue`: `EXPR_INDEX` and `EXPR_RANGE`
  recurse into the base with `indexed_above = 1` before generating the index
  or bounds; `EXPR_ID` with `indexed_above` emits `OP_PUSH` of the variable;
  `EXPR_PROP` generates the object expression and then the name.
- `src/execute.cc:1849-1861` (`OP_G_PUSH`), `2955-2967` (`OP_PUSH + n`) and the
  `OP_PUSH_CLEAR` forms (1866-1876, 3000-3012): a `TYPE_NONE` slot raises
  `PUSH_X_NOT_FOUND(E_VARNF, name, nothing)`, message `"Variable not found: <name>"`
  (macro at `execute.cc:918`).
- Consequence: with `y` unassigned, every form `y[...] = v`, `y[a..b] = v`,
  `y[i][j] = v`, `y[i][a..b] = v` raises E_VARNF before any index, bound, `$`,
  `^` or right-hand side is evaluated. `y.x[...] = v`, `y[i].x = v` and
  `y.x = v` raise E_VARNF from `generate_expr` of the object (`y` read first).
- Scatter: `src/parser.y:1097` (`scatter_from_arglist`) rejects any target
  that is not a simple variable ("Scattering assignment targets must be simple
  variables."), and `'{' scatter '}'` takes only `tID`. A scatter target
  cannot be indexed, so scatter is not on this path.

## Barn source (before the fix)

- `compiler/lower_assignments.go` `prepareCollectionBase`: for a
  `VariableTarget` it returns the slot without reading it, so
  `compileIndexAssign`, `compileRangeAssign` and `compileNestedRangeAssign`
  evaluate the indexes/bounds and the value first; the unset slot is first
  touched by `OP_INDEX_SET`/`OP_RANGE_SET` (`vm/op_index.go`), which call it a
  type error (E_TYPE), or, for nested targets, by the `OP_GET_VAR` emitted after
  the first index (E_VARNF, but after the first index ran).
- Property bases (`y.x[1] = v`, `y[1].x = v`, `y.x = v`) compile the object
  expression first, so Barn already reads `y` first there.

## Conformance test

Suite file `src/moo_conformance/_tests/language/unassigned_collection_assignment.yaml`
(suite branch `test/indexed-assign-varnf`), 24 rows: the three reported forms,
side-effect logs proving no index/bound/value runs, index/bound/value errors
not reached, the error message naming the base (`y[x] = 1`, `y[x..z] = {1}`),
`$` and `^..$`, nested index and nested range targets, property bases, and two
assigned-base controls (index then value; start, end, then value).

Toast run (managed stock WSL oracle, `make conformance-toast
CONFORMANCE_SUITE=<suite worktree> K=unassigned_collection_assignment`, WSL load
average 9.5 on 8 CPUs): 25 passed (24 rows + capability_admission), 0 failed.

Unfixed Barn run (managed harness, `make conformance-k
CONFORMANCE_SUITE=<suite worktree> K=unassigned_collection_assignment`, Barn at
origin/master): 15 failed, 10 passed. Failing rows (these decide merge order:
Barn fix first, then the suite PR):

- unassigned_index_assignment_raises_varnf (E_TYPE)
- unassigned_range_assignment_raises_varnf (E_TYPE)
- unassigned_index_assignment_with_bad_index_raises_varnf
- unassigned_index_assignment_runs_neither_index_nor_value
- unassigned_range_assignment_runs_neither_bound_nor_value
- unassigned_index_assignment_index_error_not_reached (E_DIV)
- unassigned_index_assignment_value_error_not_reached (E_DIV)
- unassigned_range_assignment_bound_error_not_reached (E_DIV)
- unassigned_index_assignment_names_the_base_not_the_index
- unassigned_range_assignment_names_the_base_not_the_bound
- unassigned_range_assignment_dollar_after_failing_start
- unassigned_nested_index_assignment_outer_index_not_run
- unassigned_nested_index_assignment_outer_index_error_not_reached (E_DIV)
- unassigned_nested_range_assignment
- unassigned_nested_range_assignment_index_error_not_reached (E_DIV)

Passing on unfixed Barn: `y[$] = 1`, `y[^..$] = {1}`, `y[1][2] = 1` (the `$`
marker or the nested intermediate read reads `y` with `OP_GET_VAR`), all four
property-base rows, and both assigned-base controls.

## Decisions and why

- Dropped `features: [index_extensions]` from the new file: no profile or
  harness code provides that feature (grep of the suite finds it only in
  `language/index_and_range.yaml`), so the first Toast run skipped all 24 rows.
  With it removed, Toast runs and passes the `^` rows.

- Fixed in the compiler, where the order is decided, not in `OP_INDEX_SET`:
  a check there would run after the index and value, so `y[1/0] = 1` would
  still raise E_DIV and side effects would still run.
- Reused `OP_GET_VAR`/`OP_POP` instead of a new opcode: the opcode space is
  full (`OP_UNWIND_JUMP` is 255) and the pair needs no verifier, disassembler,
  stack-effect or native-layout change. Measured cost on the deployed binary is
  within noise (below).

## Performance

Machine: Ryzen 9 5950X, shared with the CI runners. WSL load average 7-23 on 8
WSL CPUs during the runs; Windows CPU 7-60% (recorded per round).

1. BenchmarkVM, test binaries, before = `git archive origin/master`, after =
   same tree plus the compiler change, 6 rounds ABBA-interleaved, `-count=1`
   each: the four new set workloads show no significant change
   (list_index_set ~, map_index_set ~, list_range_set ~, nested_index_set ~,
   all p>0.4), but workloads with no index assignment at all (identical
   bytecode) were 6-12% slower (int_arith +10.5%, while_lt +9.4%, nested_1k
   +8.5%).
2. That gap is code layout, not executed code. In the test binaries
   `vm.(*VM).executeLoop` sits at ...080 (before, 64-byte aligned) and ...120
   (after, 32 mod 64). A control build with the same three instructions behind
   a never-true condition (`variable.Name == "~control"`) puts executeLoop at
   ...180 (64-aligned) and runs at before speed (int_arith ~, while_lt ~,
   nested_1k ~, geomean +0.5%), while the after build is +6.5% geomean in the
   same three-way interleaved run.
3. Deployed binary (`cmd/barn`, linux/amd64 GOAMD64=v3, default.pgo, built in
   WSL from both trees): executeLoop is 64-byte aligned in both (...500 and
   ...5c0). `scripts/bench_differ.py --barn-linux`, 3 alternating runs per
   binary, 5 repeats each, median of medians: int_arith -0.1%, while_lt -0.6%,
   list_index -1.1%, list_index_set -1.2%, map_index_set -0.6%,
   list_range_set -1.3%, nested_index_set +4.8% (its minimum is -1.7%; the set
   workloads spread 10-20% run to run because each assignment allocates). No
   measurable slowdown in the shipped binary.

## Gates (Barn worktree, with the fix)

- `gofmt -l .`: empty. `go vet ./...`: clean. `staticcheck ./...`: clean.
  `golangci-lint run ./...`: 0 issues. `go build ./...`: ok.
- `go test ./...`: all packages ok.
- `python3 -m unittest discover -s scripts -p 'test_*.py'` (WSL): 733 tests,
  1 failure and 96 errors, all from the local environment (`RUNNER_TEMP`
  unset, `lambdamoo_db` module missing, CI-only env in test_fuzz_compute /
  test_live_fuzz_page / test_jepsen_checkpoint). This change touches no
  script; CI runs these with its own environment.
- Full managed conformance on Barn with the suite branch (`make conformance
  CONFORMANCE_SUITE=<suite worktree>`, WSL, load average 8-14): 13878 passed,
  327 skipped, 0 failed, `--fail-on-unexpected-skip` on, 22m35s.

## Surprises

- `features: [index_extensions]` (see Decisions) silently skips every row of
  `language/index_and_range.yaml` on every engine.
- `bin/moo -e` on a 100000-iteration loop prints `Error: E_MAXREC`: Barn
  carries tick exhaustion as E_MAXREC with a tick reason (`vm/vm.go:28`,
  `engine/task_runtime.go:968`), so loop timing has to go through BenchmarkVM
  or bench_differ, which raise the limit.

## Unfiled findings

- Suite: `src/moo_conformance/_tests/language/index_and_range.yaml:4`
  requires feature `index_extensions`, which no profile provides, so the whole
  file is skipped on Toast and Barn (seen as "Requires feature:
  index_extensions" skips with `-rs`).
- Barn perf: `l[i] = v` on a local list copies the whole list every time.
  `list_index_set` with a 1000-element list allocates about 24 KB per
  assignment (BenchmarkVM, 24.6 GB for 1M assignments on origin/master); Barn
  is 3-5x Toast on the set workloads in bench_differ. `vm/op_index.go`
  `executeIndexSet` -> `setAtIndex`. Reproducer: `l = {}; for i in [1..1000];
  l = {@l, 0}; endfor; for i in [1..1000000]; l[1 + i % 1000] = i; endfor`.

## Next step

Publish (team lead): Barn PR first (the suite's `main` is what Barn CI runs, and
15 new rows fail on unfixed Barn), then the suite PR.

## Barn PR body

Title: Raise E_VARNF before evaluating the index of an assignment to an unassigned variable

Fixes #1106.

Toast pushes the base variable of an index or range assignment before it
generates any index, range bound or right-hand side (`src/code_gen.cc`
`push_lvalue` and `EXPR_ASGN`), and pushing an unassigned variable raises
E_VARNF, "Variable not found: <name>" (`src/execute.cc`, `OP_PUSH`). Barn
evaluated the index and the value first and then raised E_TYPE from
`OP_INDEX_SET`/`OP_RANGE_SET`, so `y[1] = 1`, `y[1..2] = 1` and `y[~""] = 1`
returned E_TYPE, `y[1/0] = 1` returned E_DIV, and side effects in the index
and value ran.

`prepareCollectionBase` now reads a variable base (`OP_GET_VAR`, `OP_POP`,
both tick-free like Toast's `OP_PUSH`) before the indexes, bounds and value.
That one site serves flat, range, nested index and nested range assignments.
Property bases already read the object first.

- `vm/unassigned_collection_assignment_test.go`: error, order and message
  cases.
- `vm/perf_bench_test.go`: four index/range assignment workloads for
  BenchmarkVM, which had none.

Performance: on the deployed linux/amd64 PGO binary, bench_differ shows no
measurable change (int_arith -0.1%, while_lt -0.6%, set workloads within their
run-to-run spread). The Windows vm test binary shows a 6-12% shift on
workloads with identical bytecode, which a same-size control build attributes
to the 64-byte alignment of `executeLoop`.

Needs the suite test `language/unassigned_collection_assignment.yaml`
(moo-conformance-tests PR); merge this first.

## Suite PR body

Title: Pin E_VARNF and evaluation order for assignment to an unassigned collection variable

Adds `language/unassigned_collection_assignment.yaml` (24 rows). Toast pushes
an index or range assignment's base variable before any index, bound or
right-hand side (`code_gen.cc` `push_lvalue`, `EXPR_ASGN`), so an unassigned
base raises E_VARNF ("Variable not found: y") and nothing after it runs. Rows
cover the flat, range, `$`, `^..$`, nested index, nested range and property-base
forms, side-effect logs, index/bound/value errors that are never reached, and
two assigned-base controls for index-then-value order.

Passes on the managed stock WSL Toast oracle (24/24). Fails 15 rows on Barn
master; Barn PR for MongooseMoo/barn#1106 fixes them and should merge first.
