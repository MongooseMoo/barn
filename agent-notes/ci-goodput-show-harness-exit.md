# Goodput series hides a failed harness run's verdict

## Goal
When a `TestMongooseRealWorkload` run fails in the "Goodput series"
workflow, the log must still show that run's numbers, its exit status and the
`::error` line.

## State
Fixed in `.github/workflows/goodput-series.yml` ("Run the harness"); PR open.

- The fault: GitHub runs the step with `bash -e {0}` (shown in the step log).
  `set -uo pipefail` does not turn `-e` off, so a nonzero exit from
  `engine.test` ended the step at that line: `status=$?`, the numbers grep,
  `run N: exit ...` and the `::error` line never ran. Run 43563381089 (PR 813's
  branch) shows exactly that: nothing after the env block, then "Process
  completed with exit code 1" 157 s in. The grep pipeline had the same fault:
  with no match, grep exits 1 and pipefail plus `-e` end the step.
- The fix: `status=0; engine.test ... || status=$?` and `grep ... | sed ... ||
  true`. Probed with `bash -e -c` on both forms: the old form exits 1 with no
  output, the new one prints `status=3` for a command exiting 3.

## Decisions and why
- No new output from the harness: the log carries numbers and the verdict
  only, as before, because the rest can name players.

## Next step
None after merge. The cause of the run 43563381089 failure stays unknown: its
output was deleted, five local reruns of that code passed, and 15 load-gated CI
runs on master after PR 813 merged all passed.
