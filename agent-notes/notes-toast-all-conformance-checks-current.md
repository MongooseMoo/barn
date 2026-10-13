# Current Toast conformance check repair

- Scope: live CI/code only; no historical project Markdown was read.
- Current `moo-conformance-tests` main is `a46fa56603451cff34533ffc0b0f104dab5bdf1f`.
- `.github/workflows/toast-conformance.yml` already invokes `pytest --pyargs moo_conformance` against the pinned Toast commit, with strict markers and fail-on-unexpected-skip.
- The workflow's completeness gate only asserts aggregate JUnit totals (`11,566` tests and `459` skips). Aggregate counts cannot prove that every canonical conformance identity ran; replacement or duplication can preserve the totals.
- PR #22 currently collects `11,569` packaged tests, so the hard-coded main total also makes suite growth require manual count edits rather than exact surface accounting.
- Current task: change checks so completeness is established from the collected/executed canonical test identities against Toast, with failures for missing or duplicate identities and unexpected skips.
- Preserve unrelated dirty worktrees and keep this note uncommitted.
- Separate blocker discovered in issue #15 review: lifecycle changes are NO-MERGE due scalar cleanup omission, failed-user restoration poisoning, and lack of randomized-order proof. This is not part of the Toast-check change.
- Next action: inspect current pytest/plugin reporting hooks and implement the narrow CI/accounting fix in a clean conformance worktree, then run unit/collection checks and the documented managed Toast command.

## Live skip inventory

- Downloaded artifact from current-main Toast run `30976490813`: 11,566 JUnit cases, 459 skipped, no duplicate JUnit identities.
- Major skips incorrectly keep supported Toast behavior out of the check: `index_extensions` 130, `maps` 127, `anonymous` 55, `fork` 43, `exec_support` 18, `command_dispatch` 4. `fork` is a language feature, not a builtin, so `function_info("fork")` is the wrong admission check.
- Variant-dependent skips: 27 cases require a 32-bit Toast build, and 4 require runtime outbound networking disabled. Current CI builds only 64-bit/outbound-on.
- Smaller optional/unsupported categories: anonymous GC 24; `read_stdin` 5; `background_test` 4; `SAVE_FINISHED_TASKS` 3 plus one builtin-presence case; `PROMOTE_NUMBERS` 3; `malloc_stats` 3; anonymous task-stack 3; wildcard verbs 3; `file_version` 1; `tostr_limit` 1.
- Pinned Toast source exposes CMake `ONLY_32_BITS`; outbound is a runtime `--outbound`/`--no-outbound` choice. `SAVE_FINISHED_TASKS` is a commented compile define. No `PROMOTE_NUMBERS` implementation was found in the pinned source.
- Clean implementation branch/worktree: `agent/issue-17-run-all-toast` at current `origin/main` `a46fa566`; no uncommitted changes.
- Next action: implement exact execution ledgers and a Toast profile matrix, and replace incorrect feature/builtin gates with proven behavioral/capability admission so supported cases execute.

## Implementation in progress

- Dedicated worktree remains `C:\Users\Q\code\.worktrees\moo-conformance-issue-17`, branch `agent/issue-17-run-all-toast`, based on current main.
- Added a fail-closed `execution_ledger` module and unit tests. It parses exact canonical IDs from every profile JUnit file, rejects duplicates, missing/unknown identities, failures/errors, unreviewed never-executed cases, skip-reason drift, and stale baseline entries.
- Focused ledger/requirements proof is green: 75 passed. Initial Ruff found one long line; it was corrected and needs rerun.
- Extended profile capability metadata to cover `feature.<name>` booleans, with tests for true, false, and malformed values. Reclassified `fork` from a nonexistent builtin to a language feature in the two affected suites.
- Reworked the live Toast workflow into four profiles: 64/32-bit crossed with outbound on/off. Each builds pinned Toast with optional executable/read-stdin/background/finished-task/jemalloc support enabled, records the exact profile, runs the full packaged suite with strict skips, and uploads JUnit evidence.
- Added an aggregator check named `Every packaged case executed`; it downloads all four reports and enforces their exact execution union against `ci/toast-never-executed.json`. The baseline is intentionally empty until profile evidence proves which cases the pinned Toast commit truly cannot execute.
- The managed issue-15 Toast run could not be interrupted by the process backend and is still running separately; no manual server was launched.
- Next action: validate workflow/YAML and all unit gates, finish the managed issue-15 process, run the four managed Toast profiles, then repair surfaced cases and populate only exact unavoidable baseline entries.

## Fail-closed quality and profile state

- Independent current-source audit reconciled all 459 original skips exactly. Maximal Toast profiles can execute all but 40 cases: 31 literal pinned-Toast incompatibilities, 3 unavailable `PROMOTE_NUMBERS` cases, 5 source-disabled `read_stdin` cases, and 1 absent `file_version` case. These 40 exact IDs/reasons now form the reviewed never-executed baseline.
- The minimal union is three profiles, not four: 64-bit/outbound-on, 64-bit/outbound-off, and 32-bit/outbound-on. The redundant 32-bit/outbound-off profile was removed.
- Optional Toast support is compiled into every CI profile with `BACKGROUND_TEST`, `SAVE_FINISHED_TASKS=15`, and jemalloc. The three `finished_tasks` literal skips were removed so those cases execute. `read_stdin` cannot be enabled without changing pinned Toast source because `EXAMPLE` is hard-defined to zero.
- Added required quality job: harness units, packaged collection, exact duplicate baseline, Ruff, and mypy.
- Added exact duplicate-baseline behavior and tests. Current reviewed baseline is 45 duplicate-name groups and 28 duplicate-content groups; any new, changed, or stale group fails. Exact baseline command passes.
- Repository-wide Ruff passes. Mypy passes all 18 source files after narrow typing fixes and only targeted missing-stub overrides. Focused duplicate tests pass.
- The prior issue-15 full Toast run completed independently with 228 failures caused by that branch's lifecycle contamination; this confirms its NO-MERGE status and does not affect issue-17 work.
- Current managed canonical Toast feature-surface proof for the issue-17 branch is still running under session `65274`; no manual server was launched.
- Next action: collect that managed result, fix any newly executed feature-suite failures, run complete local quality/collection, then push the issue-17 branch for the three-profile GitHub matrix and exact union ledger.

## PR and current verification

- Created two commits on `agent/issue-17-run-all-toast`: `681c75b` (quality gates/baselines) and `4603e95` (Toast profile execution/ledger).
- Opened draft PR #34: `https://github.com/MongooseMoo/moo-conformance-tests/pull/34`, literal `Closes #17`, exact two-commit head initially `4603e956aeb69521f87da54863041e6dca49220b`.
- Local gates after all implementation: 222 units passed; 11,566 packaged cases collected; duplicate baseline exact; Ruff green; mypy green across 18 source files; actionlint green; diff check green.
- Managed 3,468 selected feature-surface proof completed: 3,448 passed, 17 declared/selection-related skips, 3 failures. Failures were global-order assumptions: GC asserted absolute global counters; exec cases asserted exact whole-queue length/order. Head now compares GC deltas and filters queue membership by owned task IDs.
- Attempted exact isolated managed rerun using canonical WSL Toast. It is blocked before Toast launch: pytest PID 25616 remains in WSL `p9_client_rpc` against `/mnt/c`, so no isolated result was produced. Exact command/workdir/expected/actual were reported to the user; no manual server or alternate oracle was used.
- PR #34 GitHub workflow run `30979498360` began all three Toast matrices plus quality in parallel.
- Live branch protection requires context `Full suite` with strict mode and admin enforcement. The new aggregator was initially named `Every packaged case executed`; changed it locally to retain the protected `Full suite` context while replacing its semantics with exact profile-union accounting. This one-line change still needs commit/push and will supersede the initial run.
- After merge, required contexts should be `Full suite` (the three-profile union) and `Harness, schema, duplicates, types, and lint`; direct matrix contexts need not be separately required because `Full suite` depends on all of them and fails on missing artifacts/results.
- Independent adversarial review of exact PR head is running; read-only/no server.
- Next action: commit/push the protected-context rename, wait for the superseding CI run, fix any exact profile failures, obtain adversary and verifier MERGE verdicts, then mark ready, audit, normal-merge, verify two-parent merge and issue closure, and update branch protection contexts.
