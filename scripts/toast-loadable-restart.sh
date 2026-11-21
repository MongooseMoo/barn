#!/usr/bin/env bash
# Start Toast on a checkpoint Barn wrote, after converting it with
# `barn offline-checkpoint --toast-loadable`: the tasks Barn kept in its own
# frame form, which Toast cannot load, are left out and listed on stderr.
# A managed-harness restart command, as in scripts/cross-engine-checkpoint.sh:
#
#   --restart-server-command "bash scripts/toast-loadable-restart.sh BARN TOAST {db} {port}"
#
#   BARN   Linux barn binary built from this checkout
#   TOAST  ToastStunt moo binary
# With TOAST_LOADABLE_LOG set, the command's output (one `omitted task` line
# per task left out, then its `wrote` line) is also appended to that file.
# The converted copy replaces {db}, and Toast writes its own checkpoint to
# {db}.new. The copy's WAIF identity sidecar is removed: Toast neither reads
# nor rewrites it, and Barn refuses a database beside a sidecar that does not
# match it (db/format waif identity sidecar), so one left beside {db} would
# stop Barn loading what Toast writes there later.
set -euo pipefail

if [ "$#" -ne 4 ]; then
    echo "usage: $0 BARN TOAST DB PORT" >&2
    exit 2
fi
barn="$1"
toast="$2"
db="$3"
port="$4"

if [ -n "${TOAST_LOADABLE_LOG:-}" ]; then
    # The harness passes its environment to restart commands; a caller that
    # counts the omitted tasks (scripts/fuzz_cross_engine.py) names a file.
    "$barn" offline-checkpoint --db "$db" --toast-loadable --log-dir= | tee -a "$TOAST_LOADABLE_LOG" >&2
else
    "$barn" offline-checkpoint --db "$db" --toast-loadable --log-dir= >&2
fi
mv -f "$db.new" "$db"
rm -f "$db.new.waifids"
exec "$toast" "$db" "$db.new" -p "$port"
