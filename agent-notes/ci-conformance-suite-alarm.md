# Alarm on conformance suite test count and run time

## Goal

Q 2026-11-11: "how long did the conformance tests take to run, what is their
average time and number of tests, then if the average goes +- ... 10% in a
week, freak out?" Build exactly that. Part of making the fuzz-to-merge machine
run unattended (plan and evidence: agent-notes/automatic-machine.md in the
main checkout).

## State

- scripts/conformance_suite_alarm.py with 13 unit tests, and
  .github/workflows/conformance-suite-alarm.yml (runs on each push to master
  and on manual dispatch). actionlint passes.
- Run read-only against real history on 2026-11-11: newest week of merge
  queue runs 612-624 s, the week before 602-619 s, test counts 13792 and
  13792; no alarm.

## Decisions and why

- It only reports. An alarm opens one issue labelled conformance-suite-alarm
  and opens no second one while that is open. Q: "I do not want the machine to
  shuit itself down!" Nothing is stopped, gated or switched off, and the
  workflow run stays green.
- Samples are merge queue runs of ci.yml (the trees that land on master), up
  to 12 spread through each week; a run whose suite step was skipped because
  the same tree had already passed is left out. A week with fewer than five
  samples gives no run time verdict.
- Test count compares the two newest sampled runs, read from the
  conformance-results artifact; a drop alarms, a rise does not.
- Triggered by pushes to master, not a cron schedule:
  fuzz-controller.yml records that GitHub rarely runs this repository's
  schedules.
- What it can see: measured on six CI result files, about 95% of the suite's
  time is fixed waits in the tests, so a 10% move means a hang, a new timeout,
  or ordinary commands several times slower. It cannot see a 20-50% engine
  slowdown or anything about many players at once. When the fixed waits are
  cut from the harness (moo-conformance-tests branch perf/harness-fixed-waits)
  the average will drop once and alarm once; after that it becomes more
  sensitive.

## Surprises

- The runners' gh is 2.23 (2023), so the script uses only `gh api`.

## Next step

Merge. After the first master push, read the workflow run's output to confirm
it reads history with the workflow token.
