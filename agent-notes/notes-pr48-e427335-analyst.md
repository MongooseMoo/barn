# PR 48 analyst checkpoint

- Scope: read-only review of moo-conformance-tests PR 48 at exact head `e4273354d3fe9ed70670fa61ecf8afb30a9c00f7` against trusted base `6068c535159e352c159eb1c453aec61f013bedbb`.
- Worktree is clean and its HEAD matches the PR head and origin branch.
- Net diff is exactly five files: Toast workflow, deletion of the never-executed baseline, execution ledger, ledger tests, and workflow tests.
- Local targeted tests: 38 passed. Ruff passed. Local mypy was unavailable in the non-dev temporary environment; exact-head CI quality, including mypy, passed.
- CI run 31025731324 completed successfully. It is a pull_request run whose API payload and checkout logs bind head `e4273354d3fe9ed70670fa61ecf8afb30a9c00f7` and base `6068c535159e352c159eb1c453aec61f013bedbb`.
- Three Toast profile artifacts are present. The trusted inventory has 11,580 identities, the candidate inventory has the same 11,580 identities with zero additive identities, and the execution ledger reports all 11,580 executed. The two artifact identity sets match in this run.
- Per-profile outcomes: 32-bit outbound-on has 11,531 passed and 49 skipped; 64-bit outbound-off has 11,535 passed and 45 skipped; 64-bit outbound-on has 11,550 passed and 30 skipped. All profiles report zero failures and zero errors.
- Oracle checkout is pinned to `aecc51e9449c6e7c95272f0f044b5ba38948459e`; each completed build step succeeded after HEAD, full-status, and diff cleanliness checks.
- Blocker: the trusted controller writes `toast-candidate-inventory.json`, but the union gate never consumes it. The union runs candidate `moo_conformance.execution_ledger`, which recomputes expected IDs through candidate `packaged_case_ids()`. Candidate discovery can therefore omit identities from both execution and the candidate ledger while the trusted inventory artifact records them; no step compares these sets.
- Secondary issue: outside pull_request events, trusted-controller checkout falls back to the mutable default-branch name and does not pin the repository, so workflow_call/push provenance is not exact.
- Verdict: NO-MERGE despite green CI. Next action: require the independently trusted inventory to be an enforced input to the union gate and pin trusted-controller provenance in non-PR invocations, then rerun the exact-head protected checks.
