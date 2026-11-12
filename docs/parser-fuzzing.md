# Verb-source fuzzing

Barn uses Go's coverage-guided fuzzer for source invariants and runtime inputs:

| Target | Invariant |
| --- | --- |
| `parser/FuzzParseFormatRoundTrip` | Accepted source formats and reparses to the same semantic IR in both formatter modes, ignoring positions and case-insensitive variable spelling; canonical formatting is stable. |
| `compiler/FuzzCompileVerifiable` | Accepted compilation produces bytecode accepted by the bytecode verifier. |
| `vm/FuzzSourceExecution` | Accepted compilation executes, resumes suspend/fork yields and runs queued children under one shared tick budget; representable return values survive a literal round trip. Go panics and failed assertions fail the input. |
| `command/FuzzCommandLine` | Any input line passes word splitting (the pre-login path), command parsing with object matching, and verb lookup as Test.db player `#3` in `#2` without a Go panic. |

Parsing/compilation errors are normal results. Execution uses a fresh task,
core builtin registry, and session with a 100-tick budget, in a small populated
world (`vm/fuzz_world_test.go`): a parent chain of objects with inherited,
overridden and unreadable properties, verbs that call each other, pass(),
raise, catch and recurse, a wizard player `#0` and a programmer player `#6`.
Each process builds the world once and saves it as a database; every execution
loads its own copy, so no input sees another's changes. Each input runs twice,
as the wizard and as the programmer. MOO errors and exhaustion are normal
results. Suspend/fork yields are driven to a
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

`vm/FuzzWorldCheckpointEffects` uses `internal/fuzzseed.WorldHistory` to generate
setup and a bounded dependent action history together: a live parent/child,
property values and metadata, installed and replaced verbs, nonwizard reads,
temporary definitions and queued children. Each action observes values,
permissions, dispatch and code, then suspends. Every input must complete its
setup and tasks and evaluate at least one checkpoint comparison; rejected setup
and zero checkpoint work fail rather than count as agreement. The same source
runs with and without checkpoint/reload, comparing per-task outcomes and final
state. Agreement is a recovery relation, not independent proof of every MOO rule.
The nonwizard read also checks the explicit readable/unreadable permission result.

The existing checkpoint driver now writes queued children with the suspended
parent, reloads their source and captured environment, and runs them through the
production fork VM constructor. Fixed cases retain anonymous and WAIF values
held only by queued work across repeated reloads. Controls require the observer
to detect a lost write, dropped queued work and changed verb code. Fixed world
histories also compare two sufficient tick budgets; this is a supported-setting
comparison, not an arbitrary performance threshold.

`task/FuzzSuspensionCompletionHistory` drives real task APIs against a small model
of suspension identities, accepted wake values, readiness consumption and terminal
cancellation. It includes stale and duplicate completions and new-task lifetimes.
A channel-coordinated cancellation/completion overlap runs under the task race
check. Neither this overlap nor workload choice replay enumerates Go schedules.

The existing Rapid map property now mixes integer, string, float and object keys,
case-changing string overwrites, nested list/map values and retained snapshots.
It checks insertion order, lookup, old versions and cached value sizes throughout
the history. Its Go choice records remain Rapid inputs, not rendered MOO source.

`db/store/FuzzStoreCommitsMatchSerialReplay` decodes Rapid choices into up to
three interleaved snapshot transactions (create, set, define, clear and delete
properties, staged recycle, verb delete and code edits, reads, commit, abort,
`CommitAndRenewCarryingReads`) and coarse live operations between them (add and
delete verb, chparent, recycle, renumber). Every successful commit is replayed,
in commit order, on a second store built the same way. The committed states must
match; a commit that validated its reads must give every operation the result it
gives at the commit point; a transaction with nothing staged must keep reading
its snapshot; reads carried across a boundary must still hold at the final commit.

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
target the `minutes` of its matrix entry (thirty, or five for lanes whose
coverage has stopped growing), with four Go fuzz workers per target. The default
`dedicated-once` mode and `dedicated-continuous` mode run exclusively on the
server's `barn-bug-discovery` runner group. This includes routing, acceptance
and controller jobs. Legacy workstation routing and its nightly-task installer
have been removed; disable any previously installed `Barn - Nightly fuzz` task.
A failed job opens an issue with its
run and artifact links; subsequent failures for the same target add those links
to its existing open issue. Setup failures are reported separately from failed
fuzz commands. The useful
principles from [the AFL++ article](https://nullprogram.com/blog/2025/02/05/)
are focused assertions and per-input isolation; Go supplies the mutation,
coverage instrumentation and minimization described in its
[fuzzing documentation](https://go.dev/doc/security/fuzz/).

The `compute-mode` dispatch input accepts `dedicated-once` and
`dedicated-continuous`. Both require master, runner group `barn-bug-discovery`,
`FUZZ_RUNNER_POLICY_VERIFIED=true` and nonempty JSON `FUZZ_RUNNER_LABELS`
including `self-hosted` and `barn-fuzz`. Missing configuration fails before
fuzz workload routing, without a workstation fallback. `FUZZ_MAX_PARALLEL`
defaults to two and accepts only two or four. The policy marker records the
operator's verified group enforcement; it is not itself enforcement. Ordinary
PR CI and repair workflows keep their existing runner ownership.

The independent `campaign` input defaults to `native-and-toast`, preserving the
existing native matrix followed by cross-engine comparison. `native-only` runs
the same native targets, retention and reporting without building or executing
Toast. Its run title ends in `/ native-only`; its acceptance log explicitly says
`differential: "not-run-native-only"`. This mode supplies native property and
recovery checks, not cross-engine agreement. For example, start one finite
dedicated campaign with:

```sh
gh workflow run fuzz.yml --ref master -f compute-mode=dedicated-once -f campaign=native-only
```

Each native job starts with an isolated public-only fuzz cache and clears
`BARN_FUZZ_CORPUS`. `scripts/fuzz_retention.py` restores retained records,
captures successful interesting inputs as well as testdata findings, verifies
SHA-256 identities, and records package/target, source head, Go version, pinned
profile digest and origins. Corpus plus provenance is bounded to 256 MiB per
target. Exhaustion fails without deleting retained records. Campaigns disable
minimization so a saved failure identifies its original replay bytes. The
retention helper preserves that finding and removes identical copies only from
the disposable native replay cache before the next attempt. An unremovable
static seed that fails again stops promptly. Saved findings return to testdata
before capture and remain in the retained artifacts. A census preserves
the raw logs and sums the highest native `execs` count from each campaign;
zero measured executions is inconclusive and fails. This is execution work,
not a claim of valid MOO program or distinct bug counts.

The next cycle restores a target's explicit public corpus cache across hosts.
Native `fuzz-corpus-*` artifacts retain the corpus, provenance and census for
30 days, including on failure. Toast differential replay waits for the entire
native matrix and downloads that run's artifacts, so it cannot depend on matrix
ordering or a shared workstation cache. It consumes raw VM/compiler source and
the existing structured source targets through their production renderer.
World-history and task-history choices have their own replay producers and are
not passed to that source decoder or the cross-engine lane.
Artifacts expire and GitHub can evict caches: preserve confirmed findings with
their regression fixes before expiry. The helper does not determine privacy
from arbitrary input bytes; do not feed local private corpora to public jobs.

Continuous mode additionally requires `FUZZ_CONTINUOUS_ENABLED=true`. Every
dispatch ends with an `Accept discovery cycle` job. It passes when each native
lane ran to completion with nonzero measured executions, kept its census and
evidence, and ended clean or with a saved finding. For `native-and-toast`, the
differential must also complete with `success` or `divergence`. For `native-only`,
the comparison job must be skipped and no comparison result may be present.
Only the step that found something
may fail. A saved finding or a real divergence keeps its job and the run red and
opens its issue as before, yet the cycle is still accepted. A setup failure,
zero measured work, an inconclusive differential or a failed upload, cache save
or issue report is not accepted.

Every continuous dispatch ends with a `Submit continuous successor` job. It runs
after any cycle, accepted, unaccepted or cancelled, as long as routing passed. It
reads the stop switch when it starts, checks the dedicated configuration, and
submits one successor with the same `campaign` selection unless another
dedicated cycle is already queued or running. The successor waits in the
`nightly-fuzz-dedicated` concurrency group until the current cycle ends.
Setting the switch to anything other than `true` stops the chain; cancel an
active run separately when immediate stopping is required.

The cycle submits its own successor because a separate `workflow_run`
controller missed cycles: GitHub created no completion event for runs
42766490058 and 42897395293, both dispatched by the controller with
`GITHUB_TOKEN`, while it did for every cycle a person dispatched. A
`GITHUB_TOKEN` `workflow_dispatch` does create the run (see GitHub's
[GITHUB_TOKEN trigger rules](https://docs.github.com/en/actions/concepts/security/github_token)).
`fuzz-controller.yml` remains as a backup on the same server group: its
`replenish` job runs after a person's dispatch completes and does nothing when
a newer dispatch exists, and its `watchdog` runs on a schedule that GitHub has
been creating late or not at all (2 of about 34 due runs on 2026-11-10; see
[community discussion 206019](https://github.com/orgs/community/discussions/206019)).
Trigger delivery, runner policy, a completed dedicated cycle and corpus replay
after restart must be verified during activation; source changes and offline
controller tests do not establish live acceptance.

There is no workstation discovery trigger. Ordinary PR CI retains its existing
runner ownership. A discovery dispatch can still wait for GitHub or server
runner availability; this is not a guarantee against outages.
