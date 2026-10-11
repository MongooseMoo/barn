# Issue 14 and 16 correction checkpoint

- Scope: uncommitted combined worktree `C:/Users/Q/code/.worktrees/moo-conformance-issue-14`; no commit or push.
- Managed oracle: canonical WSL Toast `/root/src/toaststunt/build-release/moo` at `aecc51e9449c6e7c95272f0f044b5ba38948459e`.
- Dynamic probe oracle evidence: fresh wizard `connectable_listener_port` and `ephemeral_listen` guarded cases each passed; after a programmer case each capability was incorrectly collapsed to absence and skipped. Reports: `/tmp/issue14-connectable.5h5NlU`, `/tmp/issue14-ephemeral.LswgC2`, `/tmp/issue14-wizard-buffered_output_length_increases_after_notify.8OGzF8`, `/tmp/issue14-wizard-listeners_listen_unlisten_roundtrip.UK7CYl`.
- Option oracle evidence from a temporary managed YAML probe, now deleted: dot keys for `ONLY_32_BITS` and `OUTBOUND_NETWORK` raise `E_INVARG`; slash `ONLY_32_BITS` returns `#-1`; slash `OUTBOUND_NETWORK` returns `ON`. Report: `/tmp/issue14-option-shapes.WVG0SE`.
- Implemented: source-advertised ordinary pytest command now uses repository-root pytest with no explicit `tests/` path; no duplicate owner added. Added documentation regression plus existing subprocess parity coverage.
- Implemented: live option responses accept only exact `ON`, `OFF`, or `#-1`; malformed successful values and absence of both supported option paths raise `CapabilityProbeError`. Width detection uses the closed option result.
- Implemented: all executed admission probes switch to wizard through one probe wrapper; switching or execution exceptions fail with probe context. Dynamic probe MOO code no longer catches `ANY`; connectable capability is explicitly false when `OUTBOUND_NETWORK` is explicitly false and otherwise runs the direct probe.
- Added order regressions for connectable and ephemeral probes after a programmer identity, malformed option regressions, dynamic error regression, and updated fake transports.
- Focused tests after corrections: 118 passed in 2.25 seconds.
- Environment observation: managed WSL created a Linux `.venv` link that Windows uv cannot replace (`Access is denied`). Windows verification uses task-specific `%TEMP%/moo-conformance-issue14-win-venv`; tracked files are unaffected.
- Current blocker: none known.
- Next action: run focused managed post-fix ordering and 32/64 probes, then full units, targeted Ruff, mypy, diff check, full ID/parity checks, and report exact outcomes.
