#!/usr/bin/env bash
# Launch canonical (WSL/Linux) ToastStunt for the moo-conformance managed
# harness. The harness runs on Windows and substitutes a Windows-style db path
# and a port; this wrapper translates the path into WSL and execs the Linux moo
# in the foreground so that when the harness kills the `wsl.exe` relay, Toast
# dies with it (no orphan servers).
#
# Used as:  --server-command "wsl -e bash <this> {db} {port}"
#
# Toast CLI: moo INPUT-DB OUTPUT-DB PORT. The managed conformance harness
# adopts `{db}.new` across restart_server steps, so write checkpoint output
# there instead of discarding it.
set -euo pipefail

db_win="$1"
port="$2"

# Toast reverse-DNS-resolves every incoming connection. A connection from the
# Windows side arrives from the NAT gateway IP, whose lookup hangs ~10s and
# trips the harness's socket timeouts. Pin the gateway name locally; /etc/hosts
# is regenerated on distro restart, so this must stay idempotent and re-run.
gw="$(ip route show default | awk '{print $3}')"
if [ -n "${gw}" ] && ! grep -q "^${gw} " /etc/hosts; then
    echo "${gw} windows-nat-gateway" >> /etc/hosts
fi
moo="${TOAST_MOO:-$HOME/src/toaststunt/build-release/moo}"

db_wsl="$(wslpath "$db_win")"
exec "$moo" "$db_wsl" "${db_wsl}.new" "$port"
