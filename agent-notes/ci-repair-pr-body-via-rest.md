# A repair's "Fixes #N" is written with the REST API

## Goal

Q 2026-11-15 ("Make it so"): repaired fuzz issues stay open. Issue #841
stayed open after its repair #842 merged.

## What happened (from run 43652329130 and PR #842)

- The repair agent's #842 body had no "Fixes #841".
- claude-fix.yml step "Make the pull request close its issue" found #842 and
  ran `gh pr edit 842 --body-file ...`. The runner's gh is 2.23; its `pr edit`
  queries classic project cards, which GitHub retired. The log:
  `GraphQL: Projects (classic) is being deprecated ... (repository.pullRequest.projectCards)`,
  exit 1. The body stayed unchanged and the step failed.
- Every later step (find the PR, scope, Toast check, queue) was skipped, so
  #842 was never checked or queued; it was merged by hand.

## State

- The step reads the open PR and its body through `gh api` (REST) and writes
  the body with `gh api -X PATCH .../pulls/N -F body=@file`. `set -euo pipefail`.
- The repair prompt's step 6 told the agent to use `gh pr edit` for its
  "Failing checks" section; it now names the same REST call.
- Read-only dry run of the new list query: issue 841 selects #842, issue 847
  (body has "Fixes #847") selects nothing.
- actionlint 1.7.12 passes (shellcheck not installed locally, so the shell
  in run blocks was not linted).

## Decisions and why

- REST rather than a newer gh: the other steps already use `gh api` because
  of the 2.23 runner gh; upgrading the runner's gh is a host change outside
  this workflow.

## Unverified

- The PATCH has not run on a real repair yet; GITHUB_TOKEN with
  pull-requests: write is expected to allow it (the REST endpoint needs that
  permission).

## Not this branch

- #841 itself is still open with its fix merged; closing it is a post left to
  the lead.
- #801, #765, #761 have no repair PR at all (see the lead's report).

## Next step

Watch the next repair whose body lacks "Fixes #N".
