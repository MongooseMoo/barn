# Report fuzz findings immediately

## Goal

Q: "Can we fix the reporting delay in a pr that will get picked up on the next
cycle?" A native lane found bugs at 134 s and 202 s (run 42750434626), but
`Open failure issues` is a later Actions step, so issues 687/688 and their
repair dispatch waited for the 30-minute lane to end.

## State

Branch fix/fuzz-report-immediately from origin/master. Implemented. Locally
test_fuzz_report.py (26) and test_fuzz_compute.py (20) pass. CampaignStepTests
run fuzz.yml's own campaign and reporter scripts in bash with a fake `go` and a
local fake GitHub API: report and dispatch happen before attempt 2, the issue's
base64 block decodes to the exact record bytes and sha256, no double report at
job end, a failed report prints ::error and is retried at job end, and an
oversized record is reported at job end with the artifact. The test also checks
that GITHUB_TOKEN does not reach `go test`. PR CI next.

## Decisions and why

- One reporting owner: `scripts/fuzz_report.py --publish` (Python, urllib, the
  job's token). The inline github-script reporter is deleted. A shell loop
  cannot call an Actions step, and node is not on PATH for shell steps on the
  self-hosted runners, so the owner the loop already calls carries the posting.
- The issue carries the exact saved record base64-encoded with its sha256, so
  the repair agent has it when dispatched. A shell step cannot upload an
  artifact (the artifact runtime token reaches only JS actions), so the
  artifact cannot exist yet. Go writes records as text (`%q` encoding,
  internal/fuzz/encoding.go); the largest in this cycle's artifacts was 2991
  bytes.
- A record over RECORD_LIMIT (30000 bytes, about 40 KB base64, under GitHub's
  65536-character body cap) is not reported in the loop. The end-of-lane step
  reports it with the uploaded artifact, as before.
- The loop renames a reported log to `reported-NNN.log`. The end-of-lane step
  publishes only `run-*.log` and skips signatures already reported in the job.
  A failure to report in the loop leaves the log as `run-` for the end step and
  prints an ::error. Discovery continues either way.
- The token is removed from `go test`'s environment (`env -u GITHUB_TOKEN`).

## Surprises

- Lost behaviour: if checkout itself fails, the reporter script is absent, so
  no infrastructure issue is opened. The step fails with an ::error, and
  acceptance rejects the cycle as it did before.

## Next step

Tests, PR, CI, merge. Then watch the next cycle report a finding mid-lane.
