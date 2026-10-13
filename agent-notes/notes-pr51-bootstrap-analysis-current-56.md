# PR51 bootstrap analysis checkpoint

- Scope: read-only analyst review of exact head `94c9a8a33da5ef4a5604b528eb6c6984676210b0` against `origin/main` in `C:/Users/Q/code/.worktrees/moo-conformance-trusted-pr-check`.
- Current state: exact head and clean worktree verified. Net diff is exactly four files and two commits: workflow, execution-ledger CLI, and their two test modules. `git diff --check` is clean.
- Findings so far: the workflow checks out an immutable canonical trusted controller, checks out the exact candidate without credentials, installs/runs only trusted Python, pins all actions, uses global read-only contents permission, validates candidate paths/inventory, runs three Toast profiles, and makes the final ledger depend on both quality and profile success.
- CLI state: `--baseline` and `--inventory` are now a required mutually exclusive surface selection. Inventory mode validates strict schema-v2 identity data and enforces a zero exception baseline; legacy baseline mode still discovers the local packaged surface.
- Security question under active review: candidate databases and YAML are intentionally interpreted by trusted tooling and Toast. Need verify path confinement, symlink rejection, fixture execution boundaries, and whether any candidate-controlled host code/config can execute during `pull_request_target`.
- Architectural caveat: `pull_request` remains alongside `pull_request_target` only for this bootstrap. Until the required follow-up removes it, candidate-owned runs retain identical check contexts and the post-merge architecture is not yet fully by-construction trusted.
- Required distinction: judge the temporary candidate-owned check only as bootstrap evidence; judge the post-merge `pull_request_target` path as the intended final trust architecture.
- Constraints: do not edit the reviewed worktree, do not read project Markdown, and do not merge.
- Blocker: requested gauntlet file was absent; the installed `protocols:gauntlet` skill was read as the available protocol. No product blocker established yet.
- Next action: finish path/security tracing, run focused and full relevant tests, inspect live PR checks if available, and return a strict bootstrap verdict distinct from the final architecture verdict.
