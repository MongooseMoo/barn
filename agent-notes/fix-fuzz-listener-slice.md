# Fuzz listeners: one shared memory bound instead of 3500M each

## Goal
The per-listener `MemoryMax=3500M` added by PR #709 (.github/runner/listener.conf)
killed FuzzReadDatabase and failed acceptance on every continuous cycle from
09:58 UTC on 2026-11-10. Q chose (2026-11-10, "One shared 14G cap") to put all
four listeners in one slice with `MemoryMax=14G`.

## State
- `.github/runner/barn-fuzz.slice` (MemoryMax=14G, MemorySwapMax=0) added;
  `listener.conf` now sets `Slice=barn-fuzz.slice` and no per-unit bound;
  install-listener.sh installs the slice; README updated.
- Live guest, done ~16:55 UTC: slice installed and active (MemoryMax read back
  15032385536); drop-in updated; daemon-reload. Per-listener cgroup
  memory.max read back `max` on all four at once, so the 3500M kills stopped
  then. Listeners join the slice only on restart: a scratchpad script restarts
  each one only while GitHub reports its runner idle.
- `scripts/fuzz_ops.py`: probe reports each listener's actual ControlGroup and
  the slice's MemoryMax/MemoryCurrent; status flags a listener outside the slice
  or an unbounded slice. Fixture guest-probe-outside-slice.txt is the real
  output before the restarts; guest-probe.txt is replaced with the real output
  after them.
- barn-b2 (other session) told what changed and that brief runner restarts are
  mine.

## Decisions and why
- Evidence for the change: the kernel task table at the 12:18:44 kill of
  format.test (listener 2) lists only FuzzReadDatabase's coordinator
  (56367 pages) and two workers (410389 and 378408 pages); memcg usage
  3583992 kB of 3584000 kB. Same pairs (vm.test, then format.test) at 09:58/10:06
  (listener 1), 12:06/12:18 (2), 14:16/14:27 (1), 16:27/16:34 (4).
- The runner gives job processes oom_score_adj 500 and its own 0 (same table),
  so a slice or guest OOM prefers job processes; OOMPolicy=continue and
  Restart=always keep the listener up.
- The probe reads ControlGroup, not Slice=: the unit property shows the new
  slice before the restart that actually moves the listener.

## Surprises
- The 3500M number came from one reading (2670 MB for three lanes); too thin
  for a bound that kills lanes.
- No scheduled watchdog run has happened since #709 merged at 08:59 (checked at
  16:48): no schedule-event run of fuzz-controller.yml exists; workflow state
  active; master has the cron. GitHub docs list default-branch, load delay/drop
  and 60-day inactivity, none of which fits. Cause unknown; not investigated by
  trial commits.

Related, 17:24 UTC: PR #726 (restore fuzz harness deadline) merged, so the
restore lane's a67f1c049e52d97e no longer OOMs. Its first queue run had failed
on a Jepsen harness race (two crash operations on one generation; no checkpoint
violations); barn-b2 owns that harness fix.

As of 17:30 no listener has been idle long enough to restart: each picks up its
next lane within about a second, so the restarts mostly happen at cycle end.

## Next step
When all four listeners are in the slice: capture guest-probe.txt, run
scripts/test_fuzz_ops.py, commit, push, PR, queue. Then watch the next cycle's
FuzzReadDatabase lane finish without a kill.
