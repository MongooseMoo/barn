# Mongoose external completion and login latency

Historical command update (2026-09-21): `test-mongoose-deltas.ps1` has been
retired. Repeat generic checks through the
[managed conformance workflow](../docs/mongoose-conformance.md), using stock Toast.

Completed external calls could enter Barn's next ready batch behind newly
admitted forks. The WSL Mongoose Toast oracle admits those completions first,
while preserving siblings that were already admitted to its ready queue.
Barn now promotes completed external calls within the next batch and preserves
the existing order of ordinary work. It does not interrupt an executing batch.

The new conformance cases were checked against WSL Toast before changing Barn.
The initial Barn reduction returned `[busy,new,exec]` instead of
`[busy,exec,new]`. Coverage includes a fork created before helper completion,
one created afterward, and a sibling already admitted to the ready queue.
Each helper must exit successfully. No existing conformance expectations were
weakened. The tests are committed in the separate conformance checkout as
`3ba427e` on `fix/mongoose-workload`.

`exec()` now logs command duration, and resumed tasks log completion-to-dispatch
and completion-to-VM delay. Command arguments and input are omitted. The new
`scripts/summarize-mongoose-exec.ps1` pairs repeated calls by task in log order.

The account probe responds to username/password prompts and the MOTD marker
instead of sleeping between credentials. JSONL events distinguish prompt phases
and failed connection hooks. `-FixedDelays` preserves the earlier probe mode.
The clean-login retry mode closes promptly on successful MOTD; failed hooks keep
the receive timeout, followed by `-RetryDelay`, to avoid rapid reconnects during
startup. Character-chooser handling still requires the documented fixed-delay
mode and explicit character selection.

## Live measurements

Both runs used disposable copies of the same previously fetched snapshot,
`-promote-numbers`, port 7777, native timezone helper, and the q account. No new
remote snapshot was fetched for this comparison. The baseline includes timing
instrumentation with the old scheduler. Evidence is under
`.tmp/mongoose-resume-20260918`: `baseline-runtime.jsonl`, `final-runtime.jsonl`,
`baseline-warm-probe.txt`, and `final-warm-single-probe.txt`.

| Measurement | Baseline | Fixed |
| --- | ---: | ---: |
| Password to account welcome | 18 ms | 17 ms |
| Password to MOTD | 34,172 ms | 30,541 ms |
| Login timezone helper duration | 136.4403 ms | 19.224 ms |
| Login helper completion to dispatch | 10,787.8343 ms | 183.5933 ms |
| Explicit UTC helper completion to dispatch | 13,702.4559 ms | 3,551.9166 ms |
| Explicit Denver helper completion to dispatch | 2,836.0233 ms | 10,078.7095 ms |
| Explicit UTC plus MOO Denver probe, real time | 16.6998 s | 13.6764 s |

Both probes returned `{"mongoose-time-offset", {0, "+0000", ""}, -21600}`.
These are individual observations on evolving worlds, not a controlled latency
distribution. The remaining ten-second wait and thirty-second login show that
this ordering repair does not solve responsiveness overall. Existing ready-batch
work and the continuously busy simulation still need investigation.

Cold login initially hit the known SQL-not-open hook failure. Experimental rapid
retry runs failed to obtain a clean login within 300 seconds and were excluded
from the comparison; their cause is not established. A probe overlapping the
first connection hit the character chooser and was also excluded. The retained
fixed measurement followed closure of the initial connection. Barn PID 26992
was left running on port 7777 at the end of these measurements.

## Validation

- `go test ./... -count=1`: passed on the final scheduler implementation.
- `go test -race ./engine/... ./task -count=1`: passed, engine 194.405 s.
- `go test -race ./cmd/moo_client -count=1`: passed after the final client edit,
  3.625 s.
- Managed WSL Toast `server/exec_resume_order.yaml`: 4 passed in 13.15 s,
  including capability admission.
- Managed Barn same suite plus `fork/ready_sibling.yaml`: 5 passed in 8.86 s,
  including capability admission.
- Conformance commit hooks: duplicate lint and profile contract guard passed.

Reproduce the focused oracle check with `scripts/test-mongoose-deltas.ps1`
using `-Engine Toast -Packaged -ConformanceRoot .tmp/conformance-mongoose-workload
-RunDir .tmp/mongoose-resume-20260918 -Suites server/exec_resume_order.yaml`.
Repeat with `-Engine Barn` and the freshly built Barn binary. This is a focused
regression run, not a full conformance run.
