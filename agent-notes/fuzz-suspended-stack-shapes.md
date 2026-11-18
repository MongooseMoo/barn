# Fuzz generator: suspended stack shapes

## Goal

A choice-driven generator in `internal/fuzzseed` that renders a program whose
task reaches `suspend()` at the bottom of a generated stack of nested contexts
(verb call, eval, fork body, try/except, try/finally, for-list, for-range,
for-map, while, scatter) and returns a value back out through every layer, each
layer transforming it observably. A fuzz target in `vm` feeds it through
`checkpointResumeDifference`. Renderable by `cmd/moo-fuzz-source`. Added to the
`fuzz.yml` matrix. Programs must be valid on ToastStunt.

## State

- `internal/fuzzseed/suspended_stack.go`: `SuspendedStack(choices)` and
  `SuspendedStackKinds` (verb, eval, fork, except, finally, for-list,
  for-range, for-map, while, scatter, expression). Written.
- `vm/suspended_stack_test.go`: `FuzzSuspendedStackCheckpoint`,
  `TestSuspendedStackKindsReachCheckpoint`, `TestSuspendedStackLayersMarkResult`.
  The one-layer seeds of all eleven kinds pass through
  `checkpointResumeDifference` (wizard lane, 30000 ticks) with a checkpoint
  each and no difference.
- `cmd/moo-fuzz-source --generator suspended-stack` renders the choice bytes,
  for stdin and for `--corpus`; `structured` stays the default. An unknown
  name is a usage error (exit 2).
- `.github/workflows/fuzz.yml`: lane `FuzzSuspendedStackCheckpoint`, 30
  minutes. `scripts/test_fuzz_compute.py` lane count 18 -> 19.
- `docs/parser-fuzzing.md`: one paragraph for the target.
- Local results: both tests pass; `gofmt -l` and `go vet` clean on
  `./internal/fuzzseed ./cmd/moo-fuzz-source ./vm`; a 45 s fuzz smoke with two
  workers ran 816 executions, 3 new interesting inputs, no failure.
- No Barn bug found.
- Not run locally: the rest of the suite, `scripts/test_fuzz_compute.py`,
  `scripts/fuzz_differential.py`, anything on Toast.

## Decisions and why

- Choice layout: stack count (1-3), then per stack: depth (1-6), two bytes per
  layer from the outside in (kind, variant), one byte for the bottom form.
  Fixed two bytes per layer keeps seeds readable and a mutation local.
- Each layer has a program-unique number k and marks the value with it, using
  integers only, so eval() source nested in eval() source needs no quotes
  beyond eval's own string.
- Loops run the inner layers on the second of three iterations only. Running
  them on every iteration would multiply suspensions by 3 per loop layer.
- A fork layer's child carries the inner layers and stores their value in its
  own property `f<k>` (not one shared log, whose order would depend on task
  scheduling); the parent suspends right after the fork, so a checkpoint holds
  the outer stack with the child queued.
- scatter and expression need the inner layers as an expression, so they call
  them through a verb or eval() (the variant byte). The scatter's call is the
  default of an optional target, which runs in the middle of the scatter.
- The test object is reached as `$suspended_stack` (a property the program
  adds to #0), because eval() source and verbs share no variables with the
  program. `create(#-1)` and `o.owner` avoid assuming `$nothing`, #2, or a
  wizard #0 on another engine's database.
- The suspension is `suspend()` with no argument, as `WorldHistory` uses. It is
  one constant (`suspendedStackSuspend`). On a real server such a task sleeps
  until `resume()`; I do not know which form the cross-engine job wants.

## Surprises

- The fuzz driver's root task has no `player` variable: `{player, "r"}` at top
  level ended E_VARNF "Variable not found: player" (root runs through
  `machine.Run(program)`, `vm/fuzz_test.go:122`, not a verb context). The
  generator uses `o.owner` instead, so no shape was dropped. A real server
  task has `player`.
- `scripts/test_fuzz_compute.py:427` pins the matrix at 18 lanes, and line 429
  requires 30 minutes for any lane outside its `saturated` set. Adding a lane
  means changing that count to 19.
- `cmd/moo-fuzz-source` has no generator selection today: it only calls
  `fuzzseed.Source` (`cmd/moo-fuzz-source/main.go:59,88`). `WorldHistory` is
  not renderable by it either.

- The native target does not detect the PR 886 bug. I copied the generator and
  the tests onto the commit before that fix (the merge of PR 884) and both
  tests passed there, eval seeds included. The fix's commit message says why:
  Barn refused Toast's eval mark on import and omitted it on export, so a
  checkpoint Barn wrote and Barn reloaded agreed with itself. This lane checks
  Barn against Barn; only a job that moves the checkpoint between engines sees
  an error made the same way in both directions.
- The smoke's execution rate was bursty on the Windows workstation (stretches
  of 0/sec between bursts). I did not investigate; each execution writes and
  reloads a checkpoint per suspension.

## Unverified

- That every rendered program is valid on ToastStunt. It uses create,
  add_property, add_verb, set_verb_code, eval, suspend, raise, tostr, fork,
  try/except, try/finally, for over list/range/map (`for v, k in (map)`),
  while and scatter with `?optional = default` and `@rest`; no program was run
  on Toast.
- The expected values in `TestSuspendedStackLayersMarkResult` are Barn's.
- The setup adds `suspended_stack` to #0, so a second run on the same database
  fails at `add_property`; a consumer needs a fresh database per program.

## Next step

Lead pushes and opens the PR. The cross-engine checkpoint job consumes
`fuzzseed.SuspendedStack` or `moo-fuzz-source --generator suspended-stack`, and
decides the suspend form (`suspendedStackSuspend`).
