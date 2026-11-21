# fuzz-toast-loadable-lane (issue #1033, lane half)

## Goal

Each fuzz cycle, the generated cross-engine lane (`scripts/fuzz_cross_engine.py`,
job ToastCrossEngineCheckpoint) also runs a converted Barn-first session: Barn
writes the checkpoint, `barn offline-checkpoint --toast-loadable` converts it,
Toast resumes. Every task the conversion omits is counted and reported by
reason; a Toast load failure of a converted copy is a finding.

## State

- Built and committed locally on `fuzz/toast-loadable-lane` (recreated on
  origin/master after PRs 1032 and 1036 merged; not pushed):
  - `scripts/cross-engine-session.sh`: direction `barn-first-toast-loadable`,
    restart command `bash scripts/toast-loadable-restart.sh BARN TOAST {db} {port}`.
  - `scripts/toast-loadable-restart.sh`: with `TOAST_LOADABLE_LOG` set, the
    conversion output is also appended there.
  - `scripts/fuzz_cross_engine.py`: after each chunk's barn-first session, the
    programs whose restart failed with `RestartCommandError` (Toast did not
    boot) run again in the converted direction with `converted_case` (steps 1-4,
    the restart, step 8 `valid(#0.cross_engine)`). Rows join the report's
    divergence list (class `barn-first-toast-loadable: ...`); `## converted
    copies` gives programs, conversions, omitted tasks and counts per reason;
    `results.json` summary has `converted`. A converted session whose booted
    copies outnumber the logged conversions is a problem (inconclusive).
  - Unit tests in `scripts/test_fuzz_cross_engine.py` (`ConvertedTests`, 5) on
    real fixtures `scripts/fixtures/fuzz/cross-engine/master-barn-first.xml`,
    `master-barn-first-toast-loadable.xml`,
    `master-barn-first-toast-loadable-conversions.txt`; README updated.
  - `docs/going-back-to-toast.md`: final-checkpoint paragraph (from the lead's
    reading of server.go, checked against `server/server.go`
    `shutdownWithCheckpoint`, `checkpointWith`, `dumpCheckpoint`,
    `server/signals_unix.go`, `internal/app/app.go` default 3600) and the lane line.
- `fuzz.yml` not changed: the job already calls the script; the converted
  session runs inside it. So actionlint/zizmor were not needed.

## Decisions and why

- Converted session only for programs Toast did not boot on: those are the
  checkpoints holding a Barn-form task; the others load already, and the 14
  directed restart tests already showed the conversion keeps loadable tasks.
  It costs one extra session only in chunks that have such programs.
- Its test stops at "Toast boots and the world is there", not "B equals A":
  omitted tasks never finish, so B always differs.
- Omissions are counted per session from the wrapper's output (env var, since
  the harness passes its environment to restart commands and has no per-test
  hook); reasons are grouped with the `frame N (#o:verb): ` prefix removed.
  Per-program attribution is not made.
- The conversion log fixture is `.txt`: `test_fuzz_report` requires every
  `*.log` fixture to be a GitHub job log or a native tee log.

## Surprises

- First lane run: the converted session's admission failed, `tee:
  .tmp/tl-lane-1/barn-first-toast-loadable-001.log: No such file or directory`:
  the restart command runs in the server's directory, so the relative `--out`
  path broke the wrapper. The log path is now resolved to absolute.
- The admission test's own restart also goes through the wrapper (one
  conversion with nothing omitted), so conversions = programs + 1.
- The reason for the `call_function("create")` task now reads
  `call_function: stock Toast does not resume ...` (master after PR 1035 names
  the builtin frame `call_function`), not `create: ...` as on the first branch.

## Runs

- Python units: `python -m unittest test_fuzz_cross_engine test_fuzz_report
  test_fuzz_compute` (scripts/): 84 OK; `test_fuzz_differential` also ran OK
  in an earlier combined run (117 tests, the one failure there was the `.log`
  fixture name, fixed by renaming).
- `bash -n` on both shell scripts: OK.
- Lane, local WSL (load 11.75/11.66/7.23), Linux barn from this branch
  (`CGO_ENABLED=0 GOOS=linux GOAMD64=v3`), stock Toast `61dd1799...13dd`, suite
  detached worktree at moo-conformance-tests origin/main:
  `python3 scripts/fuzz_cross_engine.py --barn bin/barn-linux-amd64 --toast
  /root/src/toaststunt/build-release/moo --suite <mct> --out .tmp/tl-lane-1
  --seed local .tmp/tl-corpus scripts/fixtures/fuzz/FuzzSuspendedStackCheckpoint`
  (`.tmp/tl-corpus` = the two finding records). EXIT 1 (the four known
  divergences: barn-first and toast-first boot failures for both finding
  programs, #1033/#981). Converted rows: both `match`. Report:
  ```
  ## converted copies
  - programs whose Barn checkpoint Toast did not boot on: 2
  - conversions: 3; tasks omitted: 2
  - omitted 1: call_function: stock Toast does not resume a create, recreate or recycle continuation from its checkpoint
  - omitted 1: recreate: stock Toast does not resume a create, recreate or recycle continuation from its checkpoint
  ```
- Two copies at once (`--out .tmp/tl-copy-a`, `.tmp/tl-copy-b`, seeds
  `copy-a`/`copy-b`; load 26.03 before, 25.89 after): both EXIT 1 with the same
  four divergences, both converted rows `match`, identical `## converted
  copies` sections, `problems: []`.

## Not verified

- The lane on a GitHub runner (the converted session inside
  ToastCrossEngineCheckpoint, its artifact, the reporter on a cycle with only
  converted divergences).
- A converted copy that Toast refuses: no real producer yet; the class string
  is unit-tested on a synthetic message only.
- Resuming converted copies on the Mongoose 0ff6fd7 build (first branch's
  note: profile gate refuses the pairing).
- `TestShutdownFinalCheckpointRunsHooksBeforeRuntimeStops` was read, not run.

## PR body draft

Title: Convert Barn checkpoints Toast cannot boot on in the cross-engine lane

Refs #1033.

When Toast does not boot on a checkpoint Barn wrote for a generated
suspended-stack program, the ToastCrossEngineCheckpoint lane now runs that
program again with the checkpoint converted by `barn offline-checkpoint
--toast-loadable` (PR 1036) before Toast starts, and requires Toast to boot on
the copy. The report counts the tasks the conversion left out, by reason, under
`## converted copies`; a converted copy Toast does not boot on is a divergence
of class `barn-first-toast-loadable: ...`.

- `scripts/cross-engine-session.sh`: direction `barn-first-toast-loadable`.
- `scripts/toast-loadable-restart.sh`: `TOAST_LOADABLE_LOG` collects the
  conversion output.
- `scripts/fuzz_cross_engine.py`: the converted session, its test shape (up to
  the restart, then a check that the world is there), omission counts.
- `scripts/test_fuzz_cross_engine.py`: five tests on real harness output.
- `docs/going-back-to-toast.md`: when Barn writes its final checkpoint on
  shutdown and how to check it in the log; the lane.

## Agent's runs

Local WSL, stock Toast, the two programs of PR 1032's findings 2 and 3 plus two
that pass: Toast booted on both converted copies; one task omitted for each
(`recreate: ...` and `call_function: stock Toast does not resume a create,
recreate or recycle continuation from its checkpoint`). Two copies at once with
separate `--out`: same results. Unit tests: `test_fuzz_cross_engine`,
`test_fuzz_report`, `test_fuzz_compute` 84 OK.

## Not verified

The lane on a GitHub runner. `fuzz.yml` is unchanged.

## Next step

- Lead: push `fuzz/toast-loadable-lane`, open the PR from the draft above.
