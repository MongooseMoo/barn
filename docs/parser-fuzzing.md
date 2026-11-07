# Verb-source fuzzing

Barn uses Go's coverage-guided fuzzer for source invariants and runtime inputs:

| Target | Invariant |
| --- | --- |
| `parser/FuzzParseFormatRoundTrip` | Accepted source formats and reparses to the same semantic IR in both formatter modes, ignoring positions and case-insensitive variable spelling; canonical formatting is stable. |
| `compiler/FuzzCompileVerifiable` | Accepted compilation produces bytecode accepted by the bytecode verifier. |
| `vm/FuzzSourceExecution` | Accepted compilation executes, resumes suspend/fork yields and runs queued children under one shared tick budget; representable return values survive a literal round trip. Go panics and failed assertions fail the input. |
| `command/FuzzCommandLine` | Any input line passes word splitting (the pre-login path), command parsing with object matching, and verb lookup as Test.db player `#3` in `#2` without a Go panic. |

Parsing/compilation errors are normal results. Execution uses a fresh empty
store, task, core builtin registry, and session with a 100-tick budget. MOO
errors and exhaustion are normal results. Suspend/fork yields are driven to a
terminal return or exception; children run in creation order on the remaining
budget. This does not run a real scheduler or optional I/O capabilities.
It does not assert MOO behavioral conformance; use the managed Toast oracle for
that. No target recovers Go panics.

The raw source targets seed every programmed verb in the tracked `Test.db`, including
empty programs and anonymous objects. Identical source bytes are added once.
Small syntax/boundary cases complement whole verbs. Ordinary `go test` replays
these seeds and any committed findings, without running timed fuzz campaigns.
The corresponding `FuzzStructuredParseFormatRoundTrip`,
`FuzzStructuredCompileVerifiable` and `FuzzStructuredSourceExecution` targets
use the existing bounded generator in `internal/fuzzseed/structured.go`.
Their exact `[]byte` choices replay through `cmd/moo-fuzz-source`; they require
generated programs to compile. Keep those bytes with the generator's source
version. The workflow also includes snapshot, bytecode, database reader,
HTTP chunking, TCP input and string-builtin targets; its matrix is the inventory.

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
PR CI runs no timed fuzzing. `.github/workflows/fuzz.yml` gives each native
target thirty minutes, with two Go fuzz workers per target. The default
`legacy` mode preserves the workstation's 3:17 AM America/Denver task
`Barn - Nightly fuzz`, its `CI_RUNNER_LABELS` hosted fallback and two-job cap.
A failed job opens an issue with its
run and artifact links; subsequent failures for the same target add those links
to its existing open issue. Setup failures are reported separately from failed
fuzz commands. The useful
principles from [the AFL++ article](https://nullprogram.com/blog/2025/02/05/)
are focused assertions and per-input isolation; Go supplies the mutation,
coverage instrumentation and minimization described in its
[fuzzing documentation](https://go.dev/doc/security/fuzz/).

The single `compute-mode` dispatch input also accepts `dedicated-once` and
`dedicated-continuous`. Both require master, runner group `barn-bug-discovery`,
`FUZZ_RUNNER_POLICY_VERIFIED=true` and nonempty JSON `FUZZ_RUNNER_LABELS`
including `self-hosted` and `barn-fuzz`. Missing configuration fails before
fuzz workload routing, without a workstation fallback. `FUZZ_MAX_PARALLEL`
defaults to two and accepts only two or four. The policy marker records the
operator's verified group enforcement; it is not itself enforcement. Ordinary
PR CI and repair workflows keep their existing runner ownership.

Each native job starts with an isolated public-only fuzz cache and clears
`BARN_FUZZ_CORPUS`. `scripts/fuzz_retention.py` restores retained records,
captures successful interesting inputs as well as testdata findings, verifies
SHA-256 identities, and records package/target, source head, Go version, pinned
profile digest and origins. Corpus plus provenance is bounded to 256 MiB per
target. Exhaustion fails without deleting retained records. A census preserves
the raw logs and sums the highest native `execs` count from each campaign;
zero measured executions is inconclusive and fails. This is execution work,
not a claim of valid MOO program or distinct bug counts.

The next cycle restores a target's explicit public corpus cache across hosts.
Native `fuzz-corpus-*` artifacts retain the corpus, provenance and census for
30 days, including on failure. Toast differential replay waits for the entire
native matrix and downloads that run's artifacts, so it cannot depend on matrix
ordering or a shared workstation cache. It currently consumes raw VM/compiler
source records; structured-choice decoding is a separate oracle change.
Artifacts expire and GitHub can evict caches: preserve confirmed findings with
their regression fixes before expiry. The helper does not determine privacy
from arbitrary input bytes; do not feed local private corpora to public jobs.

Continuous mode additionally requires `FUZZ_CONTINUOUS_ENABLED=true`. Every
dispatch ends with an `Accept discovery cycle` job. It passes when each native
lane ran to completion with nonzero measured executions, kept its census and
evidence, and ended clean or with a saved finding, and the differential
completed with `success` or `divergence`. Only the step that found something
may fail. A saved finding or a real divergence keeps its job and the run red and
opens its issue as before, yet the cycle is still accepted. A setup failure,
zero measured work, an inconclusive differential or a failed upload, cache save
or issue report is not accepted.

A completed continuous dispatch's completion starts the separate
`fuzz-controller.yml` workflow with a new repository-variable snapshot (GitHub
rejects a workflow whose `workflow_run` trigger names itself). It runs on the
ordinary CI selector, since the dedicated group admits only `fuzz.yml`, and only
dispatches. The controller checks the stop switch and dedicated configuration,
accepts only a trusted master source dispatch whose acceptance job passed, and
submits one successor only when there is no newer or active dispatch. An
unaccepted cycle stops the chain with a red controller run. Setting the switch to anything
other than `true` prevents replenishment; cancel an active run separately when
immediate stopping is required. The controller uses its new run's variable
snapshot; the switch does not interrupt an already-running controller. No extra credential or variables REST read is
used. See GitHub's [completion event rules](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflow_run)
and [token dispatch rules](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/trigger-a-workflow#triggering-a-workflow-from-a-workflow).
Trigger delivery, runner policy, a completed dedicated cycle and corpus replay
after restart must be verified during activation; source changes and offline
controller tests do not establish live acceptance.

Install or update the nightly trigger as the logged-in runner owner with
`./scripts/install-nightly-fuzz-task.ps1`. It uses the existing authenticated
GitHub CLI and follows the workstation's Mountain timezone, including daylight
saving. Q must remain logged on, as for the Barn runner tasks. Missed triggers
are not run later in the day. GitHub cron is disabled because its scheduled
events arrived many hours late. A dispatch can still wait for GitHub or runner
availability; this is not a guarantee against outages.
