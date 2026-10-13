# Toast conformance CI checkpoint — 2026-08-04

## Current state

- PR #19 is reviewed and locally green but its merge is paused because the repository has no GitHub Actions workflows or branch protection.
- A clean worktree at `C:\Users\Q\code\.worktrees\moo-conformance-toast-ci` is on `agent/ci-full-toast` from current remote main.
- The uncommitted draft workflow is `.github/workflows/toast-conformance.yml`; `actionlint` passes.
- The draft pins Toast oracle commit `e8a353665a106244f5e01edb67239c90411ae584`, the current `json-v20-db` oracle commit, and pins all Actions by immutable SHA.
- Prior managed Toast JUnit evidence proves 11,566 total cases, 149 skips, zero failures, and zero errors.

## Findings

- The draft must add `--fail-on-unexpected-skip`; aggregate totals alone can false-green if unexpected skips replace expected skips.
- Fail-closed collection from issue #10 is required for candidate trust. Because PR #19 is not merged yet, CI bootstrapping must either incorporate the already reviewed issue #10 commit or land CI first and immediately update PR #19; a candidate can otherwise mutate both tests and totals.
- The canonical Toast command should include explicit IPv4/outbound flags and the startup fixture directory.
- Artifact capture needs diagnostics beyond JUnit and must not obscure a pre-pytest failure.
- A new pull-request workflow cannot prove itself through the ordinary `pull_request` event until the workflow exists on default main; live proof needs an authorized branch event or merge bootstrap followed by a main/PR run.
- Repository Actions defaults to write, but the workflow explicitly restricts permissions to `contents: read` and disables persisted checkout credentials.

## Blocker

- No external blocker. The workflow needs the review findings corrected and then a safe bootstrap/test path.

## Next action

- Correct strict-skip, Toast invocation, timeout, and diagnostic artifact behavior.
- Re-run static validation and independent review.
- Publish a separate CI PR, obtain live Actions proof without weakening scope, normal-merge it, require the resulting check on main, then update PR #19 and merge only after the full Toast check passes.

This file is an uncommitted execution checkpoint and must not be staged, committed, or pushed.

## Correction update

- The workflow now uses `--fail-on-unexpected-skip` and supplies `--server-db-dir`, so the 11 startup-repair cases that were configuration-skipped in the earlier 149-skip run will execute.
- The provisional strict baseline is therefore 11,566 total cases and 138 declared skips; the first live Ubuntu run must confirm or correct this from exact evidence.
- Toast is invoked with `--no-ipv6 --ipv4 127.0.0.1 --outbound` and the managed `{db} {db}.new {port}` lifecycle.
- Timeout is 60 minutes, uv caching is disabled, and apt/uv/build/pytest logs plus JUnit are uploaded without turning a missing report into a secondary masking failure.
- `actionlint` and `git diff --check` pass after these corrections.
- The bootstrap branch is included in the push trigger so the workflow can prove itself before it exists on default main; the branch-specific trigger should be removed in a follow-up after the gate is established.

## Live CI update

- PR #19 normal-merged as `6f2ef795f8bfbb4b8732e75d15dedbad38577ddd` with two parents; issue #10 is closed, so CI now inherits fail-closed collection.
- Suite-level skips are now collected and reported at runtime instead of disappearing during discovery; red was 2 failures, green is 20 focused and 102 unit tests.
- PR #21 closes issue #20 and preserves two commits: suite-skip accounting, then Toast CI.
- Both bootstrap push run `30962362317` and PR run `30962374415` failed during the real Ubuntu Toast build.
- Exact root cause: `src/version.cc` could not find generated `version_options.h`. Toast's CMake configuration invokes `version_opt_gen.pl`, which writes the header to the source working directory; the out-of-tree Ninja build includes the source `src/include` and build directory, not the checkout root.
- The workflow evidence upload succeeded on both failures and retained apt, uv, fixture, and build logs.
- Next action: place the generated header in an included path (or make the build use the source layout expected by this pinned Toast commit), prove the build fix locally/static, push a focused CI commit, and watch the full Toast job through its next failure or green completion.

## Merge and protected-main update

- The generated-header failure was fixed by configuring Toast from its build directory. The next live failure was limited to two spellcheck cases because Ubuntu had the Aspell library but not the English dictionary; adding `aspell-en` fixed that dependency without weakening tests.
- The final PR head `0e1e5834953aa3ebe85572c10a2a876f6b72ba15` passed live run `30963302008`: 11,428 passed, 138 declared skips, zero failures/errors, exact 11,566-case surface, and successful artifact upload.
- Branch protection on `main` strictly requires GitHub Actions check `Full suite` with app id `15368` and enforces protection for admins.
- Independent analyst and verifier both approved the final five-file, five-commit scope. PR #21 contained literal `Closes #20` and was normal-merged with `gh pr merge 21 --merge --repo MongooseMoo/moo-conformance-tests`.
- PR #21 is MERGED, issue #20 is CLOSED, and merge commit `a031527261632d262e1432344e0a5113119635de` has exactly two parents: prior main `6f2ef795f8bfbb4b8732e75d15dedbad38577ddd` and PR head `0e1e5834953aa3ebe85572c10a2a876f6b72ba15`.
- The protected `main` push run is `30963872201` at merge SHA `a031527261632d262e1432344e0a5113119635de`. Checkout, pinned Toast checkout, dependency installation, locked environment installation, all 11 fixture hash checks, and Toast build are green. The full packaged conformance step is currently running.

## Current blocker and next action

- No external blocker. Wait for `main` run `30963872201` to complete.
- Require the full case run, exact-surface assertion, and evidence upload to pass. If they do, verify the protected required check on the merge SHA and then continue the remaining conformance hardening issues.

## Protected-main result

- Run `30963872201` completed successfully in 5m02s on merge SHA `a031527261632d262e1432344e0a5113119635de`.
- The pinned Toast job collected all 11,566 cases and reported 11,428 passed, 138 declared skips, zero failures, and zero errors in 249.99s.
- The exact-surface gate reported `tests=11566`, `failures=0`, `errors=0`, and `skipped=138`; all 11 fixture hash checks and evidence upload passed.
- The merge SHA has exactly one `Full suite` check run and it is successful. `main` protection remains strict and requires `Full suite` from GitHub Actions app id `15368`.
- Toast CI issue #20 and PR #21 are complete. Next campaign action is the remaining conformance hardening work, beginning with the next scoped issue and Toast-first expected-behavior proof.
