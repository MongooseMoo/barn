# Jepsen slow-reader client recovery failure

## Goal

Find, with evidence, why GitHub Actions run 44593867810 (merge queue run for PR 919, job 135815655477) of the
Jepsen checkpoint campaign reported "no bounded recovery" for transport fault 57 (slow-reader, generation 12)
when the server itself had recovered. Diagnose only; no code changes.

Also state the real failing step of runs 44568208679, 44567116806 and 44439232813.

## State

Diagnosis complete (2026-11-19). No code changed, nothing rerun, nothing posted.
Evidence artifact is unpacked in WSL Debian at `~/jepsen919/a/x/` (root's home); it can be deleted.
Script and workflow were read at the run's commit e3fef382 (`scripts/jepsen_checkpoint.py` there is identical to
origin/master 042af478).

## Findings (facts, with evidence)

### Run 44593867810 (the slow-reader failure)

F1. Failing step: step 11 "Run campaign" of job "Checkpoint durability campaign". The step runs two campaigns
(`.github/workflows/jepsen.yml:144-151`): the main lane and the `--control transport-stall` control.
- 19:43:13.96 main lane: `recovery-failure: transport fault 57 (slow-reader, generation 12) ...`, then
  `verdict: recovery-failure`. This set `status=1` and is the only reason for exit 1.
- 19:44:37.93 control: `verdict: recovery-failure` followed by
  `control transport-stall: witness stalled-recovery reported`. Expected; the "transport fault 6/7 (pause)" lines
  belong to this control.
- Everything else in the main lane was clean: 13 checkpoint comparisons with 0 violations, Elle true for generations 0-12.

F2. Journal timeline (`jepsen-evidence/journal.jsonl`, `lifecycle.jsonl`), seconds relative to
`heal_ns` = 489668438695728:
- -1.112 `fault-begin` fault_seq 57 (transport, slow-reader)
- -0.616 .. -0.446 `request` rows for ops 6640-6643
- -0.117 `transport-fault-applied` fault 57 on op 6640
- -0.094 .. -0.005 `send` for 6641, 6642, 6643, 6640
- +0.004, +0.014, +0.017 `complete` ok for 6642, 6641, 6643; +0.212 `complete` ok for 6640 (the slow-reader op).
  These are the four completions in `client.failures {'ok': 4}`: all sent before the heal.
- +0.333 probe ok (token 47); +0.505 checkpoint seq 27 complete.
- +0.212 .. +10.011: no journal rows of any kind: no `request`, no `send`, no `complete`.
- +10.011 `fault-end` fault 57; +10.016 `request` 6644, +10.017 `send`, +10.021 `complete` ok; normal traffic after.

The `request` row is written first thing when the adapter receives a client transaction
(`scripts/jepsen_checkpoint.py:2170`), and that path takes no lock held by the fault (`_run` at 2177-2183 takes only
`campaign.lock` briefly; the fault holds `campaign.lifecycle`, 2103). So no client transaction reached the adapter for
9.8 s. The clients were not sending. The server was not failing to reply (it answered in 3-5 ms on both sides of the gap).

F3. Jepsen's own history (`jepsen-evidence/history.jsonl`, time in seconds since test start) shows why:
- Transport phase: first client invoke at 224.4377 (index 10342, keys in the 5000000 range), last at 284.1333
  (index 13390). The phase is `(gen/time-limit phase-transport (gen/clients ... (gen/cycle transport)))` with
  `--phase-transport 60` (`tests/jepsen-checkpoint/src/barn_checkpoint/core.clj:205-206`, `jepsen.yml:145`), so its
  limit fell at about 224.44 + 60 = 284.44.
- Nemesis op `transport slow-reader` invoked at 283.461 (index 13382), completed at 294.594 with
  `recovered: False` (index 13399).
- The heal is at about 284.58 in Jepsen time (294.594 - 10.011), about 0.14 s after the phase limit.
- Indices 13391-13398 are only completions; there is no client invoke between 284.133 and 294.594.
- The next client invoke (index 13400, at 294.594, the same instant the nemesis op completed) has keys in the
  4000000 range, which is the closing phase (`core.clj:207`).

So the transport phase's 60 s time limit expired while fault 57 was still in flight. After the limit the generator
issues no more client operations for that phase, and `gen/phases` does not start the closing phase until every
outstanding operation, including the nemesis op, has returned (stated in the generator's own docstring,
`core.clj:180`). The nemesis op was inside `recover()` (`jepsen_checkpoint.py:2057-2071`) waiting for a client
completion with `sent_ns >= heal_ns`. Each side waited for the other until the 10 s bound ran out. The client clause
could not pass in this run regardless of server health.

F4. Why the fault was still in flight at the limit: the cycle is `sleep 2` then one transport op per kind
(`core.clj:193`), and in this run the third cycle's slow-reader op started 59.02 s into the 60 s phase
(283.461 - 224.438). The earlier slow-reader faults (43, 50) took 0.35 s from fault-begin to fault-end. Fault 57 took
0.995 s just to be picked up by a client transaction (fault-begin -1.112, applied -0.117), then another 0.33 s for the
slowed reply, which carried the heal past the limit.

F5. During the last 2 s before the heal everything on the runner was slow: request-to-send in the adapter took
0.4-0.6 s (normally about 2 ms), there is a 1.0 s hole in Jepsen client invokes (282.959 to 283.963), and the server's
recovery checkpoints took 134 ms and 152 ms where the previous four took 23-26 ms (`gen-012/logs/latest.jsonl`,
12:41:10.55 and 12:41:14.18 local). The generation 12 server log has no error in that window.

### The three comparison runs: each has a different failing step and cause

- 44439232813 (pull_request, branch fix/native-finally-reasons): job "Checkpoint durability campaign", step 11
  "Run campaign", MAIN lane, 01:51:44:
  `recovery-failure: checkpoint: ValueError: suspended task 1453 has no saved program fingerprint in its own section (it is in Toast's frame form)`,
  then `verdict: recovery-failure`, with 6477 `fail:no-live-generation` and "zero applied" for every transport kind.
  The "transport fault 6/7 (pause) ... probe timed out" lines at 01:53:02 are the transport-stall control, which
  reported its witness as expected. So this run did not fail on a pause fault; it failed because the campaign's
  checkpoint step raised a ValueError on a suspended task in Toast frame form.
- 44567116806 (merge queue, PR 903) and 44568208679 (merge queue, PR 904): job "Checkpoint stage schedule and negative
  controls", step 12 "Run stage schedule and controls". The durability campaign job passed in both. The stage schedule
  itself printed `verdict: healthy`, and the six earlier controls reported their witnesses. The one failing line is the
  last control: `control hot-memo-guard: witness elle:G2-item MISSING (observed ['elle:G-single-item'])` (17:00:44 and
  17:24:56). Elle rejected the control history with G-single-item instead of the G2-item the control requires.
  Neither failed-step log contains "Input/output error" (0 matches).

## Hypotheses

- H1: workload clients were not sending in the 10 s after the heal. CONFIRMED (F2, F3): the transport phase's time
  limit had expired and the generator was waiting for the nemesis op.
- H2: clients were sending and the server was not replying. REFUTED (F2).
- H3 (theory, not proven): the slowness in F5 was host contention on the shared runner machine. Supported only by the
  simultaneous slowdown of adapter, Jepsen and server checkpoint; no load measurement exists in the evidence.
- H4 (theory): the same structural window exists for the pause and crash-restart kinds, and for any run where a
  transport op is in flight when the 60 s limit falls; normal margin for the third slow-reader op is about 0.6 s. Not
  measured across other runs.

## What is unverified

- H3 and H4 above.
- Whether the hot-memo-guard misses in the two 16:30-17:25 UTC runs relate to the /mnt/c Input/output errors of that
  time. Their logs do not show such an error; I did not download their evidence.
- The cause of the ValueError in 44439232813 (branch code, not examined).
- The exact instant Jepsen's time-limit started (taken as the first client invoke of the phase, 224.4377).

## Next step

Decision for the owner of the campaign: the recovery check's client clause must not be evaluated against a workload
that has stopped issuing operations. Two places could change: the generator (do not start a transport op that cannot
finish its recovery inside the phase, or keep clients running until the nemesis op returns) or `recover()` (tell
"no client transaction was requested after the heal" apart from "requested and not completed ok"; the journal already
has the `request` rows needed). Not done here: diagnosis only.
