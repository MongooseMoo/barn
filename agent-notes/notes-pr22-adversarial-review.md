# PR 22 adversarial review checkpoint

## Current findings

- PR head is `638a19772407e6c707d350c2217edaf89fc26f78`, based on `a031527261632d262e1432344e0a5113119635de`, with six changed files.
- The runner now routes top-level and `run`/`verb_setup` step results through a shared `_verify_expectation`, adding `contains`, `range`, `notifications`, and `satisfies` checks.
- Socket response parsing now guesses the final result line and treats preceding framed lines as notifications.
- Review concern under investigation: the schema still accepts `Expectation.output` in every placement, while execution checks it only for command/secondary-connection actions; other expectation fields on those actions may still be ignored.
- Review concern under investigation: result-line guessing may misclassify legitimate multi-line scalar results or notifications that resemble result/traceback framing.

## State

- Read-only review only. No source/test edits, commits, pushes, or GitHub mutations.
- No Markdown, plans, reports, notes, PR bodies, issue bodies, or git history have been read.

## Blocker

- None. Exact schema/action routing and parser edge cases still need executable verification.

## Next action

- Inventory every packaged expectation field by action and top-level route, inspect the final parser and verifier code, add read-only synthetic probes where useful, run focused/full unit tests, then return APPROVE or exact findings.
