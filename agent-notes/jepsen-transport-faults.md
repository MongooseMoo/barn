# Jepsen transport faults and bounded recovery (PR G)

## Goal
Plan section 5 of `plans/jepsen-expansion.md` (Delivery item 4, third part): transport failures at the adapter/SocketTransport boundary on owned connections and the owned process, with classification (`fail` before send, `info` after), late-reply correlation, and a bounded recovery check with a stall negative control.

## State
- Worktree `../barn-jepsen-transport`, branch `jepsen/transport-faults`, from master after PR E (#750) and PR F (#760) merged.
- Implemented: `scripts/jepsen_checkpoint.py` (TRANSPORT_KINDS, RecordingTransport `_send`/`_receive` faults and `receive_late`, `Campaign.transport`/`recover`/`late_reader`/`take_transport_fault`/`completed`, `ClientSession._run` pre-send and timeout hand-off, reconcile of `late-reply`, `transport_checks`, `--phase-transport`/`--transport-kinds`, CONTROLS `transport-stall`), `core.clj` (nemesis sends `:value`, transport phase at key offset 5000000), `jepsen.yml` campaign job (`--phase-transport 60`, stall control run, both archived), README section.
- `python -m unittest test_jepsen_checkpoint` locally: 132 tests OK (new: TransportFaultSocketTests on real socketpairs, TransportSessionTests, TransportCampaignTests, TransportCheckTests).
- Next: PR, CI. The healthy CI run supplies the first measured recovery latencies of the lane itself; check them against RECOVERY_BOUND.

## Decisions and why
Phase-1 design (written before E and F merged):
- One nemesis op `transport` whose value names a kind: `pre-send`, `disconnect-after-send`, `truncate-reply`, `lose-reply`, `slow-reader`, `pause`, `crash-restart`. Client kinds are armed by the campaign and consumed by the next client transaction; pre-send tears the connection down before the `send` journal row (reply `fail`), the after-send kinds act inside RecordingTransport after the eval line is written (reply `info`).
- A read timeout drops the stream from the session at once and hands it to a late reader that journals `late-reply` under the original operation id; reconcile accepts it only for a sent `info` operation. No client retries.
- `recover()` after each transport fault: fresh probe write+read, one live checkpoint complete, and a client transaction sent after the heal completing `ok`, all within RECOVERY_BOUND. Records first-success latency and the failure kinds meanwhile.
- Bound derivation: PR #749's campaign run 42971081227 (runner Q-barn-wsl-x64-03, 32 CPUs): probe 0.287-0.304 s over 30 probes; live checkpoint 8-13 ms (fault op 12-23 ms); first client ok after a heal 0.104-0.316 s typical, max 1.912 s; relaunch 0.86-0.875 s. Healthy worst sum about 2.2 s; RECOVERY_BOUND = 10 s as the explicit ceiling. To be re-measured from the new lane's healthy CI runs.
- Stall control `transport-stall`: pause without SIGCONT; the recovery check must fail at the bound with the owned PID in state T, witness `stalled-recovery`, from lifecycle/journal rows only (no Elle).

- Re-measured on PR #760's campaign run 43006012087 (the merged CI configuration: six families, --tasks, rate 30, dwell 1): probe max 0.299 s over 25; live checkpoints 13-21 ms; first client ok after a heal while clients run max 0.436 s. The one 12.9 s value is the nemesis-only `resume` between Jepsen phases, when no client runs, so it is not a recovery latency. RECOVERY_BOUND = 10 s stands (healthy worst sum about 2.2 s over both runs).

Re-check against merged E and F:
- `CONTROLS` entries without a stage and `control_result` reading findings' witnesses exist (PR E); the stall control adds a `transport` entry there. `Campaign.environment()` sets BARN_ELLE_FAULT for every control; the transport control has no elle fault, so it must not set it (the ordinary binary runs it).
- Closing client phase uses key offset 4000000 (core.clj), so the transport phase uses 5000000.
- `live_checkpoint()` plants held tasks on a generation's first live checkpoint under --tasks, and `_request_checkpoint` resumes a paused server; recover() therefore requests its checkpoint through `server.request_checkpoint` directly with purpose `recovery`, only after the probe succeeded (a stalled server must not be resumed or left with a queued SIGALRM).
- `obligations` requires a probe after every `resume` event and per-family useful work after every `crash`; the pause kind heals with `resume()` and crash-restart reuses `crash()`, so both stay under those existing obligations.
- `effect_checks` and `shaped_checks` treat `info` completions as unknown, so client faults may land on any family's transaction.

## Surprises
- First CI run of PR #762 (run 43009839220): transport checks clean (every kind applied, recovery under 4 s, stall control witnessed). The campaign was still red with a task-lane `lost-task`. The transport crash-restart relaunches from generation 0's verified pair, which rolls back past live plants 2-5. Barn then reissues their task ids and object numbers to plant 6 (tids 2236/2244 shared with plant 3; #431 reused). `Campaign.task_recovery` (jepsen_checkpoint.py:1504) and `task_checks` (:2938-2950) still judge the rolled-back plants by number. This is a harness defect in the task lane, not a Barn defect. Proposed fix: retire plants newer than the launch's baseline generation. Reported to the lead, who approved the retire fix.

Retire semantics (lead's constraints, implemented):
- A generation launched from a verified pair (no restart; kind `crash` or `checkpoint`) whose baseline generation is B (B = -1 for the original fixture) retires every plant with plant.generation > B. Those plants' world was rolled back, and Barn reissues their task ids and object numbers.
- `Campaign.retire` records one `task-retire` row per plant, and `task_recovery` never reads a retired plant again.
- `task_retirements` in the analysis derives retirement independently from the admit rows. `task_checks` skips a plant's recovery rows from its retire generation on.
- Plants at or before B are never retired and stay fully judged, so a lost task there is still a correctness violation.
- Checkpoint-crash restarts (restart set) never retire.
- Fixture `TaskRetirementTests` reproduces the tid collision. It shows the false `lost-task` when retirement is patched out, and that a lost task of a plant at or before B is still caught.
- Unverified, not in scope: a live plant whose checkpoint-crash recovered the unchanged input is also out of its world. Later plants in that generation could reissue its tids. The existing check expects it `absent` at that launch, but later launches still read it. Not observed in CI.

## Next step
Read the merged diff of PR E and F to the harness, then implement.
