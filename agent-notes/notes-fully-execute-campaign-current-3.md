# Current campaign checkpoint

- Conformance main was proven nondeterministic by eight 32-bit `slice()` assertions: pinned Toast reads a string-pointer union field as a signed integer, so ASLR changes `E_RANGE` versus success.
- Issue 17 is reopened. Recovery PR MongooseMoo/moo-conformance-tests#35 is draft at exact head `a51dfdf9474583e81490812888d481aa47626d15` and closes #17.
- PR #35 changes only `slice_call_shapes.yaml`: removes the eight invalid 32-bit claims and strengthens the original deterministic 64-bit identities with real map lookup/default assertions.
- Managed WSL Toast focused proof: 57 passed. Local gates: 228 harness tests, 11,566 packaged cases, duplicate baseline 45/28, Ruff, mypy, and diff check all green.
- PR workflow run 30984269089 is executing all 11,566 packaged identities against 64-bit outbound-on, 64-bit outbound-off, and 32-bit outbound-on Toast profiles with strict unexpected-skip enforcement; the Full suite ledger will require exact per-profile surfaces and union accounting.
- Do not merge from one green run. Require independent adversary/verifier approval, inspect ledger artifacts, rerun exact head to prove stability, then ready/audit/normal merge and verify two-parent merge plus post-merge main green and issue closure.
- Do not read historical Markdown. Preserve dirty Barn and sibling conformance roots. The clean authoritative worktree is `C:\Users\Q\code\.worktrees\moo-conformance-issue-17-stability`.
