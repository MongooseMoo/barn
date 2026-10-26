# #536: shared player-list reader

## Goal
Remove duplicate player-list parsing while preserving the supported versions' explicit top-level sequences.

## State
Implemented on merged base 2b6eaab1cece082d91522a4def543bc860b6de8e. One unchanged shared body replaces both version-named readers; all three parser callers and existing direct-helper tests use it. Final five-Go-file diff inspected; production code has 23 net deleted lines. No local tests or CI commands run.

## Decisions and why
Smallest change: move one unchanged body to reader_helpers.go as readPlayers; update the v4/v5/v17 callers; remove both version-named definitions and v17's now-unused types import. Collapse the two identical direct test invocations to one shared-reader invocation. Approximately 25 net removed production lines; no new tests, wrappers, dispatch machinery or behavior.

## Surprises
None.

## Next step
Parent reviews the scoped local commit, publishes the PR and reads existing CI. No test-pass claim is made.

## PR draft
Title: refactor(db/format): share player-list parsing across supported versions

The v4/v5 and v17 paths used identical player-list readers. Move the existing body into readPlayers, update all three callers and remove the duplicate definitions. Existing direct-helper tests now exercise the shared owner once; version-specific parser sequences remain explicit.

Validation: inspected the final diff and caller inventory. No local test run; existing PR CI is pending. Closes #536.
