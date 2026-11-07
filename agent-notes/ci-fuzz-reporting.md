# Fuzz reporting

## Goal
Improve nightly failure evidence, signature deduplication, and automated repair attribution and gap reporting (approved plan A+B).

## State
Implemented log capture, Python JSON reporting, signature-based issue matching, infrastructure dispatch exclusion, and repair settings/prompt updates. Preserved four raw run-log fixtures. Focused Python tests and actionlint passed. PR #612 is open against master. The implementation passed Build & unit tests, MOO conformance suite, and Serializable histories and negative control. Current-head checks remain visible on the PR; this final note records those completed implementation checks without changing executable files.

## Decisions and why
Keep changes in reporting and repair workflows plus one Python reporter and fixture tests. No runtime behavior changes. Publish a standalone PR against master; do not merge or post comments, reviews, or issues.

## Surprises
The staged raw logs contain trailing whitespace from upstream output. Preserve it as captured evidence. The pre-staging diff whitespace check excluded untracked logs; the PR body now distinguishes that result from the staged check's reported whitespace.

The self-hosted runner does not put Node on shell PATH. Offline github-script tests use Node from PATH where present, otherwise the runner's existing externals/node24 runtime located from RUNNER_TEMP. Concurrent WSL checks passed using that runtime; the PR's Python test step also passed.

Run 42009421620 exceeded the initial tool output limit. Re-fetched it in full and added a test rejecting tool truncation markers. Real logs show panic, unexpected termination/hang wording, and signal termination; ordinary failed checks and setup errors use synthetic tests. Go's hang wording does not itself prove a timeout. No Barn frames exist in the captured hang output; the signature records unavailable rather than inventing a frame.

## Next step
Leave PR #612 open for owner review after current-head checks finish. Do not merge. No runtime Go package changed, so package fuzzing and Go-only local lint commands do not apply. The existing CI runs the full repository checks. Live issue creation, repair dispatch, and generated attribution text remain untested; do not post test issues or dispatch repairs.

## Source evidence
Installed Go 1.27.1 testing/fuzz.go prints the failing-input path and replay selector; internal/fuzz/worker.go and sys_posix.go distinguish worker termination and non-crash signals. Fixture README records exact locations.

The pinned claude-code-action action.yml accepts settings as a JSON path; base-action/src/setup-claude-code-settings.ts loads and merges that file. Its base-action/action.yml installs Claude Code 2.1.318. The settings reference documents attribution.commit and attribution.pr empty strings and attribution.sessionUrl false; includeCoAuthoredBy is deprecated. Use attribution to disable each output explicitly.
