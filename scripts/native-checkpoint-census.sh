#!/usr/bin/env bash
# Runs TestToastNativeCheckpointRestorationCensus on a private checkpoint and
# prints only what is safe in a public log.
#
#   scripts/native-checkpoint-census.sh ENGINE_TEST_BINARY DATABASE DIRECTORY
#
# ENGINE_TEST_BINARY is `go test -c ./engine`. DIRECTORY receives the test's
# whole output (census-output.txt), which names players' objects, verbs and
# tasks; the caller keeps it private and removes it.
#
# Everything this script prints is one of:
#   - text written out in this file, with numbers the patterns below limit to
#     digits;
#   - a refusal reason that is a string literal in this repository's Go source
#     (already public), with the number of tasks refused for it.
# No other byte of the test's output is printed, whether the test passes,
# fails, panics or times out. Exit status is 0 only when the test passed and
# printed its summary.
set -uo pipefail
binary=$(realpath "${1:?usage: native-checkpoint-census.sh ENGINE_TEST_BINARY DATABASE DIRECTORY}")
database=$(realpath "${2:?database required}")
work=$(realpath "${3:?directory required}")
repo=$(cd "$(dirname "$0")/.." && pwd)
out="$work/census-output.txt"

status=0
(cd "$repo/engine" && BARN_MONGOOSE_DB="$database" "$binary" \
  -test.run '^TestToastNativeCheckpointRestorationCensus$' -test.v -test.timeout 10m) > "$out" 2>&1 || status=$?

grep -oE \
  -e 'native census coverage( requires queued tasks, native continuations and native frames)?: queued=[0-9]+ continuations=[0-9]+ frames=[0-9]+' \
  -e 'checkpoint restored: queued=[0-9]+/[0-9]+ suspended/interrupted=[0-9]+/[0-9]+' \
  -e '^(--- (PASS|FAIL|SKIP): TestToastNativeCheckpointRestorationCensus|PASS|FAIL|panic:)' \
  "$out" || true

# One log record per task Barn refused (engine/task_load.go). The reasons to
# look for come from the source, so a new one needs no change here.
grep -E 'failed to restore (queued|suspended) task|dropped suspended task: saved program does not match its source' "$out" > "$work/refusals.txt" || true
grep -rhoE --include='*.go' --exclude='*_test.go' '(Errorf|errors\.New)\("[^"%\\]+"\)' "$repo" |
  sed -E 's/^[^"]*"//; s/"\)$//' | sort -u > "$work/reasons.txt"
awk '
  FILENAME == ARGV[1] { reasons[$0]; next }
  {
    best = ""
    for (reason in reasons)
      if (index($0, reason) && length(reason) > length(best))
        best = reason
    count[best]++
  }
  END {
    for (reason in count)
      print count[reason] " not restored: " (reason == "" ? "(reason is not a fixed string in the Go source; not shown)" : reason)
  }' "$work/reasons.txt" "$work/refusals.txt" | sort

if [ "$status" -ne 0 ]; then
  echo "census failed: exit $status. The rest of its output is not shown because it names players' objects, verbs and tasks."
  exit 1
fi
if ! grep -q 'checkpoint restored: queued=' "$out"; then
  echo "census printed no summary; it did not audit the checkpoint."
  exit 1
fi
