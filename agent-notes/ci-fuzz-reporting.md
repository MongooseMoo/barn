# Fuzz reporting

## Goal
Improve nightly failure evidence, signature deduplication, and automated repair attribution and gap reporting (approved plan A+B).

## State
Implemented log capture, Python JSON reporting, signature-based issue matching, infrastructure dispatch exclusion, and repair settings/prompt updates. PR #612 is open against master; do not merge.

Review repair: saved failing inputs now override infrastructure classification; saved signal failures use the crash signal kind. Added synthetic Go-source-derived crash/non-crash signal tests, saved-input precedence coverage, and an offline repair-dispatch regression. The four raw log fixtures remain unchanged. Focused reporting tests passed on Windows. Full Python discovery passed in WSL Debian, and actionlint passed for both affected workflows. Push and current-head PR checks are next.

## Decisions and why
Keep changes in reporting and repair workflows plus one Python reporter and fixture tests. No runtime behavior changes. Publish a standalone PR against master; do not merge or post comments, reviews, or issues.

Installed Go 1.27.1 internal/fuzz/worker.go:170-176 excludes non-crash signals from recorded crashes; :186 formats unexpected worker termination for the crash result. sys_posix.go:116-128 includes SIGSEGV among crash signals. Saved-input evidence takes precedence over signal/setup/build markers and campaign outcome. Hangs have no stack frames in the captured logs, so distinct hangs on one target share a signature while that target's issue is open (no worse than the old title match).

## Surprises
The staged raw logs contain trailing whitespace from upstream output. Preserve it as captured evidence. The pre-staging diff whitespace check excluded untracked logs; the PR body now distinguishes that result from the staged check's reported whitespace.

The self-hosted runner does not put Node on shell PATH. Offline github-script tests use Node from PATH where present, otherwise the runner's existing externals/node24 runtime located from RUNNER_TEMP. Concurrent WSL checks passed using that runtime; the PR's Python test step also passed.

Run 42009421620 exceeded the initial tool output limit. Re-fetched it in full and added a test rejecting tool truncation markers. Real logs show panic, unexpected termination/hang wording, and signal termination; ordinary failed checks and setup errors use synthetic tests. Go's hang wording does not itself prove a timeout. No Barn frames exist in the captured hang output; the signature records unavailable rather than inventing a frame.

## Next step
Commit named reporting/test/note paths, push, and watch PR #612 checks; fix red checks. Do not merge. No runtime Go package changed, so package fuzzing and Go-only local lint commands do not apply. Live issue creation, repair dispatch, and generated attribution text remain untested; do not post test issues or dispatch repairs.

## Review repair verification
- Windows `python -m unittest discover -s scripts -p 'test_*.py'`: failed (8 failures, 29 errors, 26 skips), including Linux-only process tests requiring os.pidfd_open. No unrelated production or test files changed.
- Windows `python -m unittest discover -s scripts -p 'test_fuzz_report.py'`: OK.
- WSL Debian `python3 -m unittest discover -s scripts -p 'test_*.py'`, with the existing runner Node runtime on PATH: OK.
- WSL Debian `actionlint .github/workflows/fuzz.yml .github/workflows/claude-fix.yml`: exit 0, no diagnostics.

## Source evidence
Installed Go 1.27.1 testing/fuzz.go prints the failing-input path and replay selector; internal/fuzz/worker.go and sys_posix.go distinguish worker termination and non-crash signals. Fixture README records exact locations.

The pinned claude-code-action action.yml accepts settings as a JSON path; base-action/src/setup-claude-code-settings.ts loads and merges that file. Its base-action/action.yml installs Claude Code 2.1.318. The settings reference documents attribution.commit and attribution.pr empty strings and attribution.sessionUrl false; includeCoAuthoredBy is deprecated. Use attribution to disable each output explicitly.
