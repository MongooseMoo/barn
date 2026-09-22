# Managed MOO conformance for the Mongoose changes

Use the same managed `moo-conformance` entrypoint as CI. The former
`test-mongoose-deltas.ps1`, `test-shared-admission.ps1` and
`recheck-conformance-failures.ps1` were investigation helpers that invoked the
obsolete pytest path. They have been removed. Commands in the September 18
investigation records describe those historical runs, not current verification.

Run these commands inside the installed WSL distribution, from the Barn checkout.
Keep the conformance checkout beside Barn, or set `suite` to its absolute path.
Use a Linux-only uv environment. No Mongoose database is involved.

```bash
set -euo pipefail
repo=$(pwd)
suite=$(realpath ../moo-conformance-tests)
export UV_PROJECT_ENVIRONMENT="${XDG_CACHE_HOME:-$HOME/.cache}/barn-conformance-linux"
mkdir -p "$repo/.tmp/conformance"
go list -f '{{.ImportPath}} {{.Name}}' ./cmd/barn
go build -o "$repo/.tmp/conformance/barn" ./cmd/barn

uv run --project "$suite" --frozen moo-conformance \
  -m 'admission or conformance' \
  --server-command="$repo/.tmp/conformance/barn --db {db} --listen tcp://127.0.0.1:{port} --config=$repo/profiles/barn/outbound-on.conf --profile-id=barn-linux-testdb-outbound-on --profile-manifest={manifest}" \
  --server-db="$suite/src/moo_conformance/_db/Test.db" \
  --server-db-dir="$suite/src/moo_conformance/_db/startup" \
  --moo-host=127.0.0.1 \
  --oracle-profile-manifest="$repo/profiles/toast/stock-wsl-testdb.json" \
  --fail-on-unexpected-skip --strict-markers \
  --admission-evidence-output="$repo/.tmp/conformance/barn-admission.json" \
  --admission-evidence-context="barn-local:$(git rev-parse HEAD)" \
  --junitxml="$repo/.tmp/conformance/barn.xml" -q
```

For a focused packaged run, add `-k 'capability_admission or <test-name>'`.
For the generic candidate regressions maintained in this branch, add
`--candidate-root="$repo" --moo-suite-root="$repo/tests/mongoose-conformance"`
and select one or more relative files with `--moo-suite-path`, for example
`--moo-suite-path=builtins/catch_ternary_fallback.yaml`. Keep admission in the same
session. A focused run does not replace the full suite above.

Candidate mode also requires fixtures inside the candidate checkout. Before
using that mode, copy the packaged fixtures:

```bash
mkdir -p "$repo/.tmp/conformance/db"
cp -R "$suite/src/moo_conformance/_db/." "$repo/.tmp/conformance/db/"
```

Then replace the full command's database arguments with
`--server-db="$repo/.tmp/conformance/db/Test.db"` and
`--server-db-dir="$repo/.tmp/conformance/db/startup"`. These are disposable copies;
the managed harness owns the working databases for each session.

Verify uncertain MOO expectations against stock Toast first. Use the same
managed command and selection, replacing the Barn `--server-command` with
`--server-command='/root/src/toaststunt/build-release/moo {db} {db}.new {port}'`
and adding `--target-profile-manifest="$repo/profiles/toast/stock-wsl-testdb.json"`.
Give the oracle its own evidence and XML output names. The Mongoose-specific
Toast executable used for private live-world benchmarks is not this oracle.

For admission-capacity comparisons, copy `profiles/barn/outbound-on.conf` to a
disposable file and append `ADMISSION_LIMIT = 1` or `ADMISSION_LIMIT = 16`. Point
the server command's `--config` at that file. Retain the other configuration,
profile, assertions and admission check, and identify the limit in the evidence
context and output filenames.
Collect the terminal summary even when failures occur. Do not classify a failure
as a baseline failure without reproducing it through this same managed workflow.
