#!/usr/bin/env bash
# Managed conformance run of selected suite files against one engine, always
# including canonical capability admission. Run inside WSL from any directory.
#
#   scripts/check-conformance.sh toast /root/src/toaststunt/build-release/moo OUT SELECTOR FILE...
#   scripts/check-conformance.sh barn  /root/barn-linux                    OUT SELECTOR FILE...
#
# FILE paths are relative to the conformance package (e.g. _tests/language/scatter.yaml).
# SELECTOR is a pytest -k expression; capability admission is always added.
set -euo pipefail
engine=${1:?usage: check-conformance.sh toast|barn BINARY REPORT_DIRECTORY SELECTOR FILE...}
binary=$(realpath "${2:?binary required}")
reports=$(realpath -m "${3:?report directory required}")
selector=${4:?selector required}
shift 4
[ "$#" -gt 0 ] || { echo "at least one suite file is required" >&2; exit 2; }
repo=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$reports"
cd "$repo"
export UV_PROJECT_ENVIRONMENT=${UV_PROJECT_ENVIRONMENT:-/root/.cache/barn-mongoose-account-conformance-venv}
oracle="$repo/profiles/toast/stock-wsl-testdb.json"
extra=()
case "$engine" in
  toast)
    command="$binary {db} {db}.new {port}"
    extra+=("--target-profile-manifest=$oracle")
    ;;
  barn)
    command="$binary --db {db} --listen tcp://127.0.0.1:{port} --config=$repo/profiles/barn/outbound-on.conf --profile-id=barn-linux-testdb-outbound-on --profile-manifest={manifest}"
    ;;
  *) echo "unknown engine: $engine" >&2; exit 2 ;;
esac
uv run --frozen moo-conformance "$@" \
  --server-command="$command" \
  --server-db="$repo/../moo-conformance-tests/src/moo_conformance/_db/Test.db" \
  --moo-host=127.0.0.1 --moo-port="${MOO_PORT:-17940}" \
  --oracle-profile-manifest="$oracle" "${extra[@]}" \
  -k "capability_admission or ($selector)" --fail-on-unexpected-skip --strict-markers \
  --admission-evidence-output="$reports/$engine-admission.json" \
  --admission-evidence-context="check-conformance:$engine:$(sha256sum "$binary" | cut -d ' ' -f 1)" \
  --junitxml="$reports/$engine.xml" -q
