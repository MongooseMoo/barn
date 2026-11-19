# CI: cross-engine checkpoint restart job

## Goal

A Barn CI job that runs the checkpoint restart tests in both directions
(Toast writes then Barn resumes, Barn writes then Toast resumes) and fails when
either engine cannot resume what the other wrote. Tests: every
`tests/mongoose-conformance/server/*checkpoint*.yaml` in this repo plus the
harness's own kept-database restart tests. Runs on `pull_request` and
`merge_group`.

## State

Reading done; nothing built yet. Findings so far:

- `.github/workflows/ci.yml` job `conformance` builds `bin/barn`, checks out
  moo-conformance-tests at the tip recorded by the `tested` job (with
  submodules), and runs the managed harness with `uv run --project
  moo-conformance-tests --frozen moo-conformance`. No job in `ci.yml` runs a
  Toast binary; Toast appears there only as the oracle manifest.
- `.github/workflows/fuzz.yml` job `differential` is the only place this repo's
  CI gets a Toast binary: it reads `implementation_ref` and `config_checksum`
  from `profiles/toast/stock-wsl-testdb.json`, checks out
  `MongooseMoo/toaststunt` at that ref, verifies `options.h` against the
  checksum, and builds with cmake Release in `toaststunt/build`. Its comment:
  the runner user cannot read the workstation's `/root` Toast build.
- Harness (moo-conformance-tests `main`, which is that repo's default branch;
  there is no `master`): `--restart-server-command` starts a second engine on a
  `restart_server` step that keeps the database; `-m kept_db_restart` selects
  exactly the tests with such a step; a failed boot of the restart engine
  fails that test and the session continues. With
  `--oracle-profile-manifest`, the target manifest comes from the first
  engine's `{manifest}` file, so Toast first needs
  `--target-profile-manifest` (`plugin.py` `profile_metadata_gate`).
- The harness's `--candidate-root`/`--moo-suite-root` mode confines the
  database path to the candidate root, which is why the existing local
  regressions step copies `_db` to `.tmp/conformance/db`.

### Built so far

- `scripts/cross-engine-checkpoint.sh BARN TOAST SUITE`: four managed sessions
  (two directions, each over the suite's `kept_db_restart` tests and this
  repo's `server/*checkpoint*.yaml`), exit 1 when any session fails.
- `.github/workflows/ci.yml`: job `cross-engine` ("Cross-engine checkpoint
  restart"), added to the `record` job's `needs`. `actionlint` v1.7.12 on
  `ci.yml`: exit 0, no output.

### First run (one copy, Barn at the PR 899 merge, before merging master in)

Command, inside WSL Debian from the worktree, harness worktree at the PR 227
merge in `.tmp/moo-conformance-tests`, `UV_PROJECT_ENVIRONMENT` set to a
Linux-only directory under `~/.cache`:

```
bash scripts/cross-engine-checkpoint.sh .tmp/bin/barn-head /root/src/toaststunt/build-release/moo .tmp/moo-conformance-tests
```

Session summary lines (log `.tmp/results/head-1.log`, untracked):

- Barn writes, Toast resumes, suite tests: `1 failed, 57 passed, 11 skipped, 13917 deselected in 562.02s`
- Barn writes, Toast resumes, Barn checkpoint tests: `4 passed in 79.16s`
- Toast writes, Barn resumes, suite tests: `3 failed, 55 passed, 11 skipped, 13917 deselected in 539.13s`
- Toast writes, Barn resumes, Barn checkpoint tests: `4 passed in 83.20s`

### Real failures: suite tests that fail across engines on master Barn

Not skipped, weakened or excluded; the job is red on master until these are
fixed. Raw assertion lines:

Barn writes, Toast resumes:

1. `server/fork_try_computed_codes_checkpoint_restart.yaml::try_computed_codes_forks_run_after_checkpoint_restart`
   `Test 'step 'log = #0.computed_codes_fork_l...'' expected value [2, 1, 1, 0], but got [1, 1, 0, 0]`

Toast writes, Barn resumes:

2. `server/error_dump_persistence.yaml::all_named_errors_survive_dump_and_restart`
   `expected value [[3, ...], [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18], [...` `but got [[3, ...], [8030606863672279040, 1, 2, 3, 4, 5, 6, 7, 106734232272904, 9, 10, 11, 12, 13, 14, 15, 106734232272912, 17, 18], ['E_`
   (the integers read back for E_NONE, E_RECMOVE and E_FILE are wrong).
3. `server/map_dump_persistence.yaml::ordering_spelling_bytes_and_non_total_topology_round_trip`
   `expected value [1, ['kEY', 'second'], ['~C8', 'high'], [1 x12], [1 x8], [1 x6]], but got [0, ['kEY', 'second'], ['~C8', 'high'], [1 x12], [1 x8], [1 x6]]`
4. `server/restart_task_semantics.yaml::indefinitely_suspended_task_across_checkpoint_restart`
   `Test 'step 'still_queued = 0; for queued i...'' expected value [[], 1], but got [[0], 0]`

Causes not diagnosed. Whether each is Barn's export, Barn's import, or a test
that assumes one engine is unknown.

### Proof runs (after merging master in; logs in `.tmp/results`, untracked)

- Barn built from the commit before PR 886 (`pre886.log`): the job fails, and
  the failures include `suspended_eval_checkpoint_roundtrip`, the state PR 886
  fixed, plus the finally, move and range-bound checkpoint tests whose fixes
  came after it. The job would have caught 886.
- Two copies at once on master Barn (`master-a.log`, `master-b.log`): both
  finish, and both show exactly the four failures above and no others. The
  copies do not disturb each other.

### Master's required checks (read with `gh api`, 2026-11-18)

`Build & unit tests`, `MOO conformance suite`, `Serializable histories and
negative control`, `Checkpoint durability campaign`, `Checkpoint stage schedule
and negative controls`. The new job runs on `pull_request` and `merge_group`
but blocks a merge only once `Cross-engine checkpoint restart` is added to that
list. That is a repository setting; not done here.

## Decisions and why

1. **A job in `ci.yml`, not a new workflow.** `ci.yml` already has the
   triggers (`pull_request`, `merge_group`), the `tested`/`record` skip
   pattern and the harness checkout at the recorded suite commit.
2. **Its own job, not steps in `conformance`.** It needs a Toast build that
   the conformance job does not, and a separate job gives a separate check
   name to require and runs in parallel on another runner.
3. **Toast is built in the job** the way `fuzz.yml`'s `differential` job does
   (pinned ref and `options.h` checksum from
   `profiles/toast/stock-wsl-testdb.json`), because that job's comment says
   the runner user cannot read `/root/src/toaststunt`.
4. **The command lives in a script** so the same command runs locally against
   a Barn built from another commit, and twice at once.
5. **Toast command** is the Makefile's `conformance-toast` one:
   `moo {db} {db}.new -p {port}`. Barn command is the conformance job's,
   without `--log-dir`: the harness starts each server in its own temporary
   directory (`server.py` `cwd=self._temp_dir`), so Barn's default `logs`
   directory is already per server.
6. **Test selection.** Barn's tests by shell glob
   `tests/mongoose-conformance/server/*checkpoint*.yaml` turned into
   `--moo-suite-path` arguments (three files today, including
   `suspended_call_checkpoint.yaml`); the suite's by `-m "admission or
   kept_db_restart"`. Candidate-root mode and packaged mode use different
   suite roots, so they are separate sessions.
7. **No shared paths.** The database copy candidate mode needs goes to a
   `mktemp -d` directory under `.tmp/` in the checkout, removed on exit; ports
   are chosen by the harness.

## Surprises

- The brief says harness `origin/master`; the harness repo's branch is `main`.

## Next step

Count the packaged `kept_db_restart` tests, check the WSL tools, then write the
job and a script that holds its command.
