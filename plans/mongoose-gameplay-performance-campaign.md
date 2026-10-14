# Mongoose gameplay performance and memory plan

## Objective and scope

Make Barn run substantial Mongoose gameplay faster with less memory. The offline
setup already largely exists. The first missing deliverable is a representative,
verifiable gameplay workload: ships actually fly, spells actually take effect,
world services progress, and the resulting work and retention are measurable.

The [offline runner](../docs/mongoose-gameplay-runner.md) now implements the
initial flight and Lumos/Nox workloads and their collectors. The larger workload
matrix below remains a plan; no engine speedup or memory reduction is established.
Optimization follows evidence.
Keep gameplay rules, permissions, authored delays and native errors visible.
Production game changes and deployment are separate from this offline campaign.

## Inspection evidence

- Barn inspection revision: `9c705a1a8ff90243202742f9b8d0af9743933eb8`.
- Existing `engine/mongoose_real_bench_test.go` mixes only look (35), say (30),
  inventory (10), who (10), and home (15). It supplies command dispatch,
  terminal acknowledgement, profiles and diagnostics worth reusing.
- Inspection snapshot: `C:/Users/Q/projects/mongoose/db/mongoose.db.new.2026-10-12T2349`;
  216353099 bytes; SHA256
  `FAB6957ACFFA1DB8BC1C4B911FB53A8E8D6E605F4DE2A31B9F8E4FA874C3200D`.
  `moodb --no-cache info` returned exit 0: 54330 objects plus 1501 anonymous,
  21342 verbs, 437 players and 47593 waifs. This proves a full local parse,
  not freshness against the live host or successful gameplay.
- Snapshot source: console feature #3954 exposes man/unman, setspeed, heading,
  course/autopilot, warp/engage, scan, dock/undock, fire, harvest and repair.
  #4158:authorize_action requires actual uplink/permissions and station use.
  #4158:find_online_computer resolves an installed online computer.
- Snapshot source: #16980:weave authenticates the actor and calls
  $hp_magic:start. The real Lumos #20835 and Nox #20959 specs have enabled=1,
  and tutor #19898 lists them. #17318:learn invokes ordinary training.
- $hp_training:test_e2e_lumos checks actual wand light, room brightness,
  resource spending, recovery and duplicate completion. Its _e2e_fixture uses
  private enabled spell copies and sets proficiency to 85%; reuse its assertion
  ideas, not those shortcuts as proof of ordinary gameplay.
- Source inspection alone does not prove a scenario works. Historical space
  notes distinguish functioning paths, incomplete prototypes and authored packs.
  Do not select a ship merely because its name matches Nova: #5063 had no cycle
  method under either inspected spelling. Resolve the actual runtime instance,
  installed systems, interior and universe registration before choosing fixtures.

References: Mongoose hub README.md; notes/testing.md; notes/events.md;
notes/space/implementation.md; notes/hp-magic-functionality-inventory.md;
Barn docs/workload-diagnostics.md and docs/mongoose-performance.md.

## 1. Specify and prove two complete gameplay scenarios first

Start with flight and real Lumos/Nox. Each specification contains setup, actor
roles, explicit command arguments, allowed transitions, terminal conditions,
independent state assertions, deadlines and cleanup. Discover exact instances
and current command syntax from the pinned checkpoint; never guess menu IDs.

Use private disposable worlds and legitimate domain construction paths. Crew
and pupils act with ordinary permissions; privileged setup is recorded and
outside timing. Preserve normal training, focus, recovery and station admission.
Real catalogue spells and functioning ship installations are prerequisites.
No successful benchmark may substitute rejection messages, wizard bypasses,
enabled spell copies or artificial proficiency for the intended game action.

Prove the selected scenarios on the application-compatible Mongoose Toast fork
and Barn through the documented managed paths. Keep that fork distinct from the
stock WSL Toast semantic oracle. Uncertain MOO behavior must be verified against
the appropriate managed oracle before Barn diagnosis or repair.

Done when each scenario produces a trace of commands, real terminal effects,
independent assertions and cleanup evidence. A command task returning or printing
an acknowledgement is insufficient. A failed admission/mechanic is retained as a
functional finding, not counted as productive work or silently repaired.

## 2. Expand to the gameplay matrix

| Scenario | Actual activity | Required independent evidence |
| --- | --- | --- |
| Flight and navigation | Crew man stations, power an installed ship, launch/undock where appropriate, change heading/speed, set a course, engage/disengage autopilot or warp, then stop | Universe advances; ship position/velocity and navigation state change as specified; power/fuel accounting follows the installed systems; crew/console memberships remain reciprocal |
| Sensors and encounters | Multiple moving ships acquire, scan and lose contacts; crew inspect reports and exchange communications | Expected contact membership and identities, spatial-index updates, recipient delivery and bounded cleanup of stale contacts |
| Docking and crew movement | Approach, dock, operate hatches, walk between interiors, undock; transport only where its prerequisites work | Reciprocal hatch links, intended passenger locations and containment, authorized movement and unchanged state on expected denials |
| Ship combat and repair | Acquire an actual target, fire an installed supported weapon, sustain damage and perform a supported repair | Committed impacts, shield/hull/system changes, actual ammunition/energy use, repair progression and transient-object lifecycle; incomplete weapon molds are not promoted to delivered mechanics |
| Magic and world effects | Lessons/fitting/wielding, real Lumos/Nox, interruption/cancellation, then additional available status/movement spells | Terminal receipt plus actual wand/effect/target state; once-only resource accounting, recovery and expiry/removal; affected and unaffected observer output |
| Character combat and abilities | Ordinary attack/engagement, real rounds and usable abilities against disposable opponents | Actual strikes, damage/resource changes, round progression, termination and legitimate admission/refusal behavior |
| Materials and object lifecycle | Dispense material, fabricate a stocked deterministic item, carry/use/reclaim it | Created usable objects, material/stock accounting, retained versus recycled IDs and cleared jobs; external AI-generated designs are a separate dependency-bearing case |
| Habitat, movement and presentation | Crew traverse populated decks, doors and lifts; interact with room light, heat/atmosphere, gravity and sound while simulation runs | Intended locations and physical state, correct observer/packet recipients, asynchronous completion and preserved world invariants |

Source-present but unusable scenarios remain explicitly unavailable. Do not make
the performance project depend on implementing every unfinished game feature.
Expand the usable matrix as independent functional repairs become available.

## 3. Implement a stateful scenario controller

Extend the existing harness/controller rather than replacing offline startup.
Store scenario definitions and domain helpers in accountable project files.
Each actor retains identity, location, inventory, training, station and action
history across steps. Fixture references use symbolic identities for replay.

Expose a named scenario, seed, scale and duration through one documented run
command. Seed controller choices and record engine-random outcomes; do not claim
identical engine randomness without a supported mechanism. Sample allowed
transitions, not random unrelated command strings. Record successful sequences,
individual actions, legitimate fizzles/refusals and native failures separately.

Wait for domain completion: spell receipts/effects, arrival/docking state,
combat rounds or material jobs. Correlate command roots, forks, continuations and
external-operation generations. Preserve authored preparation and cycle periods;
measure scheduling lateness beyond those deadlines as well as elapsed time.

Retain the existing basic mix as a diagnostic control. Add role-coherent mixed
sessions: navigating crews, nearby traffic, combatants, casters and observers
share one active world. Keep physics, universe cycles and relevant background
jobs running, and record their progress/backlog alongside foreground actions.

Use existing $test_runner/$pbt.state APIs for domain-owned assertions, replay and
fresh-fixture lifecycle. Do not borrow live connections or reset runner state.
Capture computed observer output in the in-process harness and use independent
external sessions for wire validation. Network/media proof is a separate layer
from mock connection delivery, and protocol packets do not prove heard audio.

## 4. Add the diagnostic visibility these scenarios require

- One manifest/report links immutable input hashes, source/binary/controller
  identities, fixture changes, roles, options, expected work and raw artifacts.
- Record command-to-domain-completion timelines and queue/hold reasons; retain
  failed and unfinished steps with their exception frames and observed state.
- Collect Go CPU and MOO ticks/wall profiles with explicit interval boundaries.
  Add bounded execution traces for selected scheduling failures. Report observer
  overhead and collector overhead; overlapping phases must not be summed.
- Rank discarded attempts by scenario/root and relevant conflict keys. Separate
  intended simulation work from discarded retries and collector work.
- Capture heap profiles after load/startup, during sustained gameplay, after
  scenario drain and after natural idle recovery. Track live/retained heap,
  process resident memory, allocation rate, GC, histories/readers, tasks, caches,
  pending finalization and domain jobs. Preserve chronological samples.
- Sample resident memory externally during work; the current harness samples
  it between windows by executing a MOO task. Label forced-GC diagnostic readings
  separately from naturally observed peak/steady-state/recovery measurements.
- Add scheduled-arrival load alongside closed-loop throughput. Measure backlog,
  completion lateness and successful work so stalls cannot disappear when the
  controller merely stops submitting commands. Bound offered load and retained
  artifacts; report inability to keep up.

Done when one report explains a successful flight and magic sequence, their
background work, expensive implementation paths and memory behavior, with raw
evidence for each finding. Test observers using deliberate nonprogress, refusal
and missing-effect controls so a no-op cannot pass the gameplay benchmark.

## 5. Establish the baseline and run the optimization campaign

Run independent fresh-world repetitions. Begin with two isolated scenarios,
then mixed small/medium/fleet populations and sustained play plus idle recovery.
Keep Windows and Linux results separate until the deployment target is selected.
Pin admission/worker capacity, GC/memory settings, sound data, external-service
policy and ordinary game limits. Avoid concurrent CPU-heavy verification jobs.

Report successful scenario/action goodput, command service latency, domain
completion lateness, p50/p99/p999/max with sample counts, deadline failures,
simulation progress, peak resident memory, live retained heap, allocation/GC
cost and recovery. Determine variability before setting keep/reject margins.

Initial search budget: up to eight cheap hypothesis probes and three full
experiments. Candidate order comes from the new gameplay evidence. Existing
retry waste/hot-property contention, resume delay, lighting/visibility and
retention are leads, not established priorities for this expanded workload.
Search prior experiment failures first; do not repeat killed locking/backoff
ideas without explaining what changed.

Confirm one change at a time with alternating baseline/candidate fresh worlds,
identical scenario plans and preserved failed cohorts. Separate diagnostic runs
from uninstrumented comparisons. Keep a benefit only when it exceeds measured
noise, preserves outcome checks and stays within agreed speed/memory/tail-latency
regression margins. Document tradeoffs instead of calling all dimensions faster.
Reserve additional routes/roles/interactions as holdouts before tuning.

Run appropriate Go/race checks, managed MOO conformance, and the pinned Elle
checker for isolation-sensitive changes, preserving explicit verdicts, nonzero
path counts and witnesses. Publication and merge remain separate authorizations.
After each accepted change, reprofile the same gameplay suite and start the next
bounded round; stop or revise hypotheses when a round has no survivor.

## Immediate next action

Run the implemented flight and Lumos/Nox scenarios on an owned Barn world,
preserving the same ordinary-actor and physical/resource outcome checks. Add
Barn process-total CPU accounting and retain its runtime diagnostics, Go CPU
profiles and separately timed heap snapshots. Preserve failures and verify
uncertain shared behavior against the managed Toast oracle before engine fixes.
The Toast ownership-repair ranking does not identify the same Barn hot path;
select the next bounded experiment from Barn's measured workload instead.
