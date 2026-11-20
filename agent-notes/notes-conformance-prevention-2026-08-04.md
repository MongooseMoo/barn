# Conformance prevention research checkpoint

## Current findings

- The sibling suite contains 487 YAML files across audit, basic, builtins, capabilities, command, features, fork, generated builtins, language, objects, server, and VM families.
- Managed mode copies a selected database into a temporary server directory, starts a templated server command, and exposes restart, checkpoint-output adoption, log assertions, file assertions, and fixture writes to YAML tests.
- The current suite history includes a concentrated 2026-07-28 checkpoint/restart contract series, following 2026-07-21 skip-condition and parser fixes.
- Current documentation is inconsistent: `CLAUDE.md` gives the managed WSL Toast command, while `docs/TOASTSTUNT.md` still centers a manual server workflow and even recommends `nc`/telnet despite `CLAUDE.md` prohibiting `nc` for MOO testing.
- Historical verified results distinguish a default green-on-executed-rows run from strict coverage: the recorded default had 11,409 passed and 149 skipped; with canned Toast databases and unexpected-skip rejection, six executed anonymous startup-repair cases remained genuine mismatches amid profile/capability skip rejections.

## Observations and state

- Both Barn and `moo-conformance-tests` are very noisy with untracked artifacts. No existing files have been modified, staged, restored, or deleted by this research.
- `moo-conformance-tests` has no tracked changes in the initial status check; its latest tracked commits are test-focused checkpoint persistence and managed-lifecycle contracts.
- The requested final artifact is `docs/reports/research-conformance-prevention-2026-08-04.md` in Barn.

## Blocker

- No research blocker. A mandatory host checkpoint required this extra uncommitted note even though the delegated task requested that only the report be created.

## Next action

- Quantify skip/requires/profile usage, inspect the runner/plugin/schema and Barn wrapper line-by-line, map weak contracts to common causes, then write the requested report with evidence and sequenced PR slices.

## Current-state-only planning update

- This planning pass deliberately excludes historical plans, notes, reports, prompts, and summaries from its evidence. Earlier historical statements above are not inputs to the plan.
- Live GitHub state is 62 open Barn issues and one open PR, #81. PR #81 closes only #4, has one commit, and is mergeable but blocked by a failed required conformance check.
- Current Barn `master` is `aa82abf05a6a23783ba69105d0c393f926304a13`; current conformance `main` is `aa312b976804df05c9f9e43a02443a49697d91f3`.
- Current conformance code fails open on malformed suite collection and ignores several parsed expectations and requirements. Collection/schema/runner hardening must precede relying on new regressions.
- Current issue clusters are transaction integrity, checkpoint/load correctness, task lifecycle locking, server connection/input concurrency, bytecode operand bounds, byte-string semantics, builtin validation/limits, parser resource bounds, and performance/maintenance.

## Current blocker

- No technical planning blocker. Both repositories are heavily dirty, so future execution must use isolated branches/worktrees and exact-path staging.

## Current next action

- Finish the read-only campaign synthesis, with conformance-infrastructure PRs first, then security/data-integrity waves, then semantic correctness and performance waves. Do not implement or mutate GitHub until explicitly authorized.

## Execution update 2026-08-04

- The user authorized full execution of `plans/barn-holistic-conformance-issue-campaign-2026-08-04.md`.
- Live `moo-conformance-tests` default state is `origin/main` at `c34ebf631e94c10799996dd13d9cc574969683d1`; the noisy local `main` is 15 unpushed commits ahead and is not being used as the campaign base.
- Filed conformance issue #9 for five ignored expectation fields and issue #10 for fail-closed collection and strict unknown-field validation.
- Issue #9 worktree: `C:\Users\Q\code\.worktrees\moo-conformance-issue-9`, branch `agent/issue-9-toast-expectations`.
- Toast oracle established: `all_members(..., {})` returns an empty list; the three negative-owner `create()` cases return anonymous objects; `create_permission_wizard_owns_nothing_owner` returns a normal object.
- Focused managed results: Toast 12 passed and Barn 12 passed.
- Full managed results: Toast 11,417 passed, 149 skipped, 0 failed; Barn 11,438 passed, 128 skipped, 0 failed.
- Harness-unit subset: 71 passed. The untouched duplicate lint baseline is red with 45 duplicate-name and 28 duplicate-content groups; it is not part of issue #9.
- Current issue #9 diff is limited to two YAML files, but `objects.yaml` needs line-ending normalization before commit. JUnit XML files are untracked and must not be staged.

## Execution blocker

- No active blocker. Awaiting the independent issue #9 analyst after line-ending cleanup.

## Execution next action

- Normalize `objects.yaml` line endings, re-audit the exact diff, commit only the two YAML paths, push, open a PR with `Closes #9`, verify scope/checks, normal-merge it, and verify the two-parent merge plus issue closure. Then rebase issue #10 on the new default branch and implement fail-closed collection with TDD.

## Issue 9 delivery update

- Line endings were normalized and the final diff was exactly five one-line expectation replacements across two YAML files.
- Independent analyst verdict: APPROVE with no findings.
- Commit: `c989d5dc2eebff75dcf3dc058d156cc518eba2e5` (`test: enforce ignored object expectations`).
- PR: `MongooseMoo/moo-conformance-tests#11`, one commit, two files, literal `Closes #9`, mergeable `CLEAN`, no repository checks configured.
- Pre-merge base audit matched exactly: PR parent and remote `main` were both `c34ebf631e94c10799996dd13d9cc574969683d1`.
- Executed the required normal merge command: `gh pr merge 11 --merge --repo MongooseMoo/moo-conformance-tests`.

## Issue 9 post-merge next action

- Verify PR #11 is merged, issue #9 is closed, and the merge commit has exactly two parents. Then update the clean issue #10 worktree onto the new remote `main` and begin red tests for fail-closed collection and strict unknown-field validation.
