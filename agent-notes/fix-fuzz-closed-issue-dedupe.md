# Fuzz reports: repeats of a closed issue on an older commit

## Goal
Q 2026-11-10 asked why so many automatic fuzz issues stay open unaddressed,
proposed taking a fixed fingerprint "out of circulation", and said "Make it
so!" to this change.

## State
`publish()` in scripts/fuzz_report.py lists issues with `state=all`. Per
signature: an open matching issue gets the comment, as before. Otherwise, if
matching issues are closed, it reads the fuzzed commit's committer date
(`GET /commits/{sha}`, once per commit) and compares it with the latest
`closed_at`. Closed after the commit: comment on that closed issue with a note
that the commit predates the fix; no issue, no dispatch. Closed before: new
issue and repair dispatch (a regression). Three PublishTests cover the cases.

## Decisions and why
- Evidence: #707 repeated #698 (closed 06:26 when #699 merged) from a cycle
  fuzzing d2457f0f, which predates #699; #681 repeated #677 from an older
  commit. The old code only searched `state=open`, so each opened a new issue
  and a repair run that found the fix already on master.
- Commit date vs closing time instead of claims or pauses: a fix merged after
  a commit was made cannot be in that commit, and cycles fuzz master heads.
- The commit lookup happens only when a closed issue matches, so the campaign
  step tests' fake API (created issues, no `state`) is unaffected.

## Surprises
None yet.

## Next step
PR is blocked by the pre-PR hook until Q names it. Not covered: same bug
under a different target's signature (#693 vs #687); the repair agent's
duplicate check handles that.
