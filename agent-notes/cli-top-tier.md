# Top-tier CLI for every cmd/ binary

## Goal
Every binary in `cmd/` gets GNU-style flags (`--long`, short letters where they earn it), `--version` built from the git SHA, and the same help style. `barn`'s one-shot modes become subcommands. Q, 2026-11-15: "where's version? (from git sha of course?) where's verbosity/logging controls? ... I hate -promote-numbers instead of --promote-numbers".

## Decisions (Q, 2026-11-15, question tool)
- Modes become subcommands, built with cobra v1.10.2 (pflag v1.0.9).
- Old single-dash spellings stop working. Every caller in barn and moo-conformance-tests is updated in the same change. Q updates any launcher outside these repos (production Mongoose) before the merge.
- All seven tools get `--version`/`-V`, GNU parsing and the same help style.
- `-v`/`-q`/`--log-level` only on barn and moo, the two that emit slog records (moo through `db/format` load warnings). The other five log nothing (checked in `cmd/*/main.go`).

## Design
- `internal/buildinfo`: `Info.Time` (vcs.time), `Info.GoVersion`, `Info.Line(name)`. Done, tested.
- `internal/cli`: `NewRoot(name)` wires `--version`/`-V` via cobra's own version check (cobra skips its default `-v` shorthand when a `version` flag already exists). `LogFlags` adds `-v`, `-q` (count; `-qq` = error), `--log-level`. `Execute` returns exit codes: 2 for usage errors, 1 otherwise, or a code a command asks for.
- `barn`: each subcommand fills `app.Config` and calls `app.Run`, so `internal/app` stays the only implementation. Bare `barn` prints help.
- `mongoose_tz` stays positional (the Mongoose database runs it as `tz <zone>`); its missing-argument output stays byte-identical.

## Caller inventory (to update)
- barn repo: `scripts/bench_differ.py:162-163`, `scripts/measure-mongoose-admission.py:127-131`, `scripts/mongoose-login.ps1:89,145,155`, `scripts/benchmark-mongoose.ps1:129-136,164`, `scripts/run-conformance.ps1:63`, `scripts/check-conformance.sh:29`, `scripts/check-map-conformance.sh:19`, `scripts/bench_mongoose_matrix.py:192`, `scripts/bench_players.py:787`, `scripts/mongoose_barn_process.py:323`, `scripts/capture-mongoose-debug.ps1:40,46`, `scripts/fuzz_differential.py:165`, `bench/run_bench.sh:35`, `.github/workflows/ci.yml:185,213,235`, `profiles/barn/profiles.json` command templates, `README.md`, `CLAUDE.md`, `docs/TRACING.md`, `spec/grammar.md:483`, `tests/jepsen-checkpoint/README.md`, Go tests in `cmd/*`.
- moo-conformance-tests: `.github/workflows/paired-candidates.yml:296,329`.

## State
Worktree `../barn-cli`, branch `cli/gnu-subcommands` from master.
- Done and tested: `internal/buildinfo` (Line), `internal/cli` (`cli_test.go`), `cmd/barn` (`command_test.go`: version spellings, eval/--file, exit 1 vs 2, bare barn prints help, list-profiles).
- `internal/app`: messages now say `--port`/`--listen`; removed the `--omit-suspended`/`--memory-profile` dependency checks, which no command line can trigger now that those flags live only on `offline-checkpoint` and `memory-report` (`--profile`).
- Logging is set up only inside commands that do work, so `barn version`/`--help`/`completion` never rotate `logs/latest.jsonl`.
- Converted and tested: `moo` (`-e, --eval`, `--db`, -v/-q), `barn_logs` (`-l, --level`, `-n, --last`, rest `--long`; read failures still exit 2, unknown level is now a usage error), `moo_client` (`-c, --cmd`, `-p, --port`, `-f, --file`), `toast_oracle` (missing `--bin`/`--db` now exits 2, was 1; no caller depends on it).
- `moo-fuzz-source` (`--corpus`) and `mongoose_tz` (positional zone; missing-zone text and exit 0 kept, tested) converted.
- Callers updated in barn: every script in the inventory, `ci.yml` (3 server commands), `profiles.json` (8 templates), fuzz fixture README.
- Makefile conformance command and docs updated (README, CLAUDE.md, AGENTS.md, docs/TRACING.md, docs/mongoose-conformance.md, docs/workload-diagnostics.md, spec/grammar.md). `tests/jepsen-checkpoint/README.md` uses the jepsen script's own `--barn`/`--db` flags, unchanged.
- Next: full gates (gofmt, vet, staticcheck, golangci-lint, go test ./..., python unittest), commit, PR; then moo-conformance-tests PR.

## Surprises
- Stdlib `flag` already accepts `--x`; only help output and docs spelled `-x`.
- pflag names a flag's placeholder from a backquoted word in its usage text (`"Listener `URL`"` shows `--listen URL`); without one it shows the Go type (`stringArray`).
