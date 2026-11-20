# Fuzz reports: distinct failed checks, and repair runs that close issues

## Goal
Q ordered ("make it so") that open fuzz issues get handled by the repair
machine. Three causes were verified.

## State
- `scripts/fuzz_report.py`: a failed check's signature is
  `target|failed check|file:line: message`, with quoted strings replaced by
  `S` and digit runs by `N` (`check_message`). Before, it was
  `target|failed check|file:line`, so every FuzzReadDatabase round-trip
  mismatch (reader_fuzz_test.go:82) shared one signature: 11 "property
  definitions/order differs" reports became comments on #722 and never
  dispatched a repair; #707 absorbed a WriteCheckpoint failure the same way.
- Raw failed-check reports from #681, #693, #707, #721, #722 are saved in
  `scripts/fixtures/fuzz/failed-check-reports-2026-11-11.json`; the test
  requires six distinct findings with one signature each.
- `.github/workflows/claude-fix.yml` prompt step 1: when the recorded input
  passes on the checkout (master), the agent posts its one evidence comment and
  closes the issue as completed; a not-a-Barn-defect conclusion closes as not
  planned. Before, such runs commented and left the issue open (#681, #693,
  #707, #721, #761, #765).
- The "Make the pull request close its issue" step read
  `closingIssuesReferences`, which the runners' gh 2.23.0 does not have; the
  error was inside `for n in $(...)`, which bash -e ignores, so for #723 the
  step passed and edited nothing. It now reads `body`, tests it for a closing
  keyword plus `#N`, and assigns the query before the loop so a failure fails
  the step.

## Decisions and why
- Normalizing numbers and strings keeps repeats of one bug together (11
  property-order reports, the "too many local variables" repeats) while the
  message text splits different bugs.
- The closing-keyword regex has no backslashes: `\\b` in a `--jq` argument was
  turned into a backspace by Windows argument handling during local checks.

## Surprises
- Open issues filed under the old signature no longer match; the next finding
  opens a new issue and dispatches repair, which is wanted. The closed-issue
  path (agent-notes/fix-fuzz-closed-issue-dedupe.md) matches on the marker
  only and is unchanged.

## Next step
Push, open the PR, and watch CI.
