# Repair PRs always close their issue

## Goal
Part of Q's overnight order (2026-11-10: "start solving these problems and
actually merging the prs that solve them ... I want to wake up to a better
machine"): a merged automatic repair must close the issue it fixes.

## State
claude-fix.yml `fix` job: step "Make the pull request close its issue" runs
after the agent. For each open PR from `claude/issue-$ISSUE` whose
`closingIssuesReferences` lacks `$ISSUE`, it appends "Fixes #$ISSUE" to the
body with `gh pr edit` (GITHUB_TOKEN; the job already has pull-requests: write).

## Decisions and why
- Evidence: PR #696 "(fixes #692)" merged 2026-11-10 08:25 but #692 stayed open;
  `closingIssuesReferences` was empty and the body had no "Fixes #692". GitHub
  links issues from the body only. #706 and #711 had it and closed theirs.
- Mechanical step instead of more prompt text: the prompt already asks for
  "Fixes #N" in the body (step 5); the agent did not always follow it.
- Detection query tested on real PRs: issue 692 -> 696 flagged; 705 -> none.

## Surprises
None.

## Next step
After merge, #692 itself still needs closing by hand (its fix is merged); that
is a post under Q's name, left to Q.
