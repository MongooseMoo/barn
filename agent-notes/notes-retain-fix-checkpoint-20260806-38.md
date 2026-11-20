# Retain/fix checkpoint 38

## PR #116 remains unmerged

- PR #116 exact head remains `09992c83716d624d914bfbe427f6d7ccbe6ed02a`; live `origin/master` and GitHub master remain `b3d730e8bd3579517124fd011126f0d514547ce2`.
- The code/issue/commit/file audit is complete and all three closure references (#29, #45, #77) are recognized.
- Attempt 1 managed conformance passed with 11,587 total / 11,177 passed / 410 skipped / 0 failed / 0 errors and four passed admission probes.
- Attempt 1 build job `92677774234` and attempt 2 build job `92680616186` each received no runner, ran no steps, and were cancelled after 15:01.
- Full attempt 3 build job `92683227912` received GitHub-hosted runner `1000003250` but failed in `Set up job` before checkout. Exact annotations: `Service Unavailable` and `Failed to resolve action download info.` Attempt 3 conformance job `92683227976` received no runner and was cancelled after 15:01.
- GitHub's official status API confirmed an active critical incident and major Actions outage: workflows delayed/failing to start, queued jobs timing out, Actions API errors. Do not blindly rerun until official status reports recovery.
- Required checks are not green, so do not merge. After recovery, rerun the unchanged head; only then repeat the literal audit and use `gh pr merge 116 --repo MongooseMoo/barn --merge`, verify exactly two merge parents, and verify #29/#45/#77 closed.

## Parser issue #25

- Red contract commit: `5ebb5ed5052bd7f01d416dbaadf2cde624beb34f`.
- Main implementation: `e584f1ee0f0250d3a2d301729c06c419758ff842`.
- Independent review found noncanonical wrapped depth details for nested list/map parse paths.
- Separate red-first review fix: `55978fdecfd8f3354cf5ecc1f58d0dff5e198198 fix: preserve canonical nesting diagnostics`.
- Final parser head is clean/held at `55978fd`, still based on `b3d730e8`, not fetched/merged/pushed.
- Review-fix exact scope is three files, +77/-2. It canonicalizes only errors that resolve to `*verb.NestingDepthError`, preserving other parse details.
- Review-fix gates all passed: exact red reproduction, focused x20, focused race, gofmt, vet, build, full tests, full race (scheduler 235.282s), diff-check, clean status.
- Worker is paused; coordinator can resume it after #116 provides an exact new master SHA for one normal merge and final gates.

## Next bytecode cluster

- Current read-only audit: #69/#70 share unchecked uint16 patch truncation; #39 is a distinct 0xFF dynamic-name sentinel collision. One compiler-operand-representability PR can be coherent only if adjacent backward loop offsets are included.
- A read-only subagent is measuring Toast's 255/256 constant and 65,535/65,536 handler/fork behavior using only the managed WSL oracle script and `/root/src/toaststunt/build-release/moo`.
- No bytecode implementation starts until Toast establishes the user-visible acceptance/rejection and `set_verb_code` diagnostic/preservation contract.

This file is an uncommitted recovery note. Never stage, commit, or push it.
