# Current campaign checkpoint 7

## Current findings

- `moo-conformance-tests` default branch is `0615fc78544b806390906971942835e08a34d844`.
- Its trusted `Toast conformance` workflow runs every packaged case with no suite selector against pinned Toast in three profiles: 64-bit/outbound-on, 64-bit/outbound-off, and 32-bit/outbound-on.
- The final `Full suite` job consumes all three JUnit artifacts and enforces an exact execution union plus the reviewed never-executed baseline.
- Default-branch run 30985285334 is green: harness quality, all three Toast jobs, and `Full suite` all succeeded.
- PRs #34 and #35 landed the complete-profile execution and stabilization; issue #17 is closed.
- The uncommitted issue #18 paired-candidate workflow currently runs harness tests and collection, then executes the complete suite against Barn. It does not independently execute the candidate conformance commit against Toast inside that dispatch.

## State and blocker

- The user's immediate Toast-all-tests requirement is already satisfied on ordinary conformance PR and push checks.
- For paired-candidate CI, the safe contract is to make Toast admission a prerequisite without weakening or duplicating the exact full-suite accounting.
- No blocker is established. No project historical Markdown was read.

## Next action

- Finish the issue #18 workflow/tests so explicit candidate pairs preserve full Toast-oracle admission, run focused and full local gates, adversarially review the trusted workflow, then create a scoped PR without prematurely closing #18 before live red/green fixture proof.
