# Verb-source fuzzing

Barn uses Go's coverage-guided fuzzer for three internal invariants:

| Target | Invariant |
| --- | --- |
| `parser/FuzzParseFormatRoundTrip` | Accepted source formats and reparses to the same semantic IR in both formatter modes, ignoring positions and case-insensitive variable spelling; canonical formatting is stable. |
| `compiler/FuzzCompileVerifiable` | Accepted compilation produces bytecode accepted by the bytecode verifier. |
| `vm/FuzzSourceExecution` | Arbitrary source can be compiled and, when accepted, initially executed without a Go panic. |
| `command/FuzzCommandLine` | Any input line passes word splitting (the pre-login path), command parsing with object matching, and verb lookup as Test.db player `#3` in `#2` without a Go panic. |

Parsing/compilation errors are normal results. Execution uses a fresh empty
store, task, core builtin registry, and session with a 100-tick budget. MOO
errors, exhaustion and suspension/fork results are normal. The target does not
resume yielded tasks, run a scheduler, or exercise optional I/O capabilities.
It does not assert MOO behavioral conformance; use the managed Toast oracle for
that. No target recovers Go panics.

Each target seeds every programmed verb in the tracked `Test.db`, including
empty programs and anonymous objects. Identical source bytes are added once.
Small syntax/boundary cases complement whole verbs. Ordinary `go test` replays
these seeds and any committed findings, without running timed fuzz campaigns.

## Import a larger local corpus

Use the existing `lambdamoo-db-py` reader to extract verbs from additional
databases. Do not copy dumps into Barn or implement a second dump reader.
The reader must include the recycled-slot/anonymous-definition repair in
[lambdamoo-db-py PR #34](https://github.com/MongooseMoo/lambdamoo-db-py/pull/34)
to read Barn's `Test.db`.

From the Barn root, using PowerShell (adjust the reader checkout and database
paths to the local installation):

```powershell
uv run --project C:/Users/Q/src/lambdamoo-db-py-recycled-anon python -B scripts/fuzz_corpus.py `
  --output .fuzz/corpus `
  --db Test.db `
  --db C:/Users/Q/src/lambdamoo-db-py/LambdaCore-latest.db `
  --db C:/Users/Q/src/lambdamoo-db-py/toastcore.db `
  --db C:/Users/Q/src/toaststunt/mongoose.db
$env:BARN_FUZZ_CORPUS = (Resolve-Path .fuzz/corpus).Path
```

The importer checks extracted program counts against each dump's declared
count and source bytes against the reader's fingerprints. `.moo` seeds retain
the stored Latin-1 bytes and LF line endings, including comments, whitespace,
editor stamps and spelling. It deduplicates exact bytes, rather than stripping
syntax or normalizing identifiers. `manifest.json` records database digests,
paths, object IDs, anonymous flags and verb indexes for every origin.

`.fuzz/` is ignored: these are local inputs and provenance, potentially
containing private world source. A source count is not a parser-coverage claim.
Keep local campaign output there too. Reusing a corpus directory retains any
older seeds already there; use a new directory for a different source inventory.

## Run a campaign

Run one target per invocation. `-run '^$'` avoids rerunning unrelated unit tests;
seed replay remains part of fuzzing. `-parallel` bounds worker count. Run these
in separate terminals to exercise the targets concurrently:

```powershell
go test ./parser -run '^$' -fuzz '^FuzzParseFormatRoundTrip$' -fuzztime=10m -parallel=2 -timeout=20m
go test ./compiler -run '^$' -fuzz '^FuzzCompileVerifiable$' -fuzztime=10m -parallel=2 -timeout=20m
go test ./vm -run '^$' -fuzz '^FuzzSourceExecution$' -fuzztime=10m -parallel=2 -timeout=20m
go test ./command -run '^$' -fuzz '^FuzzCommandLine$' -fuzztime=10m -parallel=2 -timeout=20m
```

`FuzzCommandLine` seeds its own command lines; `BARN_FUZZ_CORPUS` does not
apply to it.

Without `BARN_FUZZ_CORPUS`, the commands use the tracked baseline and boundary
seeds. Go retains interesting generated inputs in its fuzz cache and minimizes
failures automatically. Preserve the final output, including executions,
interesting inputs, elapsed time, failure details and reproduction command.

Failing inputs are written to `<package>/testdata/fuzz/<target>/`. Reproduce
one using the exact `go test -run` command Go prints. The input then replays
during ordinary PR CI. Commit confirmed reproductions with their fixes, or
attach an appropriate reproduction to a separate issue. Do not publish private
verb bodies accidentally when retaining local-corpus failures.

These targets implement [Barn #414](https://github.com/MongooseMoo/barn/issues/414).
PR CI runs no timed fuzzing. `.github/workflows/fuzz.yml` runs every fuzz
target for thirty minutes nightly, dispatched at 3:17 AM America/Denver by the
runner workstation's Windows task `Barn - Nightly fuzz` (and on manual
dispatch), two at a time, and
uploads any failing input as an artifact. A failed job opens an issue with its
run and artifact links; subsequent failures for the same target add those links
to its existing open issue. Setup failures are reported separately from failed
fuzz commands. There is no grammar generator. The useful
principles from [the AFL++ article](https://nullprogram.com/blog/2025/02/05/)
are focused assertions and per-input isolation; Go supplies the mutation,
coverage instrumentation and minimization described in its
[fuzzing documentation](https://go.dev/doc/security/fuzz/).

Install or update the nightly trigger as the logged-in runner owner with
`./scripts/install-nightly-fuzz-task.ps1`. It uses the existing authenticated
GitHub CLI and follows the workstation's Mountain timezone, including daylight
saving. Q must remain logged on, as for the Barn runner tasks. Missed triggers
are not run later in the day. GitHub cron is disabled because its scheduled
events arrived many hours late. A dispatch can still wait for GitHub or runner
availability; this is not a guarantee against outages.
