# Fix argument-list size check at the default max_list_value_bytes (issue #1108)

## Goal

ToastStunt refuses a builtin, verb or pass() call whose arguments, built as a
list, exceed `max_list_value_bytes`. Barn made that check only while the limit
was lowered below its default (64537861). Barn must give Toast's result at the
default limit too, with no slowdown.

## State

Barn fix on `fix/arglist-default-limit`; suite test on
`test/arglist-default-limit` (two commits). Nothing pushed.

- Suite: `server/limits_argument_list.yaml`, 9 rows. Stock WSL Toast
  (managed `make conformance-toast`): 10 passed with K=limits_argument_list
  (9 rows + capability_admission), and 62 passed, 6 skipped with
  `K="limits or server_options"`. Cost on Toast: the rows take under 0.5 s
  in all (pytest durations; the 4-6 min per run is collecting 14225 items
  over /mnt/c). The largest process in the run tree peaked at 100 MB RSS
  (`/usr/bin/time -v`, harness included). The setup builds a 32268897-byte
  and a 32268898-byte string once and keeps them in `#0` properties. Barn's
  run tree peaked at 483 MB.
- Unfixed Barn (origin/master from `git archive` in a temp dir, same managed
  command, K=limits_argument_list): all 9 rows fail, capability_admission
  passes. The Barn PR merges first.
- First full managed run on fixed Barn: 5 failures, all in this file. The
  same 5 failed on Toast with `K="limits or server_options"`. Suite
  teardowns never run (`runner.run_suite_teardown` has no caller in
  `plugin.py`), so `server/limits.yaml`'s limit properties (1000000) were
  still defined when this file ran. Fixed in the suite's second commit: the
  setup deletes them first. After that, full managed Barn: 13899 passed,
  327 skipped, 0 failed. That run was before the last performance changes;
  the final full run is under Gates.
- Gates on the final code: gofmt, `go vet ./...`, staticcheck, golangci-lint
  (0 issues), `go build ./...` and `go test ./...` pass. `go test -race ./vm
  ./types ./compiler` passes (an earlier version also passed it on
  ./builtins and ./bytecode). `python3 -m unittest discover -s scripts`
  fails in WSL for environment reasons (99 `KeyError: 'RUNNER_TEMP'`, one
  missing `lambdamoo_db`); no script was touched.
- Final full managed conformance (final code, suite branch, WSL): 13899
  passed, 327 skipped, 0 failed.

## Performance

BenchmarkVM setup: test binaries built with `-ldflags=-funcalign=64`, with
`vm.(*VM).executeLoop` at 0 mod 64 in both. Base is origin/master from `git
archive` with this branch's `perf_bench_test.go` copied in. Rounds are
interleaved with the order alternating, `-test.cpu 1`, 10 rounds. The
workstation was busy throughout (WSL load 9-18, Windows CPU 37-68%), so
medians move by a few percent from run to run.

Final code, ms, median (min), base -> fixed:

| workload | base | fixed | median |
|---|---|---|---|
| int_arith_1M | 46.90 (45.95) | 46.69 (46.05) | -0.4% |
| string_concat_10k | 1.84 (1.77) | 1.77 (1.70) | -4.2% |
| list_append_10k | 2.47 (2.22) | 2.33 (2.27) | -5.9% |
| builtin_abs_200k | 30.85 (29.55) | 29.45 (27.58) | -4.5% |
| builtin_two_args_200k | 33.29 (32.37) | 33.83 (31.14) | +1.6% |
| builtin_three_args_200k | 36.33 (35.62) | 37.57 (34.72) | +3.4% |
| builtin_three_args_expr_200k | 39.95 (38.92) | 43.19 (41.01) | +8.1% |
| prop_access_1M | 220.31 (215.58) | 216.30 (205.72) | -1.8% |
| typeof_1M | 126.14 (124.25) | 121.55 (117.28) | -3.6% |

- builtin_three_args_expr (`maphaskey(m, i, -c)`) is the only workload that
  runs `OP_CHECK_ARGS`. Its +5-8% is real: in pprof the check is 3.8% of
  samples and the call's own check 3%. That is about 15 ns per call with a
  later argument that can fail, out of about 200 ns for the whole call.
  This cost is NOT removed; see Next step.
- builtin_two_args and builtin_three_args run only the call's own check.
  Their medians are +1.6% and +3.4%, and their mins are faster than base.
  I read that as noise, but the per-call check (one `ListBytes`, about
  4-6 ns on this machine) is not free.
- What was tried: the first version (check op on the generic dispatch path)
  cost builtin_three_args +7.5% (+12% on min). Then I added a dispatch fast
  path for `OP_CHECK_ARGS` (`fkCheckArgs`), a tag-mask skip in `ListBytes`
  for values with no payload, and no check before a read of a variable that
  a finished top-level `name = expr;` bound.
- Real workload (`TestMongooseRealWorkload`, mongoose.db.new, same
  alignment flag, 4 interleaved rounds; goodput in cmd/s):

| round | load (WSL 1m base/fixed, Win CPU) | base 1p | fixed 1p | base 16p | fixed 16p |
|---|---|---|---|---|---|
| 1 | 10.9/10.2, 64/59% | 61 | 50 | 297 | 250 |
| 2 | 12.2/13.2, 53/54% | 49 | 55 | 228 | 222 |
| 3 | 9.7/9.5, 70/60% | 37 | 63 | 321 | 232 |
| 4 | 15.2/9.4, 63/73% | 56 | 49 | 77 | 259 |

  Medians: 1p 52.5 vs 52.5; 16p 262.5 vs 241. One build's runs range from
  77 to 321, far wider than any difference between the builds, so this run
  shows neither a slowdown nor its absence. Both builds report FAIL with the
  dump's own failed/uncaught commands, at similar counts. `files/` was
  deleted from both trees.

## Toast source (origin/master)

- `MEMO_SIZE` is defined in the stock build: `src/include/options.h:343`.
- What is counted (`utils.cc:804` `value_bytes_depth`): every value is
  `sizeof(Var)` = 16 bytes. A string adds `memo_strlen + 1`, a float 8, and
  a list `list_sizeof` (`list.cc:525`: 16 for the length slot plus each
  element's value_bytes, memoised in the list's metadata). So the argument
  list `{a1..an}` takes `32 + sum(value_bytes(ai))`, and a string argument
  takes `17 + bytes`. Barn's `types.ListBytes` computes the same.
- Where it is checked (`code_gen.cc:545` `generate_arg_list`): the first
  argument uses `OP_MAKE_SINGLETON_LIST` or `OP_CHECK_LIST_FOR_SPLICE`,
  neither of which checks the size. Every later argument uses
  `OP_LIST_ADD_TAIL` or `OP_LIST_APPEND` (`execute.cc:1129`, `:1151`),
  which keeps the list only while `value_bytes(r) <=
  server_int_option_cached(SVO_MAX_LIST_VALUE_BYTES)` and otherwise does
  `PUSH_ERROR_UNLESS_QUOTA(E_QUOTA)` (`execute.cc:899`).
- Consequences, all confirmed on the managed Toast run:
  - The check runs as each argument joins the list. Arguments after the one
    that passes the limit are never evaluated in a `d` verb: no side
    effect, and no error of their own.
  - In a verb without `d` and max_concat_catchable true, PUSH_ERROR pushes
    E_QUOTA in place of the list and later arguments are still evaluated.
    The next OP_LIST_ADD_TAIL sees a non-list and pushes E_TYPE, and the
    call (`OP_BI_FUNC_CALL` `execute.cc:2259`, `OP_CALL_VERB` `:2153`, or
    pass) gets a non-list and yields E_TYPE.
  - A call whose only argument is one splice (`f(@l)`) is never checked,
    even when `l` is past the limit.

## Decisions and why

- Measure at every limit, from cached sizes. `types.ListBytes` reads each
  argument's cached size: a string's byte length, a list's `byteSize`
  atomic memo, and a map's `byteSize`, which every map constructor sets. So
  the check is a few loads per argument, whatever the values hold, and a
  list under the smallest limit (1021) never looks the limit up. The
  `ListLimitLowered` flag and the check in `Session.CallByIDWithExecution`
  are gone. The builtin, verb and pass() checks are all `VM.checkArgList`
  at the call.
- Check each prefix before the next argument runs, as Toast does. The
  compiler's `compileCallArgs`, shared by builtin, verb and pass() calls,
  emits `OP_CHECK_ARGS k` before evaluating argument k+1 (k >= 2). It skips
  the check when that argument can neither fail nor act: a literal, or a
  read of a variable that a finished top-level `name = expr;` statement
  assigned (`lowerer.bound`). MOO never unbinds a variable, so such a read
  cannot raise E_VARNF, and OP_GET_VAR charges no tick. Prefixes only grow,
  so the next check or the call's own check gives the same result.
- The opcode reuses OP_SCATTER_TAKE's number, which was dead and rejected by
  the decoder; opcode space is otherwise full. It charges no tick (Toast's
  OP_LIST_ADD_TAIL charges none). It has a dispatch fast path
  (`fkCheckArgs`: a prefix under 1021 bytes continues), and its stack
  effect is (k, k) so the verifier holds the stack depth.
- A verb without `d`: Toast replaces the list with E_QUOTA and keeps
  evaluating, and the call yields E_TYPE. `OP_CHECK_ARGS` marks the refusal
  by putting `types.None` in the first argument's slot. `ListBytes` counts
  None as `math.MaxInt32`, above every limit (the maximum is MaxInt32 -
  1021). So the call's check always reaches the slow path and returns
  E_TYPE, whatever the limit or max_concat_catchable has become since; a
  later argument may reload options. None persists in a checkpointed frame
  as CLEAR, which reads back as None. A refusal found by the call's own
  check in a verb without `d` is also E_TYPE.
- Spliced calls: the first argument is now Toast's unchecked singleton
  (`OP_TICKS 0; OP_MAKE_LIST 1`) or a type-checked splice (`OP_TICKS 0;
  OP_SPLICE`), instead of `OP_MAKE_LIST 0` plus a checked append. Later
  arguments keep the checked `OP_LIST_APPEND` / `OP_LIST_EXTEND`. The
  separate check of a spliced list at the call (`checkSplicedArgsLimit`) is
  gone. Ticks are unchanged: the first argument's tick is still charged by
  the call.
- `OP_MAKE_LIST 1` no longer checks the limit. It is Toast's
  OP_MAKE_SINGLETON_LIST, which Toast never checks; this covers the list
  literal `{x}` too, pinned by `one_element_list_past_limit_is_not_checked`.
  The list case in `vm/registry_required_test.go` now builds `{long, 1}`,
  which Toast does check.
- Native import layout: `leadingSplice` also recognises a call whose first
  argument is spliced, so the symbolic stack after `OP_SPLICE` is the
  argument-list prefix as before. The layout's `flush` keeps the arguments'
  origins across `OP_CHECK_ARGS`; its generic path would have replaced the
  k values with k copies of the call's own origin. Pinned by
  `compiler/check_args_layout_test.go`.
- `ListBytes` skips values whose tag carries no payload with one mask test
  (`sizedTags`). `types/list_bytes_test.go` now also covers "", {} and [].
- Benchmarks added: `BenchmarkVM/builtin_three_args_200k` (`maphaskey(m,
  i, c)`, with c bound, so no check op) and `builtin_three_args_expr_200k`
  (`maphaskey(m, i, -c)`, one `OP_CHECK_ARGS` per call).

## Surprises

- Barn evaluated every unspliced argument before checking (`vm/error.go`
  `checkCallArgsLimit`, called from the call ops). So at any lowered limit
  it ran later arguments that Toast never runs, gave E_QUOTA where Toast
  gives E_TYPE in a verb without `d`, and checked a lone spliced list that
  Toast does not (`OP_MAKE_LIST 0` + `OP_LIST_EXTEND` in
  `compiler/lower_expressions.go`, plus `checkSplicedArgsLimit`).
- Suite teardowns never run (see Unfiled findings). The suite file's
  first version passed alone and failed in a full session on both engines.

## Unfiled findings

- moo-conformance-tests `src/moo_conformance/runner.py:447`
  `run_suite_teardown` is never called: `plugin.py` and
  `test_conformance.py` have no caller. Every file's `teardown:` block is
  therefore dead, and a file's setup state leaks into later files.
  Reproducer: run `server/limits.yaml` followed by any file that assumes
  the default limits. This file's first version failed exactly so, on Toast
  and on Barn.
- Not verified: suppose a frame is suspended inside a later argument after
  a refusal in a verb without `d`. Barn's native frame export writes the
  prefix list with CLEAR as its first element, where Toast's own frame
  holds E_QUOTA in place of the list. Barn reads its own CLEAR back as the
  mark, but a Toast server reading that frame would see a list, not
  E_QUOTA.
- Not verified: Barn charges a call's first-argument tick
  (MAKE_SINGLETON_LIST) at the call (`bytecode.DynamicTicks`); Toast
  charges it when the first argument is pushed. A task that runs out of
  ticks between the two points could differ in what ran. This is off this
  path and I built no reproducer.

## Next step

- Decide the `OP_CHECK_ARGS` cost: about 15 ns (+5-8% on
  builtin_three_args_expr_200k) for each call whose third or later argument
  can fail or act. Removing it means not checking before such an argument,
  which runs arguments Toast never runs: the rows
  `later_arguments_are_not_evaluated_after_the_list_passes_the_limit` and
  `pass_later_arguments_are_not_evaluated_after_the_list_passes_the_limit`
  would fail. I did not find a cheaper exact form. A measurement on a quiet
  machine would also settle the +1.6% / +3.4% on the call's own check.
- Merge order: the Barn PR first, then the suite PR (all 9 rows fail on
  unfixed Barn).

## Barn PR body

Title: Check call argument lists against max_list_value_bytes at every limit, as Toast builds them

Fixes #1108.

ToastStunt builds the arguments of a builtin call, verb call or pass() as a
list (`code_gen.cc` `generate_arg_list`) and holds it to
`max_list_value_bytes` as each argument after the first joins (`execute.cc`
`OP_LIST_ADD_TAIL` / `OP_LIST_APPEND`, then
`PUSH_ERROR_UNLESS_QUOTA(E_QUOTA)`). Barn measured the arguments only while
the limit was lowered, and only after evaluating all of them.

- Every call with two or more unspliced arguments is measured, whatever the
  limit (`VM.checkArgList`, from cached sizes via `types.ListBytes`). The
  `ListLimitLowered` flag is gone.
- `OP_CHECK_ARGS k` checks the arguments pushed so far before a later
  argument that could fail or act, so that argument never runs once the
  list is refused, as in Toast. It reuses the dead OP_SCATTER_TAKE number
  and charges no tick. It is not emitted before a literal, or before a read
  of a variable that a finished top-level assignment bound.
- In a verb without the d flag, Toast's refused list becomes E_QUOTA, later
  arguments still run, and the call yields E_TYPE. Barn marks the refusal
  with None in the first argument's slot, and the call yields E_TYPE.
- Spliced calls start the list as Toast does: the first argument is an
  unchecked singleton or a type-checked splice. `OP_MAKE_LIST 1` (Toast's
  `OP_MAKE_SINGLETON_LIST`) no longer checks the limit, so `{x}` and
  `f(@l)` are not refused.
- The native import layout keeps argument origins across the new opcode
  and treats a call's leading splice as the list prefix.

Tests: `vm/arglist_limit_test.go`, `compiler/check_args_layout_test.go`,
`types/list_bytes_test.go`, `vm/registry_required_test.go`. Conformance:
moo-conformance-tests `server/limits_argument_list.yaml` (suite PR). All 9
rows fail on master and pass on this branch.

Performance (BenchmarkVM, `-funcalign=64` on both sides, interleaved, 10
rounds, busy machine): calls without the new check are within noise
(builtin_two_args +1.6% median, faster on min). A call whose third or later
argument can fail pays the check, about 15 ns (builtin_three_args_expr_200k
+8.1% median). Real workload at 1 and 16 players: within its noise.

## Suite PR body

Title: Pin the argument-list size check: default limit, evaluation order, no-d verbs, splices

Adds `server/limits_argument_list.yaml`. ToastStunt builds builtin, verb and
pass() arguments as a list (`code_gen.cc` `generate_arg_list`) and checks
it against `max_list_value_bytes` as each argument after the first joins
(`execute.cc` `OP_LIST_ADD_TAIL` / `OP_LIST_APPEND`).

- At the default limit (64537861), two strings whose argument list takes
  exactly the limit are accepted, and one byte more is refused, for a
  builtin, a verb and pass(): E_QUOTA with `max_concat_catchable` true, an
  out-of-seconds abort with it false.
- An argument after the one that passes the limit is never evaluated in a
  verb with the d flag: no side effect, and no error of its own.
- In a verb without d, the call yields E_TYPE and later arguments still run.
- A lone spliced list and a one-element list are not checked.

The setup deletes the limit properties an earlier file may have left (suite
teardowns do not run between files) and builds the two 32 MB strings once.
Stock WSL Toast: 10 passed with capability_admission; the rows take under
0.5 s. All 9 rows fail on current Barn master, so merge this after the Barn
PR for #1108.
