# Barn observability research checkpoint

Date: 2026-08-12

## Current findings

- Barn already has structured `slog` logging, text plus per-run JSONL sinks,
  `cmd/barn_logs`, runtime log-level mutation, local pprof/expvar, an expvar
  `metrics` package, and detailed opt-in MOO execution tracing.
- The current debug mux combines passive expvar data, privileged/expensive pprof
  and runtime-trace handlers, and a mutating log-level endpoint.
- Existing Barn business metrics are package-global expvar variables. Live
  gauges silently remain attached to the first server in a multi-server test
  process because duplicate publication is ignored.
- Current metric names and shapes are not a Prometheus contract: dotted names,
  millisecond last-value gauges, no HELP/TYPE metadata, no duration
  distributions, and no conventional Go/process scrape surface.
- Current detailed tracing includes MOO values, messages, player IDs, and remote
  address details; it should remain explicit local diagnostics rather than
  always-on exported telemetry.
- Current external guidance supports: structured logs; Prometheus counters,
  gauges, and histograms with base units and bounded labels; separate cheap
  liveness/readiness probes; stable OpenTelemetry Go traces/metrics but beta
  logs; and privileged Go pprof/runtime flight-recorder diagnostics.

## Recommendation state

1. First split a passive operator HTTP listener (`/metrics`, `/livez`,
   `/readyz`) from the privileged `--debug-addr` mux.
2. Then replace global expvar business metrics with a per-server Prometheus
   registry and low-cardinality golden signals for tasks/commands, connections,
   checkpoints, GC, and panics.
3. Later stabilize log vocabulary/redaction and correlation.
4. Add opt-in coarse OpenTelemetry traces only after observation/context
   ownership is explicit; do not add default per-verb spans.
5. Evaluate Go's flight recorder only after measuring workload overhead.

## Artifacts and state

- Added uncommitted report:
  `reports/research-go-server-observability.md`.
- The checkout was already heavily dirty with unrelated untracked files; none
  were modified or cleaned.
- No GitHub issue has been opened yet.
- No code, tests, configuration, commits, branches, or pull requests were
  changed.

## Blocker

No technical blocker. A mandatory notes checkpoint interrupted work immediately
after the report was added.

## Next action

Review the report and live issue inventory, draft and audit two narrow issue
bodies, then open:

1. split passive operator endpoints from privileged diagnostics;
2. publish per-server Prometheus golden signals with bounded cardinality.

Keep OpenTelemetry, dashboards, MOO-visible builtins, conformance behavior, and
broad logging rewrites out of those first issues.

## Follow-up tracker update

Date: 2026-08-12

### Findings and observations

- Confirmed `server/input_login.go` converts the entire pre-authentication login
  line into `do_login_command` arguments.
- Confirmed default execution tracing stringifies complete MOO arguments and
  returns, records `notify()` text, and can include remote-address details.
- Confirmed `TRACING.md` recommends tracing `do_login*` verbs, creating a direct
  ordinary-workflow path for recording credentials.
- No existing issue covered trace credential or payload non-disclosure.

### External tracker state

- Opened issue #209, `security: default execution tracing records login
  credentials and private MOO values`, labeled `bug`.
- Amended issue #208 with explicit metric semantics for:
  - `barn_checkpoint_last_success_timestamp_seconds` and checkpoint freshness;
  - open sockets versus authenticated connections versus unique players;
  - active run time, queue wait, and optional end-to-end foreground command
    latency across suspend/resume boundaries.
- The #208 amendment links payload non-disclosure work to #209.
- The first #208 edit attempt was rejected by `gh` because PowerShell split a
  multiline `--body` argument. The issue was unchanged. Retrying through
  `--body-file -` succeeded and live verification confirmed every new section.

### Current state

- Issues #207, #208, and #209 are open.
- No source code, tests, branches, commits, pull requests, labels other than the
  intended `bug` label on #209, or unrelated workspace files were changed.
- The research report and this checkpoint note remain untracked and must not be
  staged, committed, or pushed as part of issue work.

### Blocker and next action

No blocker. The requested tracker work is complete. Implementation should begin
with #209 if credential exposure is prioritized, otherwise #207 must precede
#208. Do not start implementation without a new explicit request.
