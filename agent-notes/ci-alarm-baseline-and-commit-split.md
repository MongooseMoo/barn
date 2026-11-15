# Alarm fixes: commit-split rule, per-run idle gate, restart after a closed alarm

## Goal
Q 2026-11-15: "What's the current state of the machine? ... several alarms
firing. What happened, what should have happened, why" then "Make it so!".

## Problems (stated before the fix)
- #829 (goodput alarm, opened 2026-11-14 07:42): "p99 at 16 players +25%,
  change in cf2f49f483e4...cf2f49f483e4". scripts/goodput_series.py
  `change()` accepted a split between two points measured on the same commit
  (hourly scheduled re-runs), so a machine-side shift was reported as a code
  change. The newest point (ce23a721f) was measured after one idle check at
  load 6.44; its runs then ended at load 9.5-14.4 because the workflow checked
  the load once, before ~10 minutes of runs. By 14:53 the same day p99 was
  99.4 ms again; series since then 93-137 ms, same as before the alarm.
- #828 (conformance suite alarm, opened 2026-11-14 07:38): run time -12.4% in
  a week. Real and intended: suite step 694 s at 2026-11-12 01:59, 411 s after
  moo-conformance-tests #193 (02:23), ~320 s after #196 (03:16). But the issue
  says "close when explained" and a close would be reopened by the next master
  push, because the two weekly windows straddle 2026-11-12 until ~11-26.

## Fix (branch ci/alarm-baseline-and-commit-split)
- goodput_series.py: a split counts only where the commit differs on its two
  sides; points measured before the newest closed goodput-alarm issue was
  closed are left out.
- conformance_suite_alarm.py: `last_closed()` (shared with goodput), and both
  weeks start no earlier than the newest close; for a week after a close there
  is no run-time verdict, the test count is still checked.
- goodput-series.yml: the idle check runs before every harness run; a point is
  made and uploaded only when all runs completed (`steps.harness.outputs.complete`).

## Evidence
- 20 + 16 unit tests pass (py -3.13). Read-only replay on real data
  2026-11-15 ~21:00 UTC: goodput finds no change over 39 points; suite alarm
  still reports the 11-12 step (-20.1%), as expected while #828 is open.

## Next step
- After merge: comment the explanations on #828 and #829, close both (Q's
  permission prompt). Then read the next master run of each alarm workflow:
  it must print "comparing only ... after the last alarm issue was closed".
