# Current campaign checkpoint 17

## Findings and state

- The checks-side Toast matrix repair is merged in `moo-conformance-tests`; post-merge `main` run 30993438113 passed the harness, all packaged identities on 64-bit outbound-on, 64-bit outbound-off, and 32-bit outbound-on Toast, plus the exact union gate.
- Draft conformance PR 40 at `610906da361713ae81e45245de9883e410049ac7` pins Toast bcrypt prefix, cost-width, and high-byte semantics. Managed run 30995941201 has a green harness and all three Toast profile jobs are currently executing every packaged case.
- Barn PR 106 at `fef968971476bf53efc1352ecdaee608b6814943` is not code-complete: its salt parser requires exactly two cost digits, while the Toast-oracle test now checks rejection of one digit and canonicalization of three-digit `004` to `04`.
- Barn PR 96 has an unpushed repair candidate at `25d689eeb6817970f0332c3e1ee5db61f03af4c5`; independent review is still pending. The dirty original worktree remains untouched.
- The integration candidate contains accepted repairs for issues 97, 100, 103, and 102, but cannot be proposed as the landing vehicle without resolving issue 97's explicit independent-landing requirement.

## Blockers

- Do not merge PR 40 until the managed three-profile Toast run and union gate are green and the exact diff/commits/closing linkage receive an independent final audit.
- Do not push the PR 96 repair until the independent review accepts the frozen-root design.
- Do not merge Barn component PRs while their required conformance checks are red from the admission dependency cycle.

## Next actions

1. Monitor run 30995941201 and capture per-profile identity totals and union evidence.
2. Receive the PR 96 and PR 106 independent verdicts.
3. If PR 40 proves the expected Toast semantics, repair PR 106 with a red regression for three-digit salt canonicalization, run focused/race/full gates, push, and re-review.
4. If PR 96 is accepted, push only its exact reviewed head to the existing PR branch and watch CI.
5. Cherry-pick only independently accepted corrections onto the paired Barn candidate, run full local gates, then use the trusted paired workflow against the exact conformance head.

This file is a local checkpoint only. It must remain untracked and uncommitted.
