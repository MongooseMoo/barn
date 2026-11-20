# Goodput and latency series on master

## Goal

Q 2026-11-11, on making the fuzz -> fix -> merge chain run with nobody
reviewing: "we can't have commits that dramatically decrease goodput or
individual player experience". Nothing in CI measured either. This branch
adds the measurement and an alarm. The overall plan is item 7 in
agent-notes/automatic-machine.md in the main checkout.

## State

- .github/workflows/goodput-series.yml: every hour, on a CI runner, when the
  1-minute load average is below 8: five runs of TestMongooseRealWorkload at
  1 and 16 players (warmup 5 s, measure 30 s, GOGC 400, the settings session
  barn-d6 uses), medians kept as artifact goodput-point. Then
  scripts/goodput_series.py compares the newest points with the ones before
  them and opens one issue labelled goodput-alarm on a lasting change for the
  worse. It stops nothing.
- scripts/goodput_series.py + test: parses the harness's result line, makes
  the point, finds a change (newest 3-10 points vs the 8-20 before, at least
  10% worse, one-sided rank test below 1 in 100).
- Fixture: C:\Users\Q\projects\mongoose\db\mongoose.db.new.2026-11-12T0055
  (432344132 bytes) and sound.sqlite.2026-11-09T2239, copied there on
  2026-11-12 from barn-d6's session scratchpad and the main checkout's
  files/sqlite; the workflow checks both checksums. The runners reach them
  under /mnt/c.
- Labels goodput-alarm, conformance-suite-alarm and needs-decision created
  in the repository on 2026-11-12 (the last two are used by workflows
  already on master and did not exist).

## Decisions and why

- A point is taken on each push to master, not only hourly (added
  2026-11-12 on branch ci/goodput-on-merge). The hourly schedule started no
  run in its first two slots (05:17 and 06:17 UTC), and GitHub's
  documentation for the schedule event says runs "can be delayed during
  periods of high loads" and "some queued jobs may be dropped"; the fuzz
  controller's `7,37 * * * *` schedule started six runs in 32 hours here.
  A point per merge also names the commit a change arrived with.

- No verdict before a merge. Two 16-player runs of one commit gave 892 and
  707 commands/s on this dump (barn-d6, 2026-11-12), and Daly et al. 2020
  found per-run thresholds useless at MongoDB (papers/Daly_2020_...). The
  alarm comes after the merge, once three points show the change.
- The harness's output is never printed or uploaded: the dump holds real
  players' data and this repository's Actions logs are public. Only the
  result line's numbers leave the runner. A failed run therefore says only
  that it failed.
- A new dump makes a new series: points carry the fixture's checksum and
  only points on the newest fixture are compared (barn-d6: per-command
  allocations rose from about 34k to 43-45k between two dumps).
- The pull_request trigger on the two files exists so a change to the
  measurement runs once before it merges; such a point is named apart and is
  not in the series.

## Surprises

- First Linux run (this pull request's own, master before #783, load 9-13
  while the pull request's CI ran): about 105 s per run; 1 player 19-22/s,
  p50 38-43 ms; 16 players 157-225/s. barn-d6 then measured Windows on the
  same dump and settings: 1 player 21/s, p50 36.8 ms. So the low 1-player
  figure belongs to this dump (barn-d6 sees about 260k allocations per
  command at 1 player against 43-45k at 16, and suspects the world's own
  restored tasks; not diagnosed), not to Linux.
- Four of five 16-player runs logged `uncaught=1`. barn-d6 finds the same
  single error in every 16-player run on this dump on Windows: E_MAXREC in a
  forked weather task restored from the checkpoint, not a measured command.
  The first version of the script refused a point for any uncaught error and
  so refused every point; it now records the count and refuses only failed
  commands. Whether Toast raises that E_MAXREC is unchecked.

## Unverified

- Memory use of the harness on the runners.
- The change limits (10%, 1 in 100, 3 points): chosen before any real series
  existed. Revisit once about 30 points exist.
- Hourly points during busy hours: the idle limit may skip most of them.

## Next step

Read this pull request's own run: does the harness pass on Linux, how long
and how much memory. After merge, read the first scheduled runs and the
first 10 points' spread.
