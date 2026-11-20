# Current campaign checkpoint 15

## Conformance checks completed

- `moo-conformance-tests` PR #38 is verified merged normally as `7af1f021d6cb06012d79c0c0bec4fc749bdcd8c8`, with two parents and reviewed head `704e0f67cd1cb1baf922bdcbdfe8d78d3de8a9de` included. Issue #18 remains open intentionally.
- Post-merge `main` run `30993438113` is fully green:
  - harness quality: 267 tests; schema collection: 11,566 identities
  - Toast 64-bit outbound on: 11,495 passed, 71 skipped
  - Toast 64-bit outbound off: 11,480 passed, 86 skipped
  - Toast 32-bit outbound on: 11,476 passed, 90 skipped
  - exact union: 11,525 of 11,566 executed; 41 reviewed skips
- Conformance issue #39 now tracks missing bcrypt prefix/cost semantics. Worker `/root/bcrypt_conformance` is adding managed-Toast cases; no manual Toast launch is permitted.

## Barn repairs

- Integration candidate `agent/conformance-admission-repairs` contains isolated #97/#100 plus #103 commit `d797c94`; full Go test/build/diff gates are green. It has not been opened as an integration PR.
- Draft PR #105 was repaired and pushed at `56891ce905bd8f3a511c1af5cad6b2c3e7526118`. It now uses opaque store-bound exact verb identity and covers loaded multi-alias, overlapping aliases, exact index targeting, and stale references. Build/unit CI is green; independent re-review is running. PR body contains literal `Closes #102` and GitHub closing linkage remains correct.
- Draft PR #106 was corrected at `fef968971476bf53efc1352ecdaee608b6814943` after a NO-MERGE review. It now:
  - supports/preserves 2a/2y salt prefixes
  - rejects 2b/2x salts
  - returns Toast's `*0` for unsupported 2b crypt
  - requires exact two-digit bcrypt cost at hashing while preserving Toast's permission ordering
  - uses exact hash assertions instead of prefix-only checks
  Focused repeat/race and full Go test/build/diff gates are green. Build/unit CI is green; conformance is still running and is expected to expose unrelated base failures. PR remains draft and closes #72.
- PR #96 original head remains NO-MERGE due mutable-graph root minimization. Clean repair commit `ff95ff75b7da18ee5cb077a4c26c42f73e42f2b8` retains all direct VM anonymous roots deterministically and passed focused repeat/race plus full Go gates. It is not pushed; independent review is running. The dirty original worktree remains untouched.

## Blockers and next actions

1. Await independent verdicts for repaired PR #105 and PR #96 repair.
2. Independently review corrected PR #106 after reviewer capacity frees.
3. Land conformance #39 only after all complete Toast profiles and exact-union admission prove the new bcrypt cases.
4. Add a distinct Barn issue/repair for genuine Toast 2x high-byte hashing only after #39 provides managed runtime proof.
5. Preserve accepted component commits on the local integration candidate and run paired candidate CI.
6. Do not open or merge a combined Barn PR until the conflict with #97's literal independent-landing criterion is resolved with the user; never bypass red required checks.

