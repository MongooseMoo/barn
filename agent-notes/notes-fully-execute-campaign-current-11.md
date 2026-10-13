# Current campaign checkpoint 11

## State

- `moo-conformance-tests` PR #36 added trusted explicit Barn/conformance candidate CI and merged normally.
- PR #37 fixed candidate quality gates to run from the conformance candidate root and merged normally.
- Live paired run 30990243152 used Barn `54382063457e94cb991f08f0de4307b21d7a11cb` and conformance `13e7bf598ce693070d05c1754e8c99c4158601c0`.
- Every packaged test ran against all three pinned Toast profiles; harness quality and exact profile-union accounting passed.
- The explicit Barn job ran all 11,566 packaged identities and retained raw evidence.

## Findings

- Barn returned exit 1 with 11,070 passed, 417 skipped, 79 failed, and 0 errors.
- Most failures share one capability cause: Barn cannot answer either `server_version("options.ONLY_32_BITS")` or slash spelling, while current conformance uses that exact Toast-compatible probe to select 32-bit versus 64-bit expectations.
- Barn PR #98 covers `OUTBOUND_NETWORK` and `PROMOTE_NUMBERS`, but not `ONLY_32_BITS`.
- The remaining non-probe failures include permission behavior, bcrypt `$2b$`, finished task permission, and pending-finalization output.
- The trusted verdict correctly rejected the expected-success dispatch and produced no false green.

## Blocker

- Issue #18 still needs an exact fixture pair: one immutable before commit with exactly one required failure and no unrelated failures/errors, and one immutable after commit with zero failures/errors.
- Barn issue #97 requires its fix to land independently on master, while PRs #98 and #101 currently form a conformance dependency cycle. A combined integration PR has not been authorized.

## Next action

- Verify the current `ONLY_32_BITS` option behavior against the managed Toast evidence already produced, add a focused Barn issue/fix with table-driven unit coverage if the contract is unambiguous, and rerun the trusted paired workflow.
- In parallel, inspect current immutable PR/commit evidence for a strict red/green fixture pair without weakening the validator.
