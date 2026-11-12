# test/fuzz-populated-world

## Goal

Ranked item 1 of the fuzz lane coverage pass: run `vm/FuzzSourceExecution`
and `vm/FuzzStructuredSourceExecution` in a small populated world instead of
`dbstore.NewStore()`, as both a wizard and a programmer, so verb calls,
property access and pass() reach real paths instead of failing at the first
object lookup.

## State

Built and verified locally (see Verification). Not pushed; no PR.

## Decisions and why

- World built by a MOO setup program run as the wizard `#0` on the existing
  `newCheckpointResumeStore` (vm/checkpoint_resume_test.go), not a new Go
  builder. `internal/dbtool.NewMinimalStore` was not usable: dbtool imports vm,
  so vm's internal tests cannot import it.
- Layout (vm/fuzz_world_test.go): `#4` root, `#5` middle (child of `#4`),
  `#3` leaf (child of `#5`, programmer-owned), `#6` programmer player; `#0`
  and `#2` reparented under the chain after their own `p` is deleted, so every
  object the structured generator names (`#0`..`#3`) dispatches its verb names
  (v, look, foo, initialize) and its property `p`. `#1` stays a hole.
- Fresh copy per execution: the world is written once per process with
  `dbformat.WriteCheckpoint` into `f.TempDir()`, and each execution loads it
  with `LoadDatabaseReadOnly` + `NewStoreFromDatabase`. One loaded `Database`
  cannot make two stores (`ObjectBuilder.Build` returns its own object), so
  each execution re-reads the file.
- Wizard and programmer: each input runs twice. The corpus record stays one
  value, because `scripts/fuzz_differential.py` `decode_corpus_file` and
  `is_choices_record` accept only single-value records; adding a principal
  byte would have dropped every retained record from the differential.
- Tick budget: separate constant `sourceFuzzWorldTicks`; `sourceFuzzTicks`
  (used by the checkpoint and restore lanes) is unchanged. Kept at 100.
  Measured on the retained structured corpus (7060 inputs, Windows, coverage
  replay): 300 and 1000 ticks both gave 35.3% against 34.5% at 100 (before the
  last verb change), but at 300 one input took 11.5 s and at 1000 the same
  input took 51 s (a named while doubling a list up to the 64 MB limit; the
  100 ms deadline is only checked at tick checkpoints). A world verb chain
  costs 11 to 22 ticks, so 100 still fits several calls.
- The generator's verb names (v, look, foo, initialize) are the verbs with the
  richest bodies; `leaf:initialize` calls eval, so pushEval is reachable from
  the structured lane. Raw-lane seeds (`sourceFuzzWorldSeeds`) call every
  world verb on `#3`.

## Verification (Windows workstation, shared with other load)

Coverage over vm, compiler, builtins, types, bytecode, task and db/store,
measured as in the fuzz coverage report (the CI retained corpus copied into
testdata and replayed with `-coverpkg=./...`; before numbers reproduce the
report exactly). Every retained input passes on the new harness.

- Structured lane: 23.5% to 34.9%; vm 47.9% to 61.3%; startVerbCallResolved
  12.8% to 89.4%, executePass 0 to 84.4%, getBuiltinProperty 0 to 56.4%,
  pushEval 0 to 100%.
- Raw lane: 21.2% to 29.4%; vm 41.3% to 52.2%.
- About 2850 of the new statements (most of the db/store gain) are world setup
  and loading, measured with the empty structured program alone (4.3% before,
  15.0% after).
- Two minutes of fuzzing each (`-parallel 2`, `-fuzzminimizetime 1x`, empty
  cache): raw 110 to 108 execs/s; structured 96 to 85 execs/s. The corpus each
  run found, replayed on its own code: raw 18.4% to 27.8% (vm 30.3% to 45.5%),
  structured 19.9% to 30.9% (vm 42.5% to 53.1%).
- With the default `-fuzzminimizetime` (60 s) both lanes, before and after,
  stall for up to a minute per new input while Go minimizes it; the structured
  lane ran about 290 execs in three minutes either way.

## Surprises

- `set_verb_code(obj, "w*rite", ...)` raises E_VERBNF; the wildcard name is
  found by its full spelling `write`.
- The scratchpad's `go.work` (coverage agent) breaks `go test` in a nested
  worktree; use `GOWORK=off`.

## Next step

Push the branch and open the PR when the parent agent says so. Open follow-up,
not in this branch: `.github/workflows/fuzz.yml` runs campaigns with the
default 60 s minimization per new input (see Verification).
