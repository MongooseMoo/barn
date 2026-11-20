# Fix Jepsen empty generation window

## Goal

The required "Checkpoint durability campaign" CI job flaked on barn PR #751
(run 42984792666): verdict incomplete-experiment (exit 4, 0 violations) because
generation 14, started after a checkpoint-crash of generation 13, received no
workload before it was checkpointed and stopped. Find why the harness can
start a generation with no workload window and fix that cause, without
weakening any obligation.

## State

- Evidence verified from the run's `jepsen-evidence` artifact (attempt 1):
  - lifecycle.jsonl: gen 14 `probe reason=launch` at monotonic_ns
    900375149078183, then `observation reason=checkpoint keys=[]` at
    900375153662294 (4.6 ms later), then the final `checkpoint` of gen 14.
  - results.json: parameters phase1=60 phase3=90 phase_live=60 rate=20;
    all 12 missing obligations are gen 14 ("after checkpoint-crash of
    generation 13" / "before checkpoint of generation 14", no successful
    read/write in smoke, props, keyed).
  - history.jsonl: gens 7-13 (live lane) each had 32-95 ok transactions per
    family.
- Root cause (tests/jepsen-checkpoint/src/barn_checkpoint/core.clj,
  `generator`): the live phase is `(gen/time-limit phase-live ... (gen/cycle
  live))` where `live` ends with `:checkpoint-crash` (stop, kill, restart
  Barn as a new generation). The next phase is the final
  `(gen/nemesis (nemesis-op :checkpoint))`. gen/phases waits for the
  in-flight checkpoint-crash, so when phase-live's limit expires during that
  op, the restarted generation is checkpointed immediately with no client
  op. Here 8 cycles of ~7.5 s fill 60 s, so the limit landed on the 8th
  restart. The same happens after phase3's `:crash` when phase-live is 0.
  The stage generator already avoids this: it ends with a client-only phase
  before its final checkpoint.
- Fix (done): `generator` now runs `(gen/time-limit closing-seconds
  (gen/clients (txns rate families 4000000)))` between the last fault phase
  and the final checkpoint; `closing-seconds` is 6. README paragraph added.
- Test (done): `LateRestartTests` in scripts/test_jepsen_checkpoint.py, a
  lifecycle fixture with the run's shape (gen 13 checkpoint-crash, gen 14
  admitted and probed, then the final checkpoint). With no gen 14 client
  ops `obligations` reports exactly the four gen 14 INCOMPLETE messages; with
  one closing-phase transaction (key 4000001) it reports none. The test
  covers the obligation contract the schedule must meet; the Clojure
  generator has no unit test harness (see Surprises). Local run: 113 tests OK.
- Compile check: `lein uberjar` in WSL with the runners' Temurin 21 (WSL's
  default java is 17 and cannot load bifurcan's class version 65).

## Decisions and why

- Fix the schedule, not the check: the obligation that every non-final
  generation gets successful reads and writes per family
  (scripts/jepsen_checkpoint.py `obligations`, `useful(generation + 1, ...)`)
  is correct and stays unchanged.
- New phase key offset 4000000: keys are decoded only by
  `key // FAMILY_SPAN` (scripts/jepsen_checkpoint.py `family_of`), and each
  la/gen instance needs keys disjoint from every other phase in the run
  (0, 1000000, 2000000 here; stage runs use 3000000 + 100000 * step and do
  not use this generator).
- No Python harness change, so PR E (#750, jepsen_checkpoint.py) has nothing to
  merge against.

## Surprises

- The repo has no Clojure test suite; CI only runs `lein uberjar`. The
  pinned jepsen 0.3.15 jar ships no generator simulator
  (jepsen.generator.test is upstream test code), so the schedule change is
  exercised only by the CI campaign. Because the flake depends on timing,
  one green campaign does not prove the fix; the argument is structural:
  every restart is now followed by a client-only phase before the final
  checkpoint.

## Next step

Edit core.clj `generator`, add the fixture test in
scripts/test_jepsen_checkpoint.py, run it, open the PR.
