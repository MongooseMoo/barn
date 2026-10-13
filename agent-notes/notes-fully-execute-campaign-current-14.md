# Current campaign checkpoint 14

## State

- `moo-conformance-tests` PR #38 merged normally as `7af1f021d6cb06012d79c0c0bec4fc749bdcd8c8`; the merge has two parents, contains reviewed head `704e0f67cd1cb1baf922bdcbdfe8d78d3de8a9de`, is on `origin/main`, and deliberately leaves issue #18 open.
- Post-merge Toast run `30993438113` is in progress. Harness/schema/duplicates/types/lint is green; all three complete Toast profiles are executing every packaged case.
- Barn integration candidate `agent/conformance-admission-repairs` contains the isolated #97 and #100 commits plus #103 commit `d797c94`; full `go test ./... -count=1`, `go build ./...`, and `git diff --check` are green.
- Draft PR #104 closes #103 and remains unmerged. Its build/unit check is green; conformance is red from the current base admission chain.
- Draft PR #105 remains NO-MERGE. Independent review found `delete_verb` re-resolves `Verb.Name`, breaking loaded multi-alias and exact-index deletion. Worker `/root/repair_object_authority` is adding exact-identity store mutation plus regression tests.
- Draft PR #106 remains NO-MERGE. Toast supports `salt()` effectively for 2a/2y only; its 2x crypt path has historical high-byte sign-extension semantics. The PR incorrectly allowed 2x salt and only relabeled a standard 2a digest. It also lacks exact two-digit cost and permission-order coverage.
- Draft PR #96 is under independent read-only review for pending anonymous cycle persistence.

## Blockers

- Barn component PRs cannot independently pass the globally expanded conformance admission suite while `master` still has the cascading #97/#100 and downstream behavioral failures. No merge may bypass required checks.
- #97 explicitly requires independent landing on master, but the current required-check graph makes that impossible without either a combined integration landing or an approved change to the issue/merge strategy.
- Bcrypt 2x requires genuine oracle-equivalent hashing for high-bit password bytes; prefix rewriting is invalid.

## Next actions

1. Wait for post-merge Toast run `30993438113` and record all profile/union results.
2. Repair and re-review PR #105 with loaded multi-alias and overlap regressions.
3. Repair PR #106 from managed Toast-backed conformance vectors: 2a/2y salt behavior, 2x/2b salt rejection, genuine 2x high-byte crypt, exact cost width, and permission ordering.
4. Finish independent PR #96 review, then preserve accepted isolated commits on the local integration candidate.
5. Run the trusted paired workflow against the complete candidate; do not open or merge a combined Barn PR without resolving the literal #97 independent-landing conflict with the user.

