# Current conformance campaign checkpoint

- Task: fresh adversarial audit of moo-conformance-tests PR 48 exact head `81747a1e1f1f24e0552067b97946e6c619a27b93` against main `4f7e56d042d57f19312a8696edbc2f71d35d1c5b`.
- Net scope: four files: delete the Toast exception JSON, remove execution-ledger baseline semantics, update ledger tests, and retain the workflow regression that the exception file stays absent.
- Topology: current main is an ancestor; final repair commit `81747a1` follows merge commit `0300857`.
- PR metadata: updated exact-head evidence, literal `Closes #47`, and explicit disclosure of the unchanged GitHub Free context-provenance limitation.
- Managed run `31036337923` is fully green at the exact head, including all three Toast profiles and `Full suite`.
- Required audit: prove inventory is the sole ledger surface, every-profile skips fail, reviewed-skip output is absent, candidate data cannot evade exact union, and managed artifact facts match the PR body.
- Schema note: pre-merge ledger artifact is expected to remain trusted-base schema 1; the post-merge main run must prove the new schema 2 output.
- Blocker: none established yet in this fresh audit.
- Next action: inspect exact source/test diff and retained symbols, then independently inspect managed artifacts before returning MERGE or NO-MERGE.
- This note is intentionally untracked and must not be staged, committed, or pushed.
