# fix/toast-differential-probe-bound (issue #668)

## Goal

The nightly ToastDifferential comparison must survive an input the pinned
Toast never finishes. An empty named while loop (`n = 1; while o (n) endwhile`)
skips Toast's tick and seconds checks (diagnosis in the main checkout's
agent-notes/nightly-fuzz-assessment.md: src/execute.cc:2286-2288 and
2616-2623). With one Toast process per batch, the 900 s lane timeout raised
TimeoutExpired before any transcript was written and before Barn ran, so
results.json was missing and acceptance said "outcome missing".

## State

Implemented in the harness only (no Barn or Toast change):

- scripts/bench_differ.py: `lane_scripts()` builds the lane shell scripts
  (shared by `run_lanes()`), optionally wrapping the engine in
  `timeout -k KILL_GRACE <bound>`; `write_inputs()` writes input files.
- scripts/fuzz_differential.py: `run_bounded()` runs each engine in
  processes of the preamble plus at most CHUNK (20) probes, each bounded by
  STARTUP_SECONDS (30) + PROBE_SECONDS (10, `--probe-timeout`) per input.
  On exit 124/137 the probe after the last record is recorded as
  `TIMED_OUT` and the chunk reruns without it, replaying the probes before
  it. `engine_runner()` is the real process seam. Each process's transcript
  is appended to `<lane>_raw.txt`.
- results.json: `summary.timeouts = {"toast": [keys], "barn": [keys]}`,
  rows carry `toast_timed_out`/`barn_timed_out`; report.md has
  "## timed out on <lane>" sections.
- Tests in scripts/test_fuzz_differential.py, including one that kills a
  real stand-in engine that ignores SIGTERM through the lane script.

## Decisions and why

- The bound lives in the shell script, not in subprocess.run(timeout=):
  run_wsl is `bash -lc` on Linux (`wsl ... bash -lc` on Windows) and the
  engine is bash's child (the script's EXIT trap prevents exec), so
  Python killing bash would leave the engine holding stdout and
  communicate() would block.
- Attribution is sound because both engines flush each record before the
  next input runs: Toast fflush(stdout) in read_stdin_line (server.cc ~1001)
  and Barn writes -eval-file results to os.Stdout unbuffered.
- Replay keeps database state the same as one process for the probes after
  a stuck one. Different chunks start from fresh database copies on both
  engines alike.
- A timeout counts as an abort: timed out or aborted on both engines is
  "aborted on both" (may coexist with success, like before); one-sided is
  inconclusive (unchanged rule). fuzz_compute.cjs accept is unchanged.

## Surprises

- `python -m unittest discover -s scripts` on Windows fails in
  test_bench_players (no lambdamoo_db module), test_mongoose_barn_process
  and test_mongoose_error_capture; none import the changed files.

## Next step

Committed locally, not pushed (the coding agent was told not to push or
open the PR). After the PR ("Fixes #668") merges, check the next nightly
ToastDifferential: the named loop's key should be in
`summary.timeouts.toast`. Unverified: what Barn returns for that probe
(aborted gives "aborted on both"; a value would make it inconclusive).
