# ci/fuzz-keep-going

## Goal

Plan item D of `agent-notes/fuzz-pipeline-expansion.md` (Q 2026-11-06: "it
shouldn't just stop after the first one right?"):

1. Within the existing 30-minute budget per target, start the campaign again
   after each failure until the budget is used, keeping every failing input and
   every failing run's log.
2. Report every distinct signature of the night through #612's reporter: one
   issue (and one claude-fix dispatch) per new signature, a comment for a
   signature that already has an open issue, the count of refinds in the body.
3. claude-fix prompt: before fixing, check open "Nightly fuzz failure" issues and
   open claude/issue-* PRs for the same root cause; if found, open no PR and use
   the one allowed issue comment to link it with the evidence.

## State

Read: the job brief, common rules, the approved plan,
`.github/workflows/fuzz.yml`, `.github/workflows/claude-fix.yml`,
`scripts/fuzz_report.py`, `scripts/test_fuzz_report.py`, `AGENTS.md`, and the
Go 1.27.1 sources below. Implementing.

## Findings (Go 1.27.1, the go.mod toolchain)

- `src/internal/fuzz/fuzz.go:249-275`: the first crasher disables the other
  workers, is minimized, written, and the coordinator stops. No keep-going flag.
- `src/internal/fuzz/fuzz.go:1051-1061` `writeToCorpus`: the file name is the
  first 16 hex digits of the SHA-256 of the input, written with
  `os.WriteFile`. Same name means same content, so a later run that saves the
  same name rewrites identical bytes; different inputs get different names.
  Go never deletes corpus files.
- `src/testing/fuzz.go:257-262`: everything in `testdata/fuzz/<Target>` is read
  as seed corpus. `src/internal/fuzz/fuzz.go:250-254`: a failing seed during
  warmup prints "failure while testing seed corpus entry" and stops without
  writing an input. So an input saved by run N, left in testdata, would stop
  run N+1 at once. The loop sets each saved input aside before the next run
  and puts all of them back when the step ends (trap on EXIT), so the existing
  artifact path and the reporter's input lookup are unchanged.

## Decisions and why

- Loop continues only when `fuzz_report.py` classifies the failed run as not
  infrastructure and names a saved input. `fuzz_report.py:34` and `:44-45`
  classify infrastructure only when no input was saved; checking both follows
  the brief literally at no cost.
- Per-run bounds: `-fuzztime=<remaining>s -timeout=<remaining+600>s`, stop when
  fewer than 60 s remain. The last run ends at most 30 min + 10 min after the
  loop starts, the same bound as today's `-fuzztime=30m -timeout=40m`, inside
  the unchanged `timeout-minutes: 45`.
- Only failing runs' logs are kept as files (`run-NNN.log`, zero-padded so a
  plain sort is run order); every run's output also stays in the job log via
  `tee`. The reporter takes `--log` repeatedly and returns a list, one entry per
  distinct signature in first-seen order, with `count`.
- Artifact unchanged: still the target's testdata directory, now holding every
  input saved in the night.

- claude-fix prompt gets a new step 2 (same-root-cause check against open
  "Nightly fuzz failure" issues and open claude/issue-* PRs); step 1 now says to
  use only the issue's own input, since the artifact can hold several.

## Verification (local, WSL Debian)

- `python3 -m unittest discover -s scripts -p "test_*.py"` with a runner's
  bundled node on PATH: 259 tests OK (WSL root has no node on PATH; the test
  falls back to `$RUNNER_TEMP`, which only exists in Actions).
- `actionlint` v1.7.12 (shellcheck present) on fuzz.yml and claude-fix.yml:
  exit 0, no findings.
- The campaign step itself, extracted from fuzz.yml and run under `bash -e`
  with a fake `go` (scratchpad driver `keep-going/drive_loop.py`; the fake
  fails with "failure while testing seed corpus entry" if any non-committed
  input is still in testdata at the start of a run):
  - panic, hang, same panic input again, panic with another input, pass:
    5 runs, step exit 1, testdata ends with aaaa, bbbb, cccc plus the committed
    file, 4 logs kept, reporter gives panic x3 and hang x1.
  - infrastructure on run 1: 1 run, step exit 1, nothing saved.
  - panic then infrastructure: 2 runs, loop stopped, reports panic x1 and
    infrastructure x1.
  - pass: 1 run, step exit 0, no logs kept.
- Not exercised: the stop when fewer than 60 s remain and the shrinking
  `-fuzztime` (the fake runs instantly, so every run got 1800 s); a real
  nightly run on the self-hosted runners.

## Surprises

- 2026-11-06: the PreToolUse Bash hook refused the combined
  `git push` + `gh pr create` call: "The command is a GitHub write (gh pr
  create), and Q's latest words do not clearly approve it: after the agent asked
  whether to merge the fuzz-plan PRs, Q replied "uh probably don't do that sort
  of cowboy shit", then "figure what the actual problem is", then "those should
  codex agents"". Nothing was pushed. Work is committed locally on
  ci/fuzz-keep-going; PR body drafted in the session scratchpad
  `keep-going/pr-body.md`.

## Next step

Team lead / Q decides whether this branch is pushed and the PR opened (and
whether Q wants this item done by a Codex agent instead).
