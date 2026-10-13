# PR #46 adversarial review checkpoint

- Task: adversarially review `moo-conformance-tests` PR #46 at exact head `a8756eef330ba686f898db3eb7ddd283492c7c3a` against issue #43.
- Scope: read-only review of current code, exact PR diff, current tests, and live CI run `31006666628`; do not read historical Markdown.
- Required attacks: pytest hook ordering and plugins/custom selectors; evidence artifact substitution, replay, and context collision; blocked/skipped semantics; producer/consumer mismatch; candidate-controlled inputs; workflow output/environment injection; phase ambiguity; partial packaged surface; direct CLI filtered paths; legacy shims; Toast admission becoming conditional.
- Current state: review not yet started.
- Blocker: none known.
- Next action: read the adversary skill, inspect exact PR metadata/diff and issue #43, then run focused non-mutating tests and report exact-head MERGE or NO-MERGE with concrete evidence.

## Review update

- Exact head confirmed: `a8756eef330ba686f898db3eb7ddd283492c7c3a`; PR is open/draft and live CI run `31006666628` is green in all five jobs.
- Issue #43 requires a complete packaged JUnit surface, fail-closed identities/phases, and no candidate-controlled expected-failure baseline.
- Current workflow creates `expected-case-ids.json` by importing `packaged_case_ids()` from the checked-out conformance candidate, uploads it with raw evidence, then the trusted controller accepts that candidate-produced file as `--expected-ids`.
- Concrete risk: a conformance candidate can remove packaged cases and thereby shrink both execution and the purported complete inventory; the trusted controller has no independent default-branch inventory anchor. The unconditional Toast matrix runs the same reduced candidate surface, so its green result does not close this gap.
- Current admission evidence validates exact schema/order/status/prerequisite relationships and binds a plain context string containing Barn SHA, conformance SHA, and profile. Remaining review must determine whether context alone prevents substitution/replay and whether pytest hook ordering permits a later selector plugin to remove admission after the `trylast` check.
- `ward set adversary` was attempted as required by the skill but `ward` is unavailable; read-only discipline is being enforced manually.
- Next action: finish paired-result producer/consumer review, inspect tests covering attack boundaries, evaluate hook ordering and direct CLI selection, and report exact-head MERGE/NO-MERGE.
