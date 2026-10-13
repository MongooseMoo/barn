# Retain/fix checkpoint 35

## Current findings and state

- Barn PR #110 passed exact-head build/unit and default-profile conformance CI: 11,587 tests, 11,177 passed, 410 skipped, 0 failures/errors, all admission probes passed.
- PR #110 was normally merged as `d2d8ccf9a3cba21a5348e96ff2d84105dd2c6884`; it matches `origin/master` and has two parents. Issue #102 is closed. Issues #45 and #29 remain open. The merged PR title/body were narrowed to the five #102 paths so they do not imply repository-wide authority convergence.
- PR #81 / issue #4 was retained but found architecturally wrong. Canonical WSL Toast uses three distinct streams: `reseed_random()` affects only `random()`; `frandom()` is independent; `random_bytes()` is separately secure. The old PR incorrectly unified all three and replaced `crypto/rand` for bytes.
- Issue #4's live body was corrected to the canonical three-stream contract and remains open.
- Current master `d2d8ccf...` was normally merged into PR #81 as `c2f29d109455e830c3e2b511850934c756cb379b`.
- PR #81 was repaired locally in commit `69deb94474710cd12ffef6f43f822f4c76c8575e`: the locked reseedable generator is integer-only, `frandom()` remains independent, `random_bytes()` is restored to `crypto/rand`, and tests cover observable 0, arity/authority, entropy failure preserving state, stream independence, cleanup, and concurrent draw/reseed.
- PR #81 exact-head focused x20, focused race, gofmt, full vet, build, full tests, diff checks, and full repository race tests are green. Independent exact-head review is running. Nothing has been pushed and the PR has not been edited.
- Transaction cluster audit: #29, #45, and #77 remain open with shared coarse preflight/partial-apply/recommit causes. The recommended coherent repair orders atomic coarse prevalidation, terminal non-validation failures plus idempotent contents/scheduler guard, then failed-flush early return/view preservation.
- Parser audit: #25 is security-critical and needs central expression+statement recursion protection after Toast/corpus ceiling verification. #50 is independent quadratic diagnostic wrapping; #51 is independent big-integer conversion and should use linear modulo-2^64 accumulation.

## Blockers

- PR #81 requires independent CODE APPROVE, accurate title/body/provenance repair, push, and fresh exact-head CI before merge.
- Local managed conformance for PR #81 is blocked because the documented launcher strips Windows backslashes from the config path and Barn exits before listening. No improvisation/manual launch is authorized; fresh GitHub CI will provide exact-head acceptance.
- Parser #25 implementation is blocked on a managed Toast comparison and current stored-verb corpus scan for the numeric nesting ceiling.

## Next action

1. Finish PR #81 independent review; fix findings if any.
2. Push and accurately update PR #81, retain literal `Closes #4`, require fresh default-profile CI, then audit and normal-merge with two-parent verification.
3. Begin the #29/#45/#77 transaction repair as one ordered cluster with red tests first.
4. Separately verify the parser ceiling against Toast and the current corpus before implementing #25.

This note is intentionally uncommitted and must not be staged, committed, or pushed.
