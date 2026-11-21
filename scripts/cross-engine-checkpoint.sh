#!/usr/bin/env bash
# Run the checkpoint restart tests across engines, in both directions: one
# engine writes the checkpoint and the other resumes from it. A restart test
# that only ever restarts Barn as Barn passes even when Barn's checkpoint
# import and export are wrong in the same way.
#
# Usage: cross-engine-checkpoint.sh BARN TOAST SUITE
#   BARN   Linux barn binary built from this checkout
#   TOAST  stock ToastStunt moo binary (profiles/toast/stock-wsl-testdb.json)
#   SUITE  moo-conformance-tests checkout, with its submodules
#
# Each direction runs two managed sessions (scripts/cross-engine-session.sh):
#   - the suite's own tests that restart on the database the server wrote
#     (its kept_db_restart marker);
#   - this checkout's tests/mongoose-conformance/server/*checkpoint*.yaml.
# All four sessions run even when an earlier one fails; the exit status is 1
# when any failed. The harness picks free ports and gives every server its own
# temporary directory, so concurrent copies of this script do not collide.
set -uo pipefail

if [ "$#" -ne 3 ]; then
    echo "usage: $0 BARN TOAST SUITE" >&2
    exit 2
fi
barn="$(realpath -e "$1")" || exit 2
toast="$(realpath -e "$2")" || exit 2
suite="$(realpath -e "$3")" || exit 2
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

local_root="$repo/tests/mongoose-conformance"
local_paths=()
for file in "$local_root"/server/*checkpoint*.yaml; do
    if [ ! -f "$file" ]; then
        echo "no checkpoint tests match $local_root/server/*checkpoint*.yaml" >&2
        exit 2
    fi
    local_paths+=("--moo-suite-path=server/$(basename "$file")")
done

# Candidate suites may only read a database inside this checkout.
mkdir -p "$repo/.tmp"
work="$(mktemp -d "$repo/.tmp/cross-engine.XXXXXX")" || exit 2
trap 'rm -rf "$work"' EXIT
cp -R "$suite/src/moo_conformance/_db/." "$work/" || exit 2

status=0
run() {
    local label="$1" direction="$2"
    shift 2
    echo "=== $label ==="
    if bash "$repo/scripts/cross-engine-session.sh" "$barn" "$toast" "$suite" "$direction" "$@"; then
        echo "=== $label: passed ==="
    else
        echo "=== $label: FAILED ==="
        status=1
    fi
}

packaged=(
    "--server-db=$suite/src/moo_conformance/_db/Test.db"
    "--server-db-dir=$suite/src/moo_conformance/_db/startup"
)
local=(
    "--candidate-root=$repo"
    "--moo-suite-root=$local_root"
    "${local_paths[@]}"
    "--server-db=$work/Test.db"
    "--server-db-dir=$work/startup"
)

run "Barn writes, Toast resumes: suite tests" barn-first "${packaged[@]}"
run "Barn writes, Toast resumes: Barn checkpoint tests" barn-first "${local[@]}"
run "Toast writes, Barn resumes: suite tests" toast-first "${packaged[@]}"
run "Toast writes, Barn resumes: Barn checkpoint tests" toast-first "${local[@]}"
exit "$status"
