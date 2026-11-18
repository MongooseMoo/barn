# CI: native checkpoint restoration census

## Goal

Run `TestToastNativeCheckpointRestorationCensus` (`engine/toast_suspended_import_test.go`)
automatically in CI against the private Mongoose checkpoint the runners already
hold, on the same triggers as the goodput workflow, as its own job. The job fails
when any task is not restored. Its public output is only the summary line and a
count per distinct Barn refusal reason. Nothing from the database may reach a
log, artifact, issue or commit.

## State

Built and committed locally on `ci/native-checkpoint-census`; not pushed, no pull
request, never run on GitHub.

- `.github/workflows/native-checkpoint-census.yml`: the workflow (push to
  `master`, hourly schedule, manual dispatch, pull requests that touch it, its
  script, the census test or `goodput-series.yml`).
- `scripts/native-checkpoint-census.sh`: runs the census and prints the safe
  summary.
- No Go file and no test expectation changed.

### Census on the pinned fixture (2026-11-18)

Windows workstation, CPU load 42% before the run (not a timing measurement).
Fixture: the file `goodput-series.yml` names in `FIXTURE`; its SHA-256 computed
locally equals that workflow's `FIXTURE_SHA256`.

`BARN_MONGOOSE_DB=<fixture> go test ./engine -run '^TestToastNativeCheckpointRestorationCensus$' -count=1 -v`, exit 0:

- `native census coverage: queued=93 continuations=12 frames=17`
- `checkpoint restored: queued=93/93 suspended/interrupted=12/12`
- `--- PASS: TestToastNativeCheckpointRestorationCensus (9.89s)`
- Refusal reasons: none. The fixture is Toast-written (native continuations and
  native frames are present, so the coverage check passes).

### Two copies at once (CLAUDE.md's rule for a new CI step)

In the WSL Debian distro that hosts the runners, load average 0.23 before: two
copies of `scripts/native-checkpoint-census.sh` started together, each with its
own directory and its own checksum-verified copy of the fixture, using a Linux
build of the engine test binary. Both printed the two summary lines above and
`--- PASS`, both exited 0. Each directory held only `mongoose.db`,
`census-output.txt`, `reasons.txt` and `refusals.txt`; `git status` in the
worktree showed no new file. The command takes no machine-wide lock and writes
only inside the directory it is given.

### Failing paths

No local dump makes the census fail, so these used a stand-in test binary that
prints refusal records through the real default `slog` handler and `testing`'s
line formats, with every stand-in for database text marked:

- Refusals: exit 1; printed the coverage line, the summary line, `--- FAIL`,
  `FAIL`, one `N not restored: <reason>` line per fixed reason, one line counting
  refusals whose reason is not a fixed string, and the closing sentence. No
  marked text was printed (19 marked lines stayed in the private output file).
- The same followed by a Go panic: exit 1; `panic:` alone was printed from the
  panic. No marked text.
- A skipped test (no database set): exit 1, "census printed no summary".
- The real census on `engine/testdata/toast-suspended-call.db` (no queued tasks):
  exit 1, printed the "native census coverage requires ..." line with its counts.

## Decisions and why

- A separate workflow file, not a job inside `goodput-series.yml`.
  `scripts/goodput_series.py` `past_points` reads only runs of that workflow with
  `status=success`, so a failing census job in the same workflow would drop that
  run's goodput point from the series. The goodput workflow's concurrency group
  would also make the census wait behind a 90-minute measurement.
- No issue-opening step. The census passes today; the brief asks for the alarm
  mechanism only if it fails today. A failing run is a red workflow run.
- The job copies the fixture into the runner's own `$RUNNER_TEMP` and checks the
  copy's checksum, as `goodput-series.yml` does. `dbformat.LoadDatabase`
  (`db/format/reader.go:150`) first completes or rolls back an interrupted
  checkpoint publication at the path it is given
  (`recoverCheckpointPair`, `reader.go:167`), so pointing it at the shared file
  under `/mnt/c` could write beside a file all eight runners read. Its lock
  (`db/format/checkpoint_lock.go:22`) is in-process only.
- The pin (`FIXTURE`, `FIXTURE_SHA256`) is read from `goodput-series.yml` at run
  time, not copied into the new workflow, so the census always audits the dump
  the goodput harness measures and a pin bump needs one edit.
- The filter is a script file, so the two-copies-at-once proof and the leak check
  ran the same bytes CI runs.
- Refusal reasons are printed only when they are a string literal in this
  repository's Go source (`fmt.Errorf("...")` / `errors.New("...")` without a
  format verb), collected with `grep` at run time. Several restore errors wrap
  database text (`compiler/native_import.go:39` wraps a compile diagnostic,
  `vm/persistence.go:309` formats an object number and verb name), so the tail of
  an error is not safe to print. A string already in the public source discloses
  nothing. A refusal with no such string is counted under a fixed label. The
  reasons are counted on the one log record `engine/task_load.go` writes per
  refused task (lines 23, 44, 54), because the test's own "was not restored"
  lines carry no reason.

### Why nothing from the database can reach the public log

- The test binary's stdout and stderr both go to one file in `$RUNNER_TEMP`
  (`scripts/native-checkpoint-census.sh`, the redirect on the test command).
  That covers the test's log lines, the runtime's warnings while loading, a
  panic's stack, and the timeout panic from `-test.timeout 10m`.
- The script prints from that file in two places only. The first is `grep -o`
  with patterns that are fixed words plus `[0-9]+`. The second is the `awk`
  program, which prints a count and either a line of `reasons.txt` (built only
  from this repository's `.go` files) or a fixed label; it never prints its
  input line.
- If the job is cancelled or hits `timeout-minutes`, the script has printed
  nothing from the file yet or only the lines above.
- The workflow uploads no artifact, opens no issue and has `contents: read`
  only. The last step removes the directory with `if: always()`; a self-hosted
  runner's `$RUNNER_TEMP` is on the workstation, not public, if that step is
  ever skipped.
- The build step compiles public source only, before the database is read by
  anything but `cp` and `sha256sum` (which prints the path already written in
  `goodput-series.yml` on a mismatch).

## Surprises

- The census passes on the pinned CI fixture both on current `master` and on the
  commit just before PR 886 (same counts, 93/93 and 12/12). This fixture holds no
  continuation of the kind PR 886 fixed, so this job on this fixture would not
  have caught that bug. Two newer local dumps also pass on current `master`
  (queued 1854/1854 and 1801/1801, suspended 6/6 and 5/5). Which private
  database gave the 22/30 result is not known to me. The newest local dump could
  not be opened ("Device or resource busy").
- `actionlint` is not installed on the workstation, so the workflow was not
  linted. It parses as YAML (PyYAML), and the pin extraction prints the expected
  path and checksum from `goodput-series.yml`.
- A cross-build of the engine test binary for Linux from Windows needs
  `CGO_ENABLED=0`; with the workstation's default it fails in `runtime/cgo`.

## Unverified

- The workflow has never run on GitHub: triggers, the `setup-go` step, runner
  selection and `$RUNNER_TEMP` behaviour are as written, untested.
- Real refusals from a real checkpoint were never seen; the reason counting was
  exercised only on the stand-in output described above.
- Whether an `if: always()` step runs after a `timeout-minutes` kill on these
  runners was not tested.
- The hourly schedule is copied from the goodput workflow as the brief asked;
  the census is deterministic per commit and fixture, so the scheduled runs
  repeat the same answer between merges.

## Next step

Push the branch and open the pull request (the workflow runs on it because the
pull request touches the workflow file), then read the run's log to confirm it
shows only the summary lines. To make the census able to catch the PR 886 class,
the pinned dump would have to be one that holds such continuations; that is a
change to the pin in `goodput-series.yml` and is not part of this branch.
