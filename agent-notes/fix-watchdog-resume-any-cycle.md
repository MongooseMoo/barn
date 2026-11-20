# Watchdog resumes the chain after any cycle

## Goal
Q 2026-11-10: "the no watchdog needs to be fixed now". Q then chose:
"Restart after any cycle" (the watchdog resumes after an unaccepted cycle too)
and "Keep GitHub cron" as the trigger.

## State
- `watchdog()` in scripts/fuzz_compute.cjs no longer stops on an unaccepted
  cycle: once the newest continuous cycle has been over for 30 min with nothing
  queued or running, it dispatches one successor and logs whether that cycle
  was accepted. Replenish is unchanged (still refuses and stops at once).
- scripts/fuzz_ops.py verdict: "WAITING FOR WATCHDOG" inside the grace period
  after an unaccepted cycle; past it, BROKEN names the acceptance result and
  points at scheduled controller runs.
- Tests: scripts/test_fuzz_compute.py and test_fuzz_ops.py, 33 pass locally.

## Decisions and why
- Evidence: controller runs at 11:30, 13:39, 15:48 and 17:59 UTC all logged
  "The source cycle was not accepted; continuous discovery stops here"; the
  previous watchdog would have resumed none of them.
- The scheduled trigger works when GitHub fires it: run at 16:59 UTC logged
  "Watchdog: a dedicated cycle is queued or running; chain is alive". GitHub
  fired it once in the 9 h since the cron reached master (none at 17:07, 17:37,
  18:07); Q chose to keep the GitHub cron anyway.

## Surprises
- Scheduled runs are attributed to github-merge-queue[bot] (actor and
  triggering actor), the account that merged the cron change.

## Next step
PR, queue, CI. After merge: the first scheduled run following an unaccepted
cycle should log "Watchdog: resumed the chain after cycle N (not accepted)".
