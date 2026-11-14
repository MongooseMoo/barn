# claude-fix: a conformance pull request merges only after its checks

## Goal
moo-conformance-tests main went red at 14:44 UTC on 2026-11-14 when its PR
#202 (written by the automatic repair for barn #831) merged 55 seconds after it
was opened. Its 32-bit Toast profile then failed
`next_recycled_object_invarg_min_int` (an ungated 64-bit literal), and every
conformance pull request was red until moo-conformance-tests #203.

## State
- Cause: `.github/workflows/claude-fix.yml`, step "Merge or close the
  conformance pull request", called `PUT pulls/N/merge` first. The
  `CONFORMANCE_PR_TOKEN` account can bypass the suite's branch protection
  (enforce_admins is off), so the merge did not wait for the required checks.
- Change: the step brings a behind branch up to date, then enables auto-merge,
  which waits for the required checks. It merges directly only when auto-merge
  refuses because the required checks have already passed (`mergeable_state`
  clean or unstable), and otherwise fails the step.
- actionlint v1.7.12 passes on the workflow. The step has not run yet; it runs
  on the next merged automatic repair that writes a conformance test.

## Next step
Merge; read the next claude-fix run that writes a conformance test.
