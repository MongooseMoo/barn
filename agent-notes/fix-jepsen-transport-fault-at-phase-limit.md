# Fix: Jepsen transport fault in flight at the transport phase limit

## Goal

Stop the Jepsen checkpoint campaign from reporting a false "no bounded
recovery" when a transport fault's nemesis op is still in flight when the
transport phase's time limit expires (GitHub run 44593867810; diagnosis in
`agent-notes/jepsen-slow-reader-client-recovery.md` of the main checkout, not
committed there). Fix the generator's scheduling; keep the recovery check
exactly as strict.

## State

Done locally, committed on `fix/jepsen-transport-fault-at-phase-limit`. Not
pushed, no PR (the lead does both). The campaign itself has not been run with
this change; CI runs it on the PR.

Changed:

- `tests/jepsen-checkpoint/src/barn_checkpoint/core.clj`, `generator`: the
  transport phase was
  `(gen/time-limit phase-transport (gen/clients txns (gen/cycle transport)))`,
  one limit over clients and nemesis. It is now
  `(gen/shortest-any (gen/clients txns) (gen/nemesis (gen/time-limit phase-transport (gen/cycle transport))))`:
  the limit is on the nemesis cycle only, and the clients stop when the
  nemesis generator is exhausted.
- `tests/jepsen-checkpoint/test/barn_checkpoint/core_test.clj` (new): the
  project's first Clojure test.
- `.github/workflows/jepsen.yml`: a `lein test` step before the uberjar build
  in the "Checkpoint durability campaign" job, so the test runs in CI.
- `tests/jepsen-checkpoint/README.md` (transport lane paragraph) and
  `.gitignore` (`.lein-failures`, which `lein test` writes).

Not changed: `scripts/jepsen_checkpoint.py` (`recover()`, `RECOVERY_BOUND`,
`transport_checks`), any flag, the other phases.

## Decisions and why

- `gen/shortest-any` instead of a nemesis margin. A margin (stop starting
  faults N seconds before the client limit) needs the fault duration and the
  recovery bound to be known to the generator and is still only as good as its
  estimate: in the failing run a slow-reader fault took 0.995 s just to be
  picked up. `shortest-any` gives the guarantee structurally and is the
  library's own combinator for this ("a workload generator you'd like to run
  while doing some sequence of nemesis operations, and stop as soon as the
  nemesis is done", `jepsen/generator.clj:1064-1071` of `io.jepsen/generator`
  0.1.6, the generator library Jepsen 0.3.15 depends on).
- Why it holds, from that source: `ShortestAny.op` (1041-1056) ends only when a
  child returns nil. While the nemesis thread is busy the nemesis child returns
  `:pending`, not nil: `OnThreads` filters the context to the nemesis thread
  (945-949), and a map op with no free process is `:pending` (`fill-in-op`
  563-600, map generator 629-633). `TimeLimit.op` (1569-1583) passes `:pending`
  through and returns nil only for a real op whose time is past the cutoff,
  which needs the nemesis thread free, so only after the in-flight transport op
  (fault, heal and `recover()`) has returned. Until then the client child keeps
  emitting.
- The phase can now run past `--phase-transport` by at most one sleep (2 s) plus
  one transport op. No transport op starts after the limit (the test asserts
  the count).
- Transport kinds cannot be empty when the phase is on
  (`scripts/jepsen_checkpoint.py:4112-4115` rejects it), so the nemesis child is
  never exhausted at once.
- Scope: only the transport phase. Only `transport()` calls `recover()`; the
  other phases' nemesis ops have no client clause.
- CI step: `lein test` is its own step in one job (the campaign job), not both
  jobs that build the uberjar.

## What was exercised

- `lein test` in WSL Debian on the runners' Temurin 21.0.12 (the workflow's Java
  version), test
  `transport-fault-in-flight-at-the-phase-limit-keeps-its-clients`:
  `core/generator` with only the transport phase (10 s, kind `pause`), driven by
  `jepsen.generator.test/simulate` on a simulated clock, 4 clients and the
  nemesis; sleeps last their value, a transport op 5 s, everything else 1 ms.
  The second transport op starts at 9.003 s and ends at 14.003 s.
  - Before the change: `1 failures` of 4 assertions: "no client transaction was
    invoked in the last 1 s of the transport op invoked at 9.003 s" (last client
    invoke at 9.995 s). The other three assertions passed.
  - After the change: `Ran 1 tests containing 4 assertions. 0 failures, 0 errors.`
- Two concurrent `lein test` runs in two separate copies of the project (as two
  runners of the same WSL user): both exit 0.

## What was not exercised

- The real campaign (main lane and `--control transport-stall`) with this
  generator: not run locally by instruction. The claim that the control still
  reports `stalled-recovery` rests on reading: `recover()` and
  `transport_checks` are untouched, and the only schedule difference is that
  clients keep being issued while the last transport op is in flight.
- The real interpreter (`jepsen.generator.interpreter`) with real time; the test
  uses the library's simulator.
- The edited workflow step on a runner.
- Kinds other than `pause` in the test (the schedule does not depend on kind:
  the kind is only the op's `:value`).

## Surprises

- Jepsen 0.3.15's jar has no `jepsen/generator.clj`; the generators are the
  separate artifact `io.jepsen/generator` 0.1.6
  (`~/.m2/repository/io/jepsen/generator/0.1.6/` in WSL Debian), which ships the
  sources and `jepsen/generator/test.clj`. The jepsen-io/jepsen GitHub repo has
  no `v0.3.15` tag to read instead.
- Java 21 for a local `lein test`: WSL's default is Java 17; the runners' tool
  cache has Temurin 21 at
  `/home/github-runner/actions-runner-barn/_work/_tool/Java_Temurin-Hotspot_jdk/21.0.12-101.0.LTS/x64`
  (set `JAVA_HOME` and `JAVA_CMD` to it).

## Next step

Lead: push, open the PR, watch the Jepsen workflow (new step "Test Jepsen
generator", then both campaign lanes).
