#!/usr/bin/env bash
# Run one managed conformance session in which every restart that keeps the
# database starts the other engine on the checkpoint the first one wrote.
#
# Usage: cross-engine-session.sh BARN TOAST SUITE DIRECTION [HARNESS-ARGUMENT...]
#   BARN       Linux barn binary built from this checkout
#   TOAST      stock ToastStunt moo binary (profiles/toast/stock-wsl-testdb.json)
#   SUITE      moo-conformance-tests checkout, with its submodules
#   DIRECTION  barn-first (Barn writes, Toast resumes), barn-first-toast-loadable
#              (Barn writes, the Toast-loadable copy of it is made, Toast
#              resumes) or toast-first
# The harness arguments choose the tests and databases. The session selects
# the admission tests and the tests marked kept_db_restart, and exits with the
# harness's status.
set -uo pipefail

if [ "$#" -lt 4 ]; then
    echo "usage: $0 BARN TOAST SUITE DIRECTION [HARNESS-ARGUMENT...]" >&2
    exit 2
fi
barn="$(realpath -e "$1")" || exit 2
toast="$(realpath -e "$2")" || exit 2
suite="$(realpath -e "$3")" || exit 2
direction="$4"
shift 4
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
oracle="$repo/profiles/toast/stock-wsl-testdb.json"

barn_command="$barn serve --db {db} --listen tcp://127.0.0.1:{port} --config=$repo/profiles/barn/outbound-on.conf --profile-id=barn-linux-testdb-outbound-on --profile-manifest={manifest}"
toast_command="$toast {db} {db}.new -p {port}"

case "$direction" in
    barn-first)
        engines=(--server-command "$barn_command" --restart-server-command "$toast_command") ;;
    # Barn's checkpoint converted by `barn offline-checkpoint --toast-loadable`
    # before Toast starts on it (scripts/toast-loadable-restart.sh).
    barn-first-toast-loadable)
        engines=(--server-command "$barn_command"
            --restart-server-command "bash $repo/scripts/toast-loadable-restart.sh $barn $toast {db} {port}") ;;
    # The harness reads the target profile from the first engine's {manifest}
    # file. Toast writes none, so its verified profile is named instead.
    toast-first)
        engines=(--server-command "$toast_command" --restart-server-command "$barn_command"
            "--target-profile-manifest=$oracle") ;;
    *)
        echo "DIRECTION must be barn-first, barn-first-toast-loadable or toast-first, not $direction" >&2
        exit 2 ;;
esac

exec uv run --project "$suite" --frozen moo-conformance \
    -m "admission or kept_db_restart" \
    --moo-host=127.0.0.1 \
    --oracle-profile-manifest="$oracle" \
    --fail-on-unexpected-skip --strict-markers \
    -q -p no:cacheprovider "${engines[@]}" "$@"
