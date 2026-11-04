# docs/readme

## Goal
Q asked: improve the README, set repo metadata, say what was wrong with the old
one, no AI-slop prose, and don't bury readers in engineering detail.

## State
- README rewritten; PGO refresh procedure moved verbatim to docs/pgo.md.
- GitHub description and topics: set via `gh repo edit` after the PR opens.
- No LICENSE file exists; choosing one is Q's decision (asked via question tool).

## Decisions and why
- Kept the Windows `scripts/run-conformance.ps1` command: CLAUDE.md points
  readers at "the PowerShell flow in README.md".
- Dropped the `conformance/` package section: the package no longer exists.
- Dropped "every test is checked on Toast in CI": ci.yml only passes a Toast
  profile manifest; it does not run Toast.
- Unicode note verified: `bin/moo -e 'return length("é");'` gives 1.

## Surprises
- Old README said Go 1.24.6; go.mod says 1.27.1.

## Next step
- Read the PR's CI result; merge when green.
