#!/usr/bin/env bash
# Run the existing managed three-way proof for exact Barn and suite commits.
# Usage: repair-conformance-proof.sh BARN_BASE BARN_HEAD SUITE_HEAD SUITE_CHECKOUT
set -euo pipefail
BASE="$1" HEAD_SHA="$2" suite_ref="$3"
suite_repo=$(realpath -e "$4")
for sha in "$BASE" "$HEAD_SHA" "$suite_ref"; do
  [[ "$sha" =~ ^[0-9a-f]{40}$ ]]
done

work=$(mktemp -d "$RUNNER_TEMP/conformance-proof-XXXXXXXX")
evidence="$RUNNER_TEMP/conformance-evidence.json"
rm -f "$evidence"
cleanup() {
  git worktree remove --force "$work/base" 2>/dev/null || true
  git worktree remove --force "$work/head" 2>/dev/null || true
  git -C "$suite_repo" worktree remove --force "$work/suite" 2>/dev/null || true
  rm -rf -- "$work"
}
trap cleanup EXIT

# Run the new tests on the chosen suite commit. The companion may add test
# data and update the startup checksums, never change the harness it uses.
git -C "$suite_repo" fetch --no-tags origin main
suite_base=$(git -C "$suite_repo" merge-base FETCH_HEAD "$suite_ref")
changes=$(git -C "$suite_repo" diff --no-renames --name-status "$suite_base" "$suite_ref")
files=()
while IFS=$'\t' read -r status file; do
  [[ -n "$file" ]] || continue
  case "$status:$file" in
    A:src/moo_conformance/_tests/*.yaml|A:src/moo_conformance/_tests/*.yml) files+=("$file") ;;
    A:src/moo_conformance/_tests/*|A:src/moo_conformance/_db/*|M:src/moo_conformance/_db/startup/startup-fixtures.sha256) ;;
    *) echo "Companion changes an existing suite file: $file" >&2; exit 1 ;;
  esac
done <<< "$changes"
if [ "${#files[@]}" -eq 0 ]; then
  echo "Companion adds no conformance test, so there is no proof." >&2
  exit 1
fi
printf 'Selected: %s\n' "${files[@]}"

git -C "$suite_repo" worktree add --detach "$work/suite" "$suite_ref"
# The suite's ToastCore startup database is a git submodule.
git -C "$work/suite" submodule update --init
git worktree add --detach "$work/base" "$BASE"
git worktree add --detach "$work/head" "$HEAD_SHA"
(cd "$work/base" && go build -o "$work/barn-base" ./cmd/barn)
(cd "$work/head" && go build -o "$work/barn-head" ./cmd/barn)
(cd "$work/base" && bash scripts/build-toast-oracle.sh)
toast="$work/base/toaststunt/build/moo"
oracle="$work/base/profiles/toast/stock-wsl-testdb.json"
config="$work/base/profiles/barn/outbound-on.conf"

run() {
  local name="$1" command="$2" status=0
  shift 2
  echo "=== $name ==="
  timeout 1200 uv run --project "$work/suite" --frozen moo-conformance "${files[@]}" \
    -m "admission or conformance" \
    --server-command "$command" \
    --server-db="$work/suite/src/moo_conformance/_db/Test.db" \
    --server-db-dir="$work/suite/src/moo_conformance/_db/startup" \
    --moo-host=127.0.0.1 \
    --oracle-profile-manifest="$oracle" \
    --fail-on-unexpected-skip --strict-markers -q -p no:cacheprovider \
    --junitxml="$work/$name.xml" "$@" || status=$?
  echo "$status" > "$work/$name.exit"
  echo "=== $name: exit $status ==="
}
barn() {
  echo "$1 serve --db {db} --listen tcp://127.0.0.1:{port} --config=$config --profile-id=barn-linux-testdb-outbound-on --profile-manifest={manifest} --log-dir=$work/$2-logs"
}
run toast "$toast {db} {db}.new -p {port}" "--target-profile-manifest=$oracle"
run barn_base "$(barn "$work/barn-base" barn_base)"
run barn_head "$(barn "$work/barn-head" barn_head)"

python3 - "$work" "$suite_ref" "$BASE" "$HEAD_SHA" "${files[@]}" > "$evidence" <<'PY'
import json
import sys
import xml.etree.ElementTree as ET
from pathlib import Path

work, suite, base, head, *files = sys.argv[1:]


def result(name):
    run = {"exit": int((Path(work) / f"{name}.exit").read_text()), "passed": [], "failed": [], "skipped": [], "errors": []}
    junit = Path(work) / f"{name}.xml"
    if junit.is_file():
        for case in ET.parse(junit).iter("testcase"):
            if case.find("error") is not None:
                run["errors"].append(case.get("name"))
            elif case.find("failure") is not None:
                run["failed"].append(case.get("name"))
            elif case.find("skipped") is not None:
                run["skipped"].append(case.get("name"))
            else:
                run["passed"].append(case.get("name"))
    return run


print(json.dumps({"suite": suite, "files": files, "base": base, "head": head,
                  "runs": {name: result(name) for name in ("toast", "barn_base", "barn_head")}}, indent=2))
PY
cat "$evidence"
