# PR 22 adversarial review checkpoint 2

## Findings and observations

- Focused tests pass: 46 tests across runner expectations, transport parsing, and schema.
- Full non-conformance unit gate passes: 113 tests, with 11,569 conformance cases deselected.
- The current schema and runner demonstrably accept and pass four unenforced contracts: top-level `expect.output`, a `wait` step with `expect.value`, a `command` step with `expect.value`, and an empty command `expect.output` mapping.
- A YAML `expect.value: null` is parsed but indistinguishable from an absent value expectation and passes a wrong non-null result.
- Multiple parsed fields can still be ignored by early returns: `match` plus `notifications` passes with a missing notification; output `exact` plus `contains` passes when only `exact` matches.
- Current packaged YAML does not use those invalid placements or malformed shapes, so the full green suite cannot prove fail-closed construction for future suites.

## State

- Review remains read-only. No PR source/tests, commits, branches, or GitHub state were mutated.
- No historical Markdown, plans, reports, notes, issue bodies, PR bodies, or git history were read.

## Blocker

- None. Parser framing edge cases and current CI completion remain to be checked before the final verdict.

## Next action

- Probe response splitting for ambiguous notification/result forms, inspect the live current check result, and send exact findings with severity and repair direction to the parent.
