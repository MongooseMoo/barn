# Nightly fuzz capacity: workers, lane minutes, guest RAM

## Goal

Q 2026-11-11, "make it so" on the capacity plan: stop the fuzz guest from
running at load 40 on 24 vCPUs and OOM-killing lanes, and spend lane time by
value.

## State

Repo changes on branch `ci/fuzz-capacity`, not yet pushed:
- `.github/workflows/fuzz.yml`: `go test -fuzz` runs `-parallel=4` (was 6).
  Each matrix entry has `minutes`; the job exports it as `FUZZ_MINUTES` and the
  campaign deadline is `FUZZ_MINUTES * 60` (was a fixed 1800 s). The restart
  after a finding and `-timeout` = remaining + 600 are unchanged. Job
  `timeout-minutes: 45` still covers 30 + 10 + setup.
- Five-minute lanes: FuzzTCPReadInput, FuzzSuspensionCompletionHistory,
  FuzzMapUpdatesPreserveSnapshotsAndInsertionOrder,
  FuzzStructuredParseFormatRoundTrip, FuzzVerifyProgram,
  FuzzParseHTTPChunkedBody (coverage +0..+6 new inputs per 30 min, never a
  bug). All others 30.
- `.github/runner/barn-compute.service`: `-m 32768`, `MemoryMax=36G`.
  `.github/runner/barn-fuzz.slice`: `MemoryMax=30G`. README and
  ops-mongoose-compute.md updated (the ops note said 8 vCPUs; the guest runs
  `-smp 24`).
- Tests: `scripts/test_fuzz_compute.py` checks every lane's minutes and the
  job limit; `scripts/test_fuzz_report.py` runs the campaign step with
  `FUZZ_MINUTES=5` and checks each attempt's `-fuzztime`, `-timeout` and
  `-parallel=4`.

The live host and guest are changed by the orchestrator, not by this branch.

## Decisions and why

- Evidence for the change: guest load average 40 on 24 vCPUs with 4 lanes x 6
  workers; `barn-fuzz.slice` memory.events `max 57084860`, `oom_kill 3` in
  under 16 h, all FuzzReadDatabase (format.test about 1.8 GB).
- #771, #772, #773 (run 43087028220): not ours. The "Upload retained corpus
  and native evidence" step of four lanes failed between 20:27 and 20:38 UTC
  with `Unexpected token '<', "<!DOCTYPE "... is not valid JSON` three times,
  then `Failed to CreateArtifact: Received non-retryable error: Failed
  request: (404) Not Found: workflow run not found`. githubstatus.com lists a
  Webhooks incident from 20:22 and a critical Actions/Pages/Copilot incident
  from 20:48 to 21:21 UTC the same day. Nothing changed for them.

## Surprises

- FuzzVerifyProgram's job log for that run is gone (`log not found`,
  BlobNotFound), consistent with the same GitHub incident.

## Next step

Push, PR, merge; orchestrator applies the host/guest memory change, then check
the next cycle's slice `memory.events` and guest load.
