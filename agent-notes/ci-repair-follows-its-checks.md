# An automatic repair follows its pull request's checks

## Goal

Q 2026-11-12, on repair #802 sitting red: "let's figure out what happened
with the failing test on the open pr and how we can avoid this. Should
Claude babysit the PR until it closes when in auto-fix mode?" Part of the
automatic fuzz -> fix -> merge chain (agent-notes/ci-automatic-fix-merge.md).

## What happened (verified from issue #800, #802 and their run logs)

- Fuzz found a hang in FuzzStructuredSourceExecution at the commit before
  #789. #789, merged 26 minutes before the issue was filed, changes the
  structured generator, so on master the recorded bytes make a different
  program that passes.
- The repair run fixed the real bug with a source-level test and also
  committed the input, because prompt step 3 said to commit it.
  scripts/fuzz_red_green.py then failed "Build & unit tests": the input
  passes on the base. The check is right.
- The fix job had ended. #802 stayed armed, blocked and unreported from
  13:24 UTC.
- For #801 the same staleness made the run follow step 1 ("the recorded
  input passes here") and report no Barn cause, while #802's description
  records that input hanging at the issue's commit.

## State

.github/workflows/claude-fix.yml only:

- Step 1: an input that passes on master is replayed at the issue's commit
  before any conclusion; if it fails there and nothing fixed the cause, the
  bug is repaired from the program that commit generated.
- Step 3: only an input that fails on master without the fix is committed.
- New step 6: the repairing run polls its pull request's checks, fixes what
  its change broke (at most three rounds), and otherwise records the
  failing check in the pull request body.
- Final step: waits up to 30 minutes for running checks; a failed,
  timed-out or cancelled check, or one still running, is a reason: disarm,
  `needs-decision`, comment.

## Decisions and why

- Claude waits inside the repairing run, not in a second workflow started
  by the failed check: the run that wrote the fix still has the cause and
  the reproduction in context, and the scope and Toast steps then judge the
  final head. Cost: one runner is held for about ten more minutes.
- No re-dispatch path for an issue whose repair is already open: the
  conformance step would push a second time to an existing branch. #802
  itself is repaired by hand (its stale input removed).

## Unverified

- No repair has run with the new prompt.
- A check failing in the merge queue, after the fix job ended, still
  reports to nobody.
- The runner's gh (2.23) is assumed to print pending checks in
  `gh pr checks`.

## Next step

Watch the next repair run: does it wait, and what does the final step
print for the check runs.
