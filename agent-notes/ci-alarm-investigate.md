# Alarm investigation: Claude reads every new alarm issue and leaves a verdict

## Goal
Q 2026-11-15: "can we connect intelligence to alarms?", then "Make it so!".

## State
- Branch ci/alarm-investigate (worktree C:/Users/Q/code/barn-alarm-investigate).
- .github/workflows/alarm-investigate.yml: workflow_dispatch(issue_number);
  Claude Code (claude-opus-5-5, same install/settings as claude-fix.yml) reads
  the alarm issue, its run and series, and:
  - goodput: lists points with load, the commits in range, re-measures the
    commit before and after alternately (idle-gated, harness settings as in
    goodput-series.yml, numbers only posted), finds the responsible commit;
  - conformance suite: step seconds over time, merges in barn and
    moo-conformance-tests, junit before/after (tests lost, times moved).
  - one comment with the verdict; noise -> close "not planned", explained ->
    close "completed", defect -> fix PR (Fixes #N, not armed) or handoff.
- scripts/conformance_suite_alarm.py `investigate()` dispatches it after an
  alarm issue opens (GITHUB_TOKEN issue events start no workflows;
  workflow_dispatch is exempt, as fuzz_report.py does for claude-fix). Both
  alarm workflows now have `actions: write` for the dispatch.
- Independent of PR #851 (alarm baseline/commit-split); both edit the alarm
  scripts in different places.

## Decisions and why
- Dispatch, not an `issues` trigger: the alarm opens its issue with
  GITHUB_TOKEN, which starts no `issues` workflow.
- Fix PRs are not armed for merge: Q's automatic-merge scope rule applies to
  fuzz repairs with a replayed input; a perf fix has no such proof yet.
- Closing is allowed for noise/explained: a closed alarm restarts the
  comparison (PR #851), so a wrong close costs at most one week of time
  verdicts; the comment carries the evidence.

## Unverified
- No run yet. First real check: after merge, `gh workflow run
  alarm-investigate.yml -f issue_number=829` (or the next alarm).

## Next step
- After merge, if #828/#829 are still open, dispatch it on both and read the
  verdicts against my own diagnosis (in agent-notes/ci-alarm-baseline-and-commit-split.md).
