#!/usr/bin/env bash
# wsl_oracle.sh — evaluate MOO expressions on the canonical (WSL/Linux) ToastStunt
# build via emergency-mode wizard eval. This is THE oracle for convergence work.
#
# Build: lisdude/toaststunt origin/master, ~/src/toaststunt/build/moo (Linux).
# The Windows MSVC build is NOT canonical (crypt/other divergences) — do not use it.
#
# Usage (run from WSL, or via `wsl -e bash -lc '.../wsl_oracle.sh ...'`):
#   wsl_oracle.sh ';EXPR'                 # eval one expression against mongoose_fresh2.db
#   wsl_oracle.sh ';E1' ';E2' 'program ...'   # multiple emergency commands in order
#   DB=/path/to/other.db wsl_oracle.sh ';EXPR'
#
# Each invocation copies the db to a scratch file (emergency mode rewrites the db
# on exit) and picks a random high port to avoid colliding with a running server.
set -u

MOO="${MOO:-$HOME/src/toaststunt/build/moo}"
DB="${DB:-/mnt/c/Users/Q/code/barn/mongoose_fresh2.db}"

if [[ ! -x "$MOO" ]]; then
  echo "ERROR: oracle moo binary not found/executable at $MOO" >&2
  exit 2
fi
if [[ ! -f "$DB" ]]; then
  echo "ERROR: oracle db not found at $DB" >&2
  exit 2
fi

scratch="$(mktemp /tmp/oracle_in.XXXXXX.db)"
out="$(mktemp /tmp/oracle_out.XXXXXX.db)"
cp "$DB" "$scratch"
port=$(( (RANDOM % 5000) + 20000 ))

# Build the emergency-mode command stream: each arg is one command line,
# followed by quit so the server exits cleanly.
{
  for cmd in "$@"; do
    printf '%s\n' "$cmd"
  done
  printf 'quit\n'
} | "$MOO" -e -p "$port" "$scratch" "$out" 2>&1 \
  | grep -E '^\(#2\): =>|^=>' \
  | sed -E 's/^\(#2\): //'

rc=${PIPESTATUS[1]}
rm -f "$scratch" "$out"
exit "$rc"
