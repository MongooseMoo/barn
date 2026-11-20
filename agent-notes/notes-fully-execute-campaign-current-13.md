# Current campaign checkpoint 13

## Conformance CI state

- PR #38 is open at head `704e0f67cd1cb1baf922bdcbdfe8d78d3de8a9de`.
- It consumes recorded `runtime.arch_bits` instead of probing Toast's `ONLY_32_BITS` build definition when profile metadata is available.
- Independent review found and reproduced a precedence bug where `feature.64bit` could hide contradictory architecture facts.
- A second TDD commit now reconciles `feature.64bit`, `runtime.arch_bits`, and `option.ONLY_32_BITS`; malformed or conflicting facts fail closed.
- Local proof is green: 78 requirement tests, all 267 harness tests, Ruff, mypy, and diff checks.
- The new exact-head Toast matrix run is 30992077717; harness quality is green and all three complete Toast profiles are running.

## Barn failure clustering

- After removing 71 architecture-probe failures, eight independent Barn behavior failures remain.
- Five share missing target-object read/write authority in property and verb metadata builtins. This is issue #102.
- `finished_tasks()` independently lacks a wizard gate. This is issue #103 and is delegated in a separate worktree.
- The other two clusters are bcrypt `$2b$` behavior and issue #95 pending anonymous finalization.

## Issue #102 TDD state

- Dedicated worktree: `C:\Users\Q\code\.worktrees\barn-issue-102-object-authority`.
- Branch: `agent/issue-102-object-authority`, based on current `origin/master` `aa82abf05a6a23783ba69105d0c393f926304a13`.
- A new table-driven test exercises `properties`, `add_property`, `delete_property`, `verbs`, and `delete_verb` against a real transaction.
- Red proof: all five unauthorized calls currently succeed instead of returning `E_PERM`.
- Tests also assert denied mutations leave both properties and verbs unchanged and pin owner, object flag, and wizard positive paths.

## Blocker

- Barn feature PRs still cannot obtain a green full conformance check independently because issues #97 and #100 form an admission dependency cycle and #97 explicitly requires independent landing on master.
- No strict one-failure before and zero-failure after fixture pair exists yet for conformance issue #18.

## Next action

- Add one transaction-aware object authority helper and route the five issue #102 builtins through it, preserving validation precedence.
- Complete and normally merge PR #38 only after its exact-head three-profile Toast run and independent re-review are green.
