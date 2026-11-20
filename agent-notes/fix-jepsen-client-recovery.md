# fix/jepsen-client-recovery

## Goal
Find why the checkpoint durability campaign (scripts/jepsen_checkpoint.py,
.github/workflows/jepsen.yml) intermittently reports
`recovery-failure: transport fault N (...): no bounded recovery within 10.0s of the heal`
with probe and checkpoint recovered but `client {'ok': False}`, and fix the cause.

## State
Fixed and committed on fix/jepsen-client-recovery (not pushed). CI not run.

## Findings
1. Run 44578291285 (artifact jepsen-evidence, journal.jsonl): the transport
   phase's first request (id 4923) is at monotonic 483747.620 s; fault 57
   (slow-reader) begins at 483807.575 s, 59.96 s in, with --phase-transport 60
   (.github/workflows/jepsen.yml:145). The last transport-phase request (id 6562,
   keys 25000025/25000022, offset 5000000) arrives at 483807.601 and takes the
   fault. No other client request reaches the adapter until fault-end at
   483817.606; the next request (6563, key 4000004 = closing-phase offset
   4000000) arrives 4 ms after fault-end. So the client workers sent nothing
   after the heal: Barn was not withholding service (the probe and checkpoint
   recovered in 0.3 s, and 6563 completed in 7 ms right after).
2. Mechanism: tests/jepsen-checkpoint/src/barn_checkpoint/core.clj:205-206 wraps
   the transport cycle in gen/time-limit phase-transport inside gen/phases.
   When the limit expires while a :transport op is in flight, time-limit emits no
   more client ops and gen/phases waits for the outstanding nemesis op before
   the closing phase. The adapter's recover() (scripts/jepsen_checkpoint.py:2056-2071)
   meanwhile waits up to 10 s for a client op sent after the heal, which the
   generator will not issue until recover() returns. Any transport kind can hit
   this; it fires only when a fault begins within the last moments of the phase.

3. PR #896 (run 44379286772 attempt 1) and PR #900 (run 44439232813): the
   `transport fault 6/7 (pause, generation 1)` failures with PID state 'T' are
   the transport-stall negative control (expected; log line 534 "control
   transport-stall: witness stalled-recovery reported"). Their main-run failures
   were different classes: #896 `live-checkpoint: TimeoutError: timed out`,
   #900 `checkpoint: ValueError: suspended task 1453 has no saved program
   fingerprint...`. They are not this failure. Merge-group failures 44568208679
   and 44567116806 are a stage-control `checkpoint-crash ... waif_identities`
   mismatch, also a different class. Not examined further.

## Decisions and why
- Fix in the generator, not the check: recover() correctly demands a client op
  sent after the heal; the generator withheld them. Clients now run
  `--transport-tail` seconds past the transport nemesis cycle, so every fault
  that begins inside the phase has clients until its recovery window closes.
  Python owns the constants and computes the tail
  (max pre-heal wait + RECOVERY_BOUND) and passes it to the jar.

## Fix
- tests/jepsen-checkpoint/src/barn_checkpoint/core.clj generator: the transport
  phase is `(gen/clients (gen/time-limit (+ phase-transport transport-tail) txns)
  (gen/time-limit phase-transport (gen/cycle transport)))`; new option
  `--transport-tail SECONDS`.
- scripts/jepsen_checkpoint.py: ARM_GRACE (was the 1.0 s literal in
  transport()), TRANSPORT_TAIL = max(ARM_DEADLINE + ARM_GRACE, PAUSE_HOLD) +
  RECOVERY_BOUND (21 s), passed by run_campaign as `--transport-tail`. Each
  transport-phase run now has clients 21 s longer than --phase-transport.
- Tests: tests/jepsen-checkpoint/test/barn_checkpoint/core_test.clj runs the real
  generator through jepsen.core/run! with an in-process client and a nemesis
  modelling recover() (hold 0.8 s, bound 1 s, phase 2.5 s). Before the fix:
  `expected: (= [true] ...) actual: (not (= [true] [false]))`; after: 0 failures.
  Two concurrent copies both pass. scripts/test_jepsen_checkpoint.py
  test_jepsen_gets_the_transport_tail errors without the fix (no TRANSPORT_TAIL)
  and passes with it; all 215 test_jepsen_*.py tests pass.
- .github/workflows/jepsen.yml main job: `lein do test, uberjar` so the
  generator test runs in CI (the stages job still only builds).

## Surprises
- WSL default Java is 17; Jepsen 0.3.15 needs 21. /tmp/jdk21 has Temurin 21:
  `JAVA_CMD=/tmp/jdk21/bin/java lein test` from a copy under /root/jepsen-recovery.
- Writing files with Python text mode on Windows turns LF into CRLF; use
  newline="" or sed -i 's/\r$//' afterwards.

## Next step
Push, open the PR, and watch the Checkpoint durability campaign job.
