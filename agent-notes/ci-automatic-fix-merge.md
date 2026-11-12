# Automatic repairs merge without a person reading them

## Goal

Q 2026-11-11: make the fuzz -> issue -> fix -> PR -> merge -> conformance test
chain run with nobody reviewing, without letting a wrong fix through. "make it
so" given the same day. This branch is the merge path. The overall plan and
its evidence (PR record, four papers, measurements) are in
agent-notes/automatic-machine.md in the main checkout.

## State

Local commits only; nothing pushed. Waiting for session barn-41's branch
ci/fuzz-report-dedupe to merge first (it edits the repair prompt text and
repairs the "Make the pull request close its issue" step in claude-fix.yml);
then rebase this branch on master and push.

- scripts/automatic_fix_scope.py + test: a repair is in scope when it adds a
  fuzz input and removes no line from an existing Go test and touches no
  existing test data, workflow, script, profile, agent rules or build
  configuration. Replayed on the 16 merged claude/issue-* PRs: 11 in scope.
- .github/workflows/claude-fix.yml, `fix` job, new steps after the existing
  ones: find the repair PR; scope check with the script taken from master;
  second Claude run that writes the conformance test and runs it on Toast and
  on the PR's Barn before the merge, records one of test / not-observable /
  already-covered / toast-disagrees, and arms the merge when told it may;
  open the conformance PR; a final step that recomputes the decision and
  disarms the merge unless every condition holds, labelling the PR
  `needs-decision` with the reasons otherwise.
- `conformance` job: no longer runs Claude. When the repair merges it merges
  the already-open conformance PR (only if that PR adds test files and nothing
  else); when the repair closes unmerged it closes the conformance PR.
- No on/off switch and nothing that turns the machine off. Q 2026-11-11: "I do
  not want the machine to shuit itself down!" An earlier local commit had a
  repository variable AUTOMATIC_MERGE_ENABLED that left queueing off until
  set; Q never asked for it and it is removed.
- actionlint 1.7.12 passes on claude-fix.yml. zizmor was not run: the local
  uv configuration excludes zizmor 1.32.0 by date.

## Decisions and why

- The merge is armed by Claude's app token inside the second Claude run, not
  by a workflow step. A queue entry made with GITHUB_TOKEN starts no
  merge_group workflows and stalls: GitHub's github_token page says events
  from that token start no workflows, and several independent GitHub issue
  reports describe exactly this stall (not official documentation). The
  action revokes its app token when it ends (action.yml "Revoke app token"),
  so a later step cannot use it. The last step is the mechanical check: it
  disarms with GITHUB_TOKEN unless scope, Toast outcome, conformance PR and
  unchanged head all allow the merge.
- The scope script is read from master (`git archive FETCH_HEAD`), so a
  repair cannot change the rule that judges it.
- Steps call the REST and GraphQL APIs through `gh api`. The runner's gh is
  2.23 (2023); barn-41 found the existing close-issue step silently does
  nothing because that gh rejects `--json closingIssuesReferences`.
- Toast check before the merge, not after as Q first wrote: with the old order
  a repair Toast disagrees with was already on master when that was found.
- Out-of-scope repairs stay open with a label rather than being closed: 5 of
  the 16 good past repairs are out of scope by the rule.

## Surprises

- The first scope rule ("changes any existing Go test") rejected 8 of 16 good
  repairs; the repair agent usually appends a regression test to an existing
  test file.

## Unverified until a real repair runs through it

- That Claude's app token may enable auto-merge on the PR, and that an entry
  it queues gets its merge_group checks.
- That GITHUB_TOKEN may call disablePullRequestAutoMerge.
- That CONFORMANCE_PR_TOKEN may merge, update and close pull requests in
  moo-conformance-tests (it is known to push branches and open PRs there).
- python3 on the runners is new enough for the scope script (the required CI
  job already runs scripts/fuzz_red_green.py with it).

## Next step

When barn-41 reports its merge: rebase, re-run actionlint and the script
test, push, open the PR, queue it, watch checks. Then watch the first real
repair end to end.
