#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "$0")/.." && pwd -P)
context=$1
shift
suite="$root/moo-conformance-tests"
cd -- "$root"
exec uv run --project "$suite" --frozen moo-conformance \
  -m "admission or conformance" \
  --server-command "$root/bin/barn serve --db {db} --listen tcp://127.0.0.1:{port} --config=$root/profiles/barn/outbound-on.conf --profile-id=barn-linux-testdb-outbound-on --profile-manifest={manifest} --log-dir=$root/conformance-logs" \
  --server-db="$suite/src/moo_conformance/_db/Test.db" \
  --server-db-dir="$suite/src/moo_conformance/_db/startup" \
  --moo-host=127.0.0.1 \
  --oracle-profile-manifest="$root/profiles/toast/stock-wsl-testdb.json" \
  --fail-on-unexpected-skip \
  --strict-markers \
  --admission-evidence-output="$root/conformance-admission.json" \
  --admission-evidence-context="$context" \
  -q \
  --junitxml="$root/conformance-results.xml" \
  "$@"
