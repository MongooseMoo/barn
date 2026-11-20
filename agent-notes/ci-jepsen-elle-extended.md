# ci/jepsen-elle-extended

## Goal

Q asked whether Jepsen and Elle run in an extended or continuous mode. They did
not: only push, pull_request, merge_group and workflow_dispatch, 20-25 minute
job timeouts, Jepsen at `--rate 30 --dwell 1`. Q ordered an extended mode.

## State

`.github/workflows/jepsen.yml` and `.github/workflows/elle.yml` gain an
extended mode, selected by `github.event_name == 'schedule' || inputs.extended`.
Same jobs, same steps, same required check names; only the run parameters
change. PR, push and merge-group behaviour is unchanged: a script that ran
each changed run step with stubbed `uv`/`python3`/`go` printed identical argv
for the master and branch versions in PR mode. actionlint v1.7.7 reports
nothing (shellcheck was not installed, so shell bodies were not linted by it).

- Elle: nightly `17 1 * * *` UTC. 50 healthy seeds from
  `GITHUB_RUN_NUMBER * 100` onward, 1000 rounds each, fault cases unchanged.
  Dispatch: `extended` boolean; blank `seeds`/`rounds` take the mode default.
  Job timeout 180 minutes (25 in PR mode).
- Jepsen: nightly `37 4 * * *` UTC. Campaign: three seeds from
  `GITHUB_RUN_NUMBER * 10`, phases 300/450/300/300 s (five times PR), rate 60,
  concurrency 16, dwell 4, all six families, task lane, all seven transport
  kinds. Stage schedule: five seeds, all six families, stage-seconds 60, same
  rate/concurrency/dwell. Controls unchanged. Job timeouts 180 minutes.

## Decisions and why

- Concurrency group gets an `-extended` suffix in extended mode: the old group
  `jepsen-/elle-${ref}` with cancel-in-progress would let a master push cancel
  the nightly run (both have ref master), or the reverse.
- The tested-tree check is skipped for schedule (as for dispatch) so the
  nightly run always runs; the record jobs do not run in extended mode because
  extended seeds differ from the PR configuration the record stands for.
- Seeds come from the run number so each night covers new seeds; the step
  log and each results.json name them.
- Failure signal: no failure reporting existed in either workflow; the
  scheduled run's red status is the signal. GitHub's documentation says
  scheduled-workflow notifications go to the user who last changed the cron
  line (not re-checked against the docs in this task).

## Duration evidence

- Elle dispatch run 42354689760 (rounds 200): about 25 s per seed; PR rounds
  40: about 10 s per seed; fault block about 3 minutes. Estimate for 1000
  rounds: about 100 s per seed, about 90 minutes total. check_elle.py's
  per-seed `go test -timeout=5m` is the hard limit.
- Jepsen PR run 43126491899: campaign main run 6.0 minutes for 270 s of
  phases; stage schedule 2.3 minutes at stage-seconds 10. Estimates: about
  30 minutes per campaign seed (about 95 minutes), about 15 minutes per stage
  seed (about 75 minutes plus 7 minutes of controls).

## Unverified

None of the extended parameters has run. Unknown until the first nightly or
extended dispatch: Elle check time on 1000-round histories, Jepsen analysis
time on longer histories at rate 60, whether the stage schedule is healthy
with the effects/authority/topology families (PR runs it with
smoke,props,keyed only), and whether transport recovery stays inside
RECOVERY_BOUND (10 s) at rate 60 with dwell 4.

## Next step

Push, open the PR, then dispatch both workflows with `extended: true` on the
branch to measure the real duration before the first nightly run.
