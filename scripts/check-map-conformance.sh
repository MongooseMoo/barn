#!/usr/bin/env bash
# Focused managed map verification, including canonical capability admission.
set -euo pipefail
engine=${1:?usage: check-map-conformance.sh toast|barn BINARY REPORT_DIRECTORY}
binary=$(realpath "${2:?binary required}")
reports=$(realpath -m "${3:?report directory required}")
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
uv run --frozen moo-conformance \
  _tests/builtins/map.yaml \
  _tests/builtins/mapdelete_call_shapes.yaml \
  _tests/builtins/maphaskey_call_shapes.yaml \
  _tests/builtins/mapvalues_call_shapes.yaml \
  _tests/language/map_authority.yaml \
  _tests/server/map_dump_persistence.yaml \
  _tests/generated_builtins/mapdelete.yaml \
  _tests/generated_builtins/maphaskey.yaml \
  _tests/generated_builtins/mapkeys.yaml \
  _tests/generated_builtins/mapvalues.yaml \
  --server-command="$command" \
  --server-db="$repo/../moo-conformance-tests/src/moo_conformance/_db/Test.db" \
  --moo-host=127.0.0.1 --moo-port=17930 \
  --oracle-profile-manifest="$oracle" "${extra[@]}" \
  -k "capability_admission or ${MAP_TEST_SELECTOR:-map}" --fail-on-unexpected-skip --strict-markers \
  --admission-evidence-output="$reports/$engine-map-admission.json" \
  --admission-evidence-context="map-verification:$engine:$(sha256sum "$binary" | cut -d ' ' -f 1)" \
  --junitxml="$reports/$engine-maps.xml" -q
