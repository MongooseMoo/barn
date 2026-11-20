# Current Toast all-conformance check state

- Scope: verify the GitHub checks run every packaged conformance case against the managed Toast oracle and fail closed on omissions.
- Current conformance main: `e9b6532c5dd9f46104739f4671137e38bc95363e`.
- Implemented by merged normal-merge PRs #60 and #61; merge commits `0e9ac7db76d8f6e06372516ae5452313aef5de4d` and `5351e62d6a67bba376c6a8551129b51af16f35e8`, each verified with two parents.
- Current workflow uses the reusable Toast core from push/PR and paired-candidate entrypoints.
- The Toast matrix runs `-m conformance` for 64-bit outbound-on, 64-bit outbound-off, and 32-bit outbound-on.
- The execution ledger rejects inexact profile surfaces, any failed/error case, and any case skipped by every profile.
- Post-merge run 31056299779 succeeded. Its ledger records 11,580 packaged cases and 11,580 executed cases. Each profile accounted for all 11,580 cases; all had zero failures and errors.
- Issue #43 is closed.
- Blocker: none for this requested check repair.
- Next action: receive the independent current-main audit, then report completion with PR/run links.
