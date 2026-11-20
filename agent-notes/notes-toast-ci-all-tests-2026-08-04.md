# Toast CI all-tests checkpoint (2026-08-04)

## Request

Make the checks run every conformance test against Toast.

## Current findings

- `moo-conformance-tests` main contains the required `Full suite` Toast workflow from normal-merged PR #21.
- The workflow is selector-free and recursively discovers the complete packaged YAML corpus against pinned Toast commit `aecc51e9449c6e7c95272f0f044b5ba38948459e`.
- The latest PR #22 run collected 11,569 cases, passed 11,431, and skipped 138.
- The workflow explicitly accepts 138 skips, so it does not satisfy the literal zero-unexecuted-tests request.
- Aggregate counts alone allow remove-one/add-one collection substitution; exact executed node identity also needs fail-closed accounting.
- Open issue #17 covers required CI and strict skip accounting, but the user's literal request is stricter than its existing reviewed-skip-baseline wording.

## Work state

- Clean worktree: `C:\Users\Q\code\.worktrees\moo-conformance-issue-17`
- Branch: `agent/issue-17-run-all-toast`
- Base: `origin/main` at `a031527261632d262e1432344e0a5113119635de`
- Current branch is clean; no implementation changes yet.
- Current Toast artifact downloaded read-only to `C:\Users\Q\AppData\Local\Temp\toast-conformance-30967048385-audit` for exact skip inventory.
- Three existing subagents are auditing skip inventory, multi-profile CI design, and false-green invariants using only current source/config/live CI.

## Blocker

- A single Toast build/runtime profile cannot be assumed to execute mutually exclusive capability and option cases. The 138 skipped node IDs must be classified before changing the check to require zero skips.
- Known Toast behavior gaps and unsafe/exit-only cases may require either alternate profiles, harness support, or removal from the conformance contract; silently forcing them is not acceptable.

## Next action

1. Parse the current JUnit artifact into exact skipped node IDs and source conditions.
2. Reconcile subagent classifications into executable, alternate-profile, harness-debt, and oracle-gap groups.
3. Add a red fail-closed coverage contract for zero unexecuted cases and exact node identity across Toast profiles.
4. Implement the smallest harness/workflow changes that make the contract genuinely green, then run focused units, collection proof, actionlint, and live Actions.
5. Publish a narrowly scoped draft PR with literal `Closes #17`; do not merge without independent approval, required checks green, normal-merge audit, and two-parent verification.

This file is an untracked working note and must never be staged, committed, or pushed.

## Progress checkpoint: crypto skip repair

- Exact skip audit artifact proves 138 unique skips: 81 literal and 57 conditional.
- Execution classes: 48 repairable on current Toast profile, 59 alternate profile, 31 known pinned-Toast failures.
- Minimal nonduplicating profile design is default 64-bit/outbound-on full run, 32-bit/outbound-on for 27 width cases, and 64-bit/outbound-off for 4 runtime-option cases.
- Coverage must be keyed by full YAML path plus expanded case name; aggregate JUnit totals are insufficient.
- Open issue #25 and draft PR #26 address the first 12 current-profile cases.
- Managed red for #25: 12 selected and 12 skipped.
- After removing stale platform skips, pinned WSL Toast passed 11 cases and showed `$2b$` returns `*0`; the case now asserts that exact oracle behavior.
- Managed green for #25: 12 selected and 12 passed with zero skips.
- Harness units: 102 passed. Packaged collection remains 11,566.
- PR #26 head `1702521` has two commits: source expectations, then reviewed skip baseline 138 to 126. Required full Toast check is running.
- Separate clean issue #14 and permission-skip worktrees are active; subagents own fail-closed requirement semantics and the 20 permission setup repairs.

## Progress checkpoint: first merged repairs and profile proof

- PR #26 normal-merged as `b5f9184e2ec37f20ba57b3cfdfe8db201b64aeb2`; verified two parents. Issue #25 closed.
- Its live required Toast run: 11,566 collected, 11,440 passed, 126 skipped, zero failures/errors.
- Issue #27 and draft PR #28 repair all 20 owner/wizard permission skips with isolated wizard fixture ownership, programmer assertions, and wizard cleanup.
- PR #28 branch has managed red 20 skipped, intermediate red 3 fixture failures, and final managed green 20 passed/zero skipped. Its workflow baseline is 126 to 106; full Toast check is running.
- Issue #14 implementation now closes skip grammar, enforces builtin/feature/min-version/config requirements, fails closed on probe errors, and authorizes only exact declared skip reasons.
- Live 32-bit profile investigation found `-DONLY_32_BITS=ON` does produce a semantic 32-bit Toast through generated `config.h`, but the existing `typeof(2147483648) == INT` probe falsely reports 64-bit because 32-bit Toast still parses/wraps the literal as INT.
- The candidate width probe is `(~0 >> 31) > 1`: expected false on 32-bit and true on 64-bit. Managed profile verification is pending the combined #14/#16 branch stabilization.
- Toast CMake must run from inside the build directory so `version_options.h` is generated there. `cmake -S/-B` fails later at compile. Local WSL lacks Ninja; Make-only local build proved the source shape while live CI continues to install/use Ninja.
- The #14 branch is being extended to close #16 by deleting the duplicate ordinary-pytest implementation, using one canonical owner, and introducing full-path case identities. No compatibility wrapper should land.
## 2026-08-04 continuation checkpoint

- PR `MongooseMoo/moo-conformance-tests#28` was technically audited, marked ready, and normal-merged with `gh pr merge 28 --merge`.
- Merge commit: `37c18bcbd460209c3a9fdda1e02798f44daa3091`; verified parents are `b5f9184e2ec37f20ba57b3cfdfe8db201b64aeb2` and `91e97393b1f5034d9a573543154d4e7696bf8f03`.
- Issue #27 is closed. Required pinned-Toast CI now collects 11,566 cases and reports 11,460 passed / 106 skipped / zero failures or errors.
- Combined #14/#16 work has one execution owner and byte-identical canonical collection through ordinary pytest and `python -m moo_conformance`: 11,566 full-path case IDs, SHA-256 `155948712212ffd6fb1edd85dd647d3f885fa9727b9f9fc7fb21dafaa01cc26f`.
- Blocker: its candidate 64-bit probe `(~0 >> 31) > 1` is unsafe on the managed 32-bit Toast profile. A managed run returned `None`, then the server connection aborted with WinError 10053; the loose selector also admitted minint cases, so causality is not yet isolated.
- The #14/#16 worker is replacing the probe and must prove the replacement against both pinned 64-bit and 32-bit Toast using only the managed harness and narrowly selected canonical cases.
- Remaining work after #14/#16: merge current main into the branch, prove both entrypoints and profile behavior, then implement an immutable Toast profile matrix plus an exact full-path execution ledger whose union equals canonical collection. Aggregate totals alone are insufficient.
- Current skip surface includes repairable fixture/harness debt, alternate-build/profile cases (32-bit, outbound-off, optional compile/runtime features), and source-declared pinned-Toast failures. Literal skips cannot satisfy the deliverable that every conformance case body executes.
- Next action: await the safe profile-probe proof and task-local skip repair; independently review their diffs, then deliver audited PRs before building the final profile/ledger CI gate for issue #17.

## 2026-08-04 task-local and probe checkpoint

- Issue #29 created: `https://github.com/MongooseMoo/moo-conformance-tests/issues/29` (`Execute all task-local cases against Toast`).
- Task-local baseline was 20 collected / 13 passed / 7 literal skips. The repaired suite passed three consecutive managed pinned-Toast runs at 20 passed / zero skipped or failed (18.14s, 18.15s, 18.19s).
- The repair uses real temporary objects/verbs, isolated secondary connections for the long suspend and self-kill cases, observable marker properties, and unconditional cleanup. The added but unused `toliteral` requirement was removed after review.
- Task-local branch commit `b766fb2` contains only `task_local.yaml`; it was then updated from merged main with a normal merge commit. Workflow accounting is changed separately from 106 to 99 skips but is not yet committed.
- Post-merge focused schema/selector gate passed in WSL: 54 passed. Windows `uv` could not replace the WSL-created `.venv/lib64` symlink (`Access is denied`); this was an environment-only failure before test execution, so validation continues from WSL without force-deleting the path.
- Combined #14/#16 replaced the unsafe shift probe with the fail-closed `ONLY_32_BITS` option query. Managed 32-bit and 64-bit Toast runs showed inverse pass/authorized-skip behavior on narrow cases. Focused 106, units 153, Ruff, mypy, diff, and exact 11,566-ID entrypoint parity are green; an independent adversarial audit is in progress.
- Next action: finish task-local collection/diff validation, commit its exact skip accounting, push PR with `Closes #29`, and require live pinned-Toast CI before normal merge. Then update/reverify the combined #14/#16 branch against current main.

## 2026-08-04 profile and PR30 checkpoint

- PR #30 is open as a draft at `https://github.com/MongooseMoo/moo-conformance-tests/pull/30`, head `9e17832e40c1350b9246a8cca89f1e8c40104fb9`, with literal `Closes #29`. Required live pinned-Toast `Full suite` passed in 5m27s; independent artifact/scope verification is in progress.
- Exact pinned WSL Toast source `/root/src/toaststunt` is clean at `aecc51e9449c6e7c95272f0f044b5ba38948459e` aside from untracked local build artifacts.
- Minimal configurable Toast matrix is the existing 64-bit/outbound-on profile plus one enhanced alternate build: `ONLY_32_BITS=ON`, jemalloc enabled, compiler macros `BACKGROUND_TEST` and `SAVE_FINISHED_TASKS=15`, runtime outbound off.
- The enhanced alternate profile makes 39 current cases runnable: 27 integer-width, 4 outbound-off, 4 background-test, 3 malloc-stats, and 1 finished-tasks presence case.
- Standard outbound-on Toast should own five connectable-listener cases, but their current failed capability probe still requires managed runtime diagnosis.
- Fifteen alternate-classified cases are not solvable by profile flags in pinned Toast: 5 read_stdin (hardcoded example extension off), 3 literal finished_tasks cases, 1 absent file_version builtin, 3 absent PROMOTE_NUMBERS semantics, and 3 ephemeral-listen cases caused by listener port 0 not being replaced with the kernel-assigned port.
- #14/#16 remains NO-MERGE after adversarial review: source advertised `pytest tests/` excludes the sole packaged owner, option result shapes are not closed, and admission probes inherit prior session identity. A corrective worker is proving/fixing all three without restoring a duplicate owner.
- A separate worker is repairing the eight remaining fixture-solvable literal skips on current main using managed pinned Toast.
- Next action: merge PR #30 only after independent MERGE verdict and immutable-head audit; then finish #14/#16 fixes and current-main integration before starting the exact profile execution ledger.

## 2026-08-04 adversarial hardening checkpoint

- PR #30 first went green at `9e17832`, but independent review correctly found that the long-suspend case observed only the secondary socket and left an uncaptured 10-second child task alive after the case.
- Head `7fd93cd` added object-local worker/child/result/done markers, primary-connection result assertions, explicit child termination, and cleanup task termination. A second review then found that partial marker-property setup could make cleanup abort before recycle.
- Current PR #30 head `b45a7b1` guards child and worker property reads independently and runs recycle in its own final guarded block. Three managed pinned-Toast stress runs at this exact tree passed 20/20 in 19.52s, 19.63s, and 19.86s. A fresh required full-suite run and independent JUnit audit are pending; prior green artifacts do not authorize merge.
- #14/#16 now uses the actual root pytest command, retains one packaged execution owner, explicitly selects wizard for capability admission, propagates dynamic probe errors, accepts exact option shapes `ON`/`OFF`/`#-1`, rejects malformed successful shapes, and uses `ONLY_32_BITS` for width detection.
- Managed pinned Toast now executes connectable-listener and ephemeral-listen selectors that previously skipped. Corrective gates report 174 units, exact 11,566-ID entrypoint parity, Ruff/mypy/diff green. A new independent adversarial audit is running before commits.
- The remaining eight fixture-solvable literal skips initially passed 8/8 on managed Toast across seven YAML files. Review found failure-path isolation gaps: suite teardown is currently never called, several statement-local resources recycle only on success, anonymous fixture construction can fail before capture, and OOB cleanup can abort after partial setup.
- The eight-case worker is converting those fixtures to per-test rollback/cleanup and must re-earn the exact managed 8-case gate. No branch has been committed or pushed.
- Next action: finish/merge PR #30 if its latest head earns MERGE; accept and split #14/#16 into logical commits only after adversarial approval; then publish the hardened eight-case literal-skip PR.

## 2026-08-04 PR30 merge gate checkpoint

- Independent verdict for PR #30 is MERGE at exact head `b45a7b1e5efe131a5805aa896ae204e57fc71932`.
- Required `Full suite` run `30972774895` passed against that head. Independent JUnit audit: 11,566 collected, 11,467 passed, 99 skipped, zero failures/errors.
- All seven repaired task-local IDs occur exactly once and pass; the full task-local surface is 22/22 with zero skips.
- Final scope is only `task_local.yaml` plus exact Toast skip accounting; worktree/diff are clean and PR body retains literal `Closes #29`.
- Final isolation proof records and terminates child/worker tasks, asserts completion/result from the primary connection, and guards partial-setup cleanup so recycle remains an independent final action.
- Merge precondition: mark draft PR ready, re-check unchanged head/check/base/files/commits/body, execute only `gh pr merge 30 --merge`, then verify the merge commit has two parents and issue #29 closes.

## 2026-08-04 post-PR30 and final admission checkpoint

- PR #30 was normal-merged as `d89ac408c37420f542f712af92824f8da9e43eea`; verified parents are `37c18bcbd460209c3a9fdda1e02798f44daa3091` and `b45a7b1e5efe131a5805aa896ae204e57fc71932`. Issue #29 closed. Main-branch Toast run `30973126935` passed.
- Issue #31 created: `https://github.com/MongooseMoo/moo-conformance-tests/issues/31` for the remaining eight fixture-solvable literal skips.
- Issue #31 branch remains uncommitted. Its OOB fixture now includes an ordinary asserted restoration step; fallback cleanup retains observation/recovery artifacts unless restoration is proven. Managed exact selector is again 8/8, pending independent final source verdict and later required PR CI.
- #14/#16 fixed digit-leading requirement names, exact literal skip reasons, and current exact assumes reasons. Adversarial rerun found one final false authorization: literal reasons were added but unreachable skip_if/requirement reasons remained whitelisted even though literal skip executes first.
- Minimal precedence fix now applied: `declared_runtime_skip_reasons()` immediately returns only the winning literal reason. Added regression rejects an unreachable `Requires feature: maps` skip on a literally skipped case. Assumes remains separately authorized at report time because its setup hook can preempt the call.
- No #14/#16 files are staged or committed. Next action: run focused/full gates for the precedence fix, obtain adversarial MERGE, split #14 and #16 into logical commits, update from current main, then run full managed Toast before setting exact workflow accounting.

## 2026-08-04 issue31 and execution-ledger checkpoint

- #14/#16 final source verdict is MERGE after literal precedence fix. Current local gates: 144 focused passed, 192 full units passed, Ruff clean, scoped mypy clean with ephemeral types-PyYAML, diff check clean, and exact 11,566 ordered unique ID parity.
- Issue #31 source verdict is MERGE. YAML repair commit `5f3d151` contains exactly seven files; merge commit `7b8ccc1` updates the branch from current main `d89ac408`. Workflow skip accounting is changed from 99 to 91 but not yet committed.
- Managed post-merge loose selector passed 11 cases, not exactly eight: substring collisions added `renumber_basic_simple` and two other `setadd_checks_list_max_value_bytes*` cases. All 11 passed, but this is not exact eight-case identity proof. Required full PR CI plus independent JUnit identity audit remains authoritative; future focused proof should use canonical full-path IDs after #14/#16 lands.
- Issue #17 design requires a custom canonical-ID execution ledger; JUnit remains diagnostic. A case counts as executed only with setup/call/teardown all passed and no xfail marker. Deselection, incomplete phases, literal/conditional skips, xfail/xpass, probe failures, malformed evidence, and mixed profile revisions fail closed.
- Workflow sequencing: merge #14/#16, merge #15 lifecycle, then add closed Toast profile config, per-profile evidence, exact skip baselines, and union aggregation. Quality and union become stable required checks.
- A complete width/outbound matrix is preferred for construction guarantees. Pinned Toast cannot make literal skips or PROMOTE_NUMBERS cases execute; those semantic gaps must be repaired before the union gate can pass.
- Next action: commit issue #31 accounting, open its draft PR with `Closes #31`, require full live Toast/JUnit; meanwhile split and commit approved #14/#16, merge current main, and run its full managed Toast surface to derive exact accounting.
