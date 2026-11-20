# Command-line fuzz target

## Goal
Fuzz the command parser, which every connection reaches, including before
login (server/input_login.go:60 calls command.CommandWordList). Part of the
fuzzing expansion Q approved 2026-11-01 (plan in barn master checkout
agent-notes/fuzzing-expansion.md).

## State
command/fuzz_test.go: FuzzCommandLine runs CommandWordList,
ParsePlayerCommand (object matching), HasVerbNameMatch and FindVerb as Test.db
player #3 in room #2. Invariant: no Go panic. Local campaign, 3 minutes,
parallel 2: 1,312,853 execs, 267 interesting inputs, PASS. Docs row and
campaign command added to docs/parser-fuzzing.md.

## Decisions and why
- Test.db world rather than hand-built objects: real verb names, wildcards and
  argument specs exercise the matcher as production does.
- One store per process: matching is read-only.
- No runtime-cost bound here; per-input time bounds belong to the
  compile-time-limits piece.

## Next step
PR CI; then the nightly workflow includes this target.
