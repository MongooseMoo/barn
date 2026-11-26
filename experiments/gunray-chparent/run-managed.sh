#!/usr/bin/env bash
set -euo pipefail
run=$(realpath "$1")
phase=$2
label=$3
scriptdir=$(cd "$(dirname "$0")" && pwd)
suite=${MOO_CONFORMANCE_ROOT:-$run/suite}
repo=${BARN_REPO:-$(cd "$scriptdir/../.." && pwd)}
oracle=$repo/profiles/toast/stock-wsl-testdb.json
out=$run/evidence/$phase-$label
mkdir "$out"
export UV_PROJECT_ENVIRONMENT=$run/conformance-venv
extra=()
case "$label" in
  toast)
    printf -v server '%q {db} {db}.new {port}' "${TOAST_BINARY:?set TOAST_BINARY to the stock Toast executable}"
    extra+=(--target-profile-manifest="$oracle")
    ;;
  before|after|current)
    if [[ $label != current ]]; then repo=$run/$label; fi
    printf -v server '%q' "${BARN_BINARY:-$run/barn-$label}"
    if [[ $label == current ]]; then server+=" serve"; fi
    printf -v barn_config '%q' "$repo/profiles/barn/outbound-on.conf"
    server+=" --db {db} --listen tcp://127.0.0.1:{port} --config=$barn_config --profile-id=barn-linux-testdb-outbound-on --profile-manifest={manifest}"
    ;;
  *) exit 2 ;;
esac
if [[ $phase == packaged ]]; then
  select='capability_admission or chparent_keeps_written_value_of_common_ancestor_property or chparent_keeps_common_ancestor_slot_written_in_same_task'
else
  candidate=${4:-gunray_499}
  select="capability_admission or $candidate"
  extra+=(--candidate-root="$run/candidate" --moo-suite-root="$run/candidate/cases" --moo-suite-path="$candidate.yaml" -p gunray_observe)
  export PYTHONPATH=$scriptdir GUNRAY_OBSERVATIONS=$out/observations.jsonl
fi
cd "$repo"
args=(uv run --project "$suite" --frozen moo-conformance -m 'admission or conformance' -k "$select"
  --server-command="$server" --server-db="$run/candidate/db/Test.db"
  --server-db-dir="$run/candidate/db/startup" --moo-host=127.0.0.1
  --oracle-profile-manifest="$oracle" "${extra[@]}" --fail-on-unexpected-skip --strict-markers
  --admission-evidence-context="$phase-$label-$(git rev-parse HEAD)"
  --admission-evidence-output="$out/admission.json" --junitxml="$out/results.xml" -v)
printf '%q ' "${args[@]}" >"$out/command"
if "${args[@]}" >"$out/session.log" 2>&1; then status=0; else status=$?; fi
echo "$status" >"$out/exit-code"
tail -12 "$out/session.log"
exit "$status"
