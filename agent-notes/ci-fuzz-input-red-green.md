# CI: added fuzz inputs must fail before the change

## Goal
Q 2026-11-10: fix PRs must show red/green: the committed fuzz input fails on
the commit before the fix and passes with it ("Let's make it hard! Let's build
this machine!").

## State
scripts/fuzz_red_green.py: for each file a change adds under
`<package>/testdata/fuzz/<Target>/`, replays it as that target's seed
(`go test <pkg> -run '^Target$/^name$'`, 120 s timeout) in a worktree of the
base commit, where it must fail, and in the checkout, where it must pass.
Step "Added fuzz inputs fail before the change" in ci.yml's required
"Build & unit tests" job runs it on pull_request and merge_group (base: the
merge commit's first parent, or `merge_group.base_sha`). Tests:
scripts/test_fuzz_red_green.py (verdicts, plus scratch-repo runs with a real Go
fuzz target; those skip on Windows, where `timeout` is not GNU timeout).

## Decisions and why
- In the required job instead of a new job: required checks are "Build & unit
  tests", "MOO conformance suite", "Serializable histories and negative
  control"; a new job would not gate the merge queue without a settings change.
- Applies to every PR, not only `automatic-fix` ones: merge_group events carry
  no labels, and a regression input that never failed proves nothing whoever
  wrote it. Exemption: inputs for a target the base lacks ("no tests to run"),
  i.e. seeds of a new fuzz target. Since 2026-10-01 every other added input
  came with a fix.
- A base that does not build counts as a failure of the check, not as red.
- A hang (timeout exit 124) or kill on the base counts as red.

## Surprises
Verified on real data: at #706's merge, with base its first parent,
`parser/testdata/fuzz/FuzzParseFormatRoundTrip/763eb47a4391919b` fails on the
base (exit 1) and passes with the change.

## Next step
Watch the first automatic-fix PR after merge show the check in its CI log.
