# ToastDifferential: kill a stalled engine after PROBE_SECONDS

## Goal

A probe that hangs an engine in `scripts/fuzz_differential.py` should cost about
PROBE_SECONDS, not the whole chunk bound (STARTUP_SECONDS + PROBE_SECONDS * (1 + n)).

## State

- `scripts/fuzz_differential.py`: `run_streaming` reads the lane script's stdout
  line by line; once no result record (a `RECORD_START` line that is not
  `NOT_A_RESULT`, as `results()` counts them) has arrived for PROBE_SECONDS
  (STARTUP_SECONDS before the first), it runs a kill command and collects the
  output. `engine_runner` prefixes the lane script with `echo $$ > <lane>_process.pid`
  and kills with `for c in $(pgrep -P <shell pid>); do kill -KILL -- "-$c"; done`
  (timeout's process group). The script exits 137, so `run_bounded` is unchanged:
  the probe after the last record is TIMED_OUT and the chunk replays without it.
  The coreutils whole-process bound and the harness deadline stay as outer guards.
  `bench_differ.py` is unchanged; the kill command runs through `run_wsl`, so it
  works on Linux and through WSL on Windows.
- Test `test_real_shell_kills_a_stalled_engine_after_probe_timeout`: bound 34 s,
  stall limit 1 s. Passes on Windows/WSL and on Linux; with the stall kill off the
  same test takes 42.3 s and fails. Locally each `bash -lc` costs 1.9 s (root's
  login profile in Debian), so the test takes about 7.6 s; the runners' normal
  process took 0.15 s in the artifact.

## Decisions and why

- Pid file instead of `Popen.pid`: on Windows `Popen.pid` is wsl.exe, not the
  Linux shell, so the shell writes its own pid where both platforms can read it.
- Kill timeout's group, not the shell's: the shell must live to run its EXIT trap
  (`rm -rf` of the private directory holding the db copy and, for Barn, the binary).

## Risk

- The stall limit is per probe in wall time (10 s against FG_SECONDS 2). Under
  heavy runner contention a slow but finishing probe could be killed and recorded
  TIMED_OUT, which makes the run inconclusive if the other engine completed it.
  Before, the 240 s chunk bound absorbed that. No such case is observed yet.

## Findings

- Run 43291034257, job 131501889727, artifact fuzz-failure-ToastDifferential,
  `toast_raw.txt`: 123 Toast processes, 113 exit 0 and 10 exit 137. Toast's
  stderr `STARTING` timestamps give the wall time between process starts: the
  9 hung processes that have a successor took 2195 s (about 244 s each, the 240 s
  bound plus kill), the 113 normal processes took 17 s in all (about 0.15 s each).
  The lane reached input 2300 of 5550 (`toast_in.txt` has 5551 lines with the
  preamble); the Barn lane never started.
- Records reach the pipe as each probe finishes, on both engines, with no
  `stdbuf` or pty needed:
  - Toast (`/root/src/toaststunt` at aecc51e): stdout is block-buffered on a pipe,
    but `read_stdin_line` calls `fflush(stdout)` (server.cc:1001) before every
    input read, and the emergency loop reads each input through it
    (server.cc:1070). So the `printf("=> %s\n", ...)` of a probe's result is
    flushed when Toast goes to read the next input, which happens right after the probe.
  - Barn: `cmd/barn/main.go:107` passes `os.Stdout` (an unbuffered `*os.File`)
    to `app.Run`, and `dbtool.EvalFile` (internal/dbtool/dbtool.go:392) writes
    each record to it; there is no `bufio` in `internal/dbtool`.
  - `bench_differ.lane_scripts` puts nothing between the engine and the pipe
    (no `tee`/`stdbuf`; `timeout` does not touch stdout).
  - Observed: the SIGKILLed Toast process for inputs 21..40 in the artifact still
    shows the preamble and seven records. Buffered stdio data would be lost on
    SIGKILL, so those records were already in the pipe.
  - The new test passes through WSL on Windows too, so wsl.exe relays lines as
    they come.
- The kill: `timeout` (without `--foreground`) calls `setpgid(0, 0)`, so it and the
  engine form their own process group under the lane's bash. Killing bash's group
  or bash alone misses them (the job log's orphaned timeout/moo at cleanup).
  Killing timeout's group with SIGKILL takes timeout and the engine; bash then
  exits 137 under `set -e` and its EXIT trap removes the private directory.

## Estimate for the 45-minute job

- Toast, 5550 inputs: about 280 chunk processes plus one replay per hang at about
  0.15 s each is about 45 s. 10 hangs in the first 2300 inputs extrapolates to
  about 24 over 5550; at about PROBE_SECONDS (10 s) each that is about 240 s.
  Toast lane: about 5 minutes (was 40+ minutes for 2300 inputs).
- Barn: no Barn timing exists in this run (the lane never ran). Barn aborts the
  #668 programs on ticks, so no hang cost is expected; process startup copies the
  binary and loads the db, unmeasured here. Even at 1 s per process, 280
  processes is under 5 minutes.
- Both lanes then fit well inside 45 minutes; the timeout is not changed.

