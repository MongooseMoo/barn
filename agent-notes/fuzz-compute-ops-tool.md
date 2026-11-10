# Continuous compute job: status and management tool

## Goal
Q 2026-11-10: "Please write me tooling to check on the current state of the
continuous compute job -- to mange it in general", then "we'll need this to be
more self-healing", then "Make it so -- there's another claude code working in
here too coordinate with it". The job is the dedicated continuous fuzz chain:
`.github/workflows/fuzz.yml` dispatched with `compute-mode=dedicated-continuous`,
successors submitted by `fuzz-controller.yml` through
`scripts/fuzz_compute.cjs`, running on runner group 4 `barn-bug-discovery`
(listeners mongoose-barn-fuzz-1..4 in the KVM guest on mongoose.world).

## State
Built on branch feat/fuzz-compute-ops:
- `scripts/fuzz_ops.py`: `status [--host] [--json]`, `stop [--cancel]`,
  `start [--campaign]`, `runner restart N`. Run live against the real chain:
  works, about 19 s with --host. Tests `scripts/test_fuzz_ops.py` (7, pass) on
  raw output saved in `scripts/fixtures/fuzz/ops/`.
- `watchdog()` in fuzz_compute.cjs, plus a `schedule` (minutes 7 and 37) job in
  fuzz-controller.yml. Shared helpers `accepted`, `dedicatedCycleActive`,
  `dispatchSuccessor` now used by replenish too. test_fuzz_compute.py: 24 pass.
- `.github/runner/listener.conf` drop-in (Restart=always, RestartSec=30,
  OOMPolicy=continue, MemoryMax=3500M, MemorySwapMax=0); install-listener.sh
  installs it; README documents applying it to existing listeners.
PR #709 opened on Q's "open the pr" (the pre-PR hook had refused the first
attempt: its .q-words.jsonl held only barn-b2's message, not Q's "Make it
so"); auto-merge queued via the merge queue; CI pending.

Live guest, done 08:30 UTC: listener.conf installed for slots 1-4,
daemon-reload; read back Restart=always, OOMPolicy=continue,
MemoryMax=3670016000; running listeners' cgroup memory.max is 3670016000, so
the bound applied without a restart (verified). `fuzz_ops.py runner restart 2`
brought listener 2 back (active); `runner restart 1` refused (exit 3).
`status --host` then showed no listener or runner problems.

Probe fixed after a live run: the reload made systemd log "system.slice: A
process of this unit has been killed by the OOM killer" at 08:30:17 (same
second as "Reloading."); the kernel logged one oom-killer in 24h (06:43) and
system.slice memory.events oom_kill is 1. The probe now counts only kernel
`oom-kill:` lines. Fixtures: guest-probe-before.txt (old probe, listener 2
failed), guest-probe.txt (new probe, after repair).

Coordination: other session barn-b2 works on claude-fix.yml and
fuzz_report.py publish(); no file overlap. It holds the #694/#695 claude-fix
dispatches until I tell it the OOM diagnosis (agent
oom-restore-vmsnapshot-diag, note agent-notes/fuzz-oom-restore-vmsnapshot.md)
is done.

## Decisions and why
- Python + gh CLI: matches scripts/fuzz_*.py and their unittest CI lane.
- Watchdog never restarts after a cycle that failed acceptance: that is
  replenish's stop check; getting past it would loosen a check (Q's call).
- Controller concurrency group is keyed by event: GitHub cancels an older
  pending run when a newer one queues in the same group, so a scheduled run
  could otherwise cancel a pending replenish run.
- Per-listener MemoryMax: the 06:43 kill was `global_oom` with
  task_memcg=.../mongoose-barn-fuzz-2.service, task=vm.test (guest journal).
  A cgroup bound turns that into a kill inside the lane's own listener.
- fuzz_ops reads the acceptance result and the controller's own log line
  instead of reimplementing acceptance.

## Surprises
- svc.sh's unit has no Restart= (systemd default `no`); after the OOM kill
  runsvc.sh logged "no retry needed" and the unit stayed `failed`.
- Issue #705 was closed at 08:22 by someone else, between my two reads.

Unverified: whether a lane OOM-killed inside its cgroup passes Accept. Whether cycle
42766490058 passes Accept with its Canceled lane.

## Next step
Watch PR #709 CI to merge; fix any red. After merge, see the first scheduled
watchdog run log a "Watchdog:" line (status shows it). When cycle
42766490058 ends, check its acceptance and that a successor follows. Remove
this worktree and branch after merge.
