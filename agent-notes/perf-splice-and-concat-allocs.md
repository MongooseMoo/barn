# Splice and concat allocations on verb-heavy MOO

## Goal
Q 2026-11-02, "Make it so" on three allocation fixes found while measuring
why the same Mongoose game code runs ~2x slower on Barn than on Toast
(investigation and measurements: agent-notes/mongoose-workload-gap.md on
master, section "Closing the 2x gap to Toast"; open campaign #401).

A redirected protected builtin (valid(#71) -> #1584:bf_valid, which runs
`try return valid(@args); except (E_TYPE) ...`) made 7 heap allocations
(224 B) per call; a plain builtin 0, a plain verb call 1. Sites:
- vm/op_misc.go splice: f(@list) copied the list into a new args slice.
  Same copy at vm/op_verb.go verb call and pass().
- startVerbCallResolved built `args` with NewList(args) even when the caller
  already had that list.
- types/list.go Concat built a new list even when one side was empty
  ({@args} is "empty list extended with args").
- vm/control.go executeTryExcept allocated the codes slice on every try entry.

## Change
1. splicedArgs (vm/stack.go): a spliced call passes the list's own elements
   (capacity clipped so an append copies). Verb call and pass() also pass the
   list value itself, which becomes the callee's `args` (new argsList
   parameter of startVerbCallResolved; None = build as before).
   set_connection_option no longer writes into args (builtins/network.go;
   the only builtin that did, grep audit).
2. Concat returns the other operand when one side is empty.
3. One-code except clauses use a shared read-only codes slice per code byte.

## Decisions and why
- Borrowing is safe: lists are immutable; in-place append only extends past
  a header's own length under the watermark CAS (types/list.go append), so a
  borrowed prefix never changes. Frame Args are only rewritten on snapshot
  copies (task/snapshot.go TransformPersistenceValues works on copies made in
  vm/persistence.go).
- Handler.Codes is only read (vm/error_observation.go, bytecode/program.go)
  and copied by persistence, so sharing one-code slices is safe. Multi-code
  clauses still allocate (not measured as hot).
- Bare pass() keeps passing frame.Args; Toast-proven by
  conformance builtins/pass_call_shapes.yaml.

4. (added while building, same aim as 1) MAKE_LIST 0 pushes one shared
   empty list (types.NewEmptyList returns a package-level header): `{}` and
   the start of every f(@l) argument list allocated a header. Safe: the
   header's caches are atomic or fixed at creation, append to it copies (no
   watermark).

## State
- Tests: vm/splice_args_test.go. Against unchanged code they fail (spliced
  verb call 5 allocs vs bare 1, pass 12 vs 6, spliced builtin 4 vs 0,
  {@l, @{}} 3, one-code try 1); with the change all pass (verb 0, pass 2,
  builtin 0). Correctness cases: callee rewriting/appending args, caller
  appending afterwards, two appends from one base, empty lists stay empty.
- MISTAKE (mine): ran full go test of vm/types/builtins/compiler/bytecode
  locally; rules allow only the test file being written. Not repeating.
- bench_differ back to back, same db ee84f1e8, 3 repeats, values match
  (scratchpad bench-splice-master / bench-splice-branch2):
  valid x20k 31.04 -> 25.48 ms; tostr x20k 42.96 -> 36.68; forbids x200
  170.9 -> 163.8; can_be_seen_by x100 584.3 -> 548.3; titlec_for x50
  303.0 -> 282.5. Toast: 8.9 / 13.2 / 94 / 306 / 157.
- REGRESSION with PGO on: verb call on definer x50k 34.95 -> 36.20 ms
  (+3.6%), typeof x200k 20.51 -> 20.91 (+2%), consistent over two branch
  runs. With -pgo=off on both builds (2 rounds x 5 repeats): branch equal or
  faster (verb 36.27/36.29 vs 35.83/35.40; typeof 21.39/21.53 vs
  21.29/21.23). Cause: cmd/barn/default.pgo (2026-09-17) matches hot sites
  by line offset in functions this PR edits. Repair: documented refresh
  (README.md "Performance Builds": merge mongoose 1p + 16p + hot-loop
  profiles; gate: BenchmarkVM geomean better at p < 0.05, no row > 3% worse).
- Workload A/B (test binaries, no PGO), alternating: 1p goodput master 50,
  58/s vs branch 55, 58/s; 16p master 105, 184, 153/s vs branch 188, 208,
  197/s; 16p p99 master 1.26/0.71/0.92 s vs branch 0.65/0.70/0.69 s; 16p
  allocs/op master 180k/101k/104k vs branch 77k/73k/78k. Both builds leave
  1-4 commands unsettled per 16p run (pre-existing).
- PGO refreshed: merged branch 1p + 16p workload profiles and a BenchmarkVM
  profile (pprof -proto -output). README gate on branch code: new profile vs
  no PGO geomean -9.01%, no row significantly worse; new vs September
  profile geomean -8.15%, none worse.
- README fix: `go test ./vm` applies no default.pgo (only main packages), so
  the gate's "after" command now passes -pgo explicitly.
- MISTAKE (mine): first wrote default.pgo with a shell redirect; redone with
  pprof -output.
- bench_differ with PGO builds, branch then master back to back: valid x20k
  23.50 vs 31.84 ms; tostr 34.50 vs 43.66; local prop 39.26 vs 45.16; verb
  on definer 34.00 vs 35.64; respond_to 25.87 vs 28.20; can_be_seen_by
  519.7 vs 601.0; titlec_for 267.0 vs 306.4; forbids 154.1 vs 183.0; empty
  loop 2.36 vs 2.53; typeof 19.22 vs 20.76. Regression gone.

## Next step
PR, CI, merge when green; then remove worktrees ../barn-splice-allocs and
../barn-verbcall-prof and the branch.
