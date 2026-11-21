#!/usr/bin/env bash
# Runs `barn toast-check` on a private database against the builtins a built
# Toast registers, and prints only what is safe in a public log.
#
#   scripts/toast-check.sh TOAST_BINARY BARN_BINARY DATABASE DIRECTORY
#
# The builtin list comes from TOAST_BINARY itself: its function_info() names,
# asked in emergency mode on a copy of a small Toast database from this
# repository. Toast's source cannot give it: five registrations there are
# compiled out of the Mongoose build (anon, background_test, finished_tasks,
# malloc_stats, read_stdin on 72e3c7f).
#
# DIRECTORY receives the list (toast-builtins.txt) and the check's whole output
# (toast-check-output.txt), which names players' objects and verbs; the caller
# keeps it private and removes it.
#
# Everything this script prints is text written out in this file with numbers
# the patterns below limit to digits, or a builtin name in the check's "which
# Barn registers and Toast does not" lines, which comes from Barn's own
# registry. Exit status is 0 only when the check passed and printed its count
# line.
set -uo pipefail
toast=$(realpath "${1:?usage: toast-check.sh TOAST_BINARY BARN_BINARY DATABASE DIRECTORY}")
barn=$(realpath "${2:?barn binary required}")
database=$(realpath "${3:?database required}")
work=$(realpath "${4:?directory required}")
repo=$(cd "$(dirname "$0")/.." && pwd)
names="$work/toast-builtins.txt"
out="$work/toast-check-output.txt"

# Toast creates its listener before emergency mode (server.cc main); -p 0
# binds an ephemeral port. notify() in emergency mode prints "#N <- text".
cp "$repo/engine/testdata/toast-command-environment.db" "$work/function-info.db"
printf ';;for f in (function_info()) notify(player, f[1]); endfor\nquit\n' |
  timeout 120 "$toast" -e -p 0 "$work/function-info.db" "$work/function-info.out.db" > "$work/function-info.txt" 2>&1
status=$?
grep -oE ' <- [a-z_0-9]+$' "$work/function-info.txt" | sed 's/^ <- //' | LC_ALL=C sort -u > "$names"
echo "Toast registers $(wc -l < "$names") builtins (emergency mode exit $status)"
if [ "$status" -ne 0 ] || [ ! -s "$names" ]; then
  echo "could not read Toast's builtin list."
  exit 1
fi

status=0
"$barn" toast-check --db "$database" --builtins "$names" --log-dir= -qq > "$out" 2>&1 || status=$?

grep -oE '^toast-check: verbs=[0-9]+ calls_barn_only_builtin=[0-9]+ unparsable=[0-9]+ calls_unregistered_builtin=[0-9]+ not_checkable=[0-9]+$' "$out" || true
grep -oE 'calls [a-z_0-9]+, which Barn registers and Toast does not$' "$out" |
  awk '{ count[$2]++ } END { for (name in count) print count[name] " calls to " substr(name, 1, length(name) - 1) ", which Barn registers and Toast does not" }' | sort

if ! grep -qE '^toast-check: verbs=' "$out"; then
  echo "toast-check printed no count line (exit $status); the rest of its output is not shown because it names players' objects and verbs."
  exit 1
fi
if [ "$status" -ne 0 ]; then
  echo "toast-check failed: exit $status. Verbs are named only in the private output."
  exit 1
fi
