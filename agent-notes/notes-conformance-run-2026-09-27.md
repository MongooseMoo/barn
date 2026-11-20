# Conformance run vs latest suite — 2026-09-27

## State
- moo-conformance-tests: origin/main 7f05b70 (latest; landed 07:59Z, after bd2befe CI at 07:24Z).
- barn: master = origin/master bd2befe; built bin/barn.exe and
  bin/barn-linux-amd64 (CGO_ENABLED=0 GOAMD64=v3, per Makefile).

## My invocation mistakes (not Barn bugs)
- Copied CI's `--oracle-profile-manifest` onto a Windows run -> profile gate
  aborted (linux vs windows). Q: nobody asked for a comparison. Drop it on Windows.
- Added `--checkpoint-interval 0` (from profiles.json template; CI does NOT pass
  it). Caused all 12-14 startup_repair_anon*/pending_waif/fixture_provenance
  failures (no `<db>.new` startup output). Without it: 16/16 pass on Linux.
- Conformance `.venv` is a stale WSL stub; use UV_PROJECT_ENVIRONMENT outside
  the checkout (Windows: scratchpad/conf-venv; WSL: /root/.cache/moo-conformance-venv-linux).

## Linux (WSL, CI-equivalent) result
- 14 failed / 12656 passed / 420 skipped, all 14 from --checkpoint-interval 0.
  Rerun of those without the flag: 16 passed. => Linux effectively clean.

## Windows result (no oracle manifest, with the bad checkpoint flag)
- 89 failed / 12607 passed / 394 skipped. Log: scratchpad/win-full.log, win.xml
- ~60 fail with CapabilityProbeError: "Failed to probe option ONLY_32_BITS:
  neither supported server_version option path is available". Without the
  manifest the harness probes server_version("options/ONLY_32_BITS").
- REAL DIVERGENCE (Toast source, version.cc + build/version_options.h):
  Toast always lists ONLY_32_BITS in the options group; 64-bit build yields
  `#-1` (_DNDEF). Barn builtins/system.go:547 options list lacks it -> E_INVARG.
- Others to triage: memory_usage E_FILE (Windows has no /proc; likely platform),
  math x5, divergence_builtin_edges x3, time_server_builtins, divergence_introspection.

## Triage done
- memory_usage E_FILE x3 on Windows: Toast returns E_FILE when /proc/self/statm
  can't be opened (server.cc bf_memory_usage). Barn matches. Unix-only tests; no fix.
- Only real divergence: ONLY_32_BITS.

## Fix in progress
- Worktree ../barn-only32, branch fix/server-version-only-32-bits off bd2befe.
- builtins/system.go: options group gains ONLY_32_BITS -> #-1.
- builtins/runtime_options_test.go: TestServerVersionReportsOnly32BitsUndefined
  (red before, green after). go test ./builtins green.
- Built bin/barn.exe + bin/barn-linux-amd64 in worktree. Next: go test ./...,
  Windows suite (no oracle/no checkpoint flag), Linux suite, then PR.

## DONE
- Commit 14a2a3e, PR https://github.com/MongooseMoo/barn/pull/366
- Linux (CI-equivalent): 12670 passed / 420 skipped / 0 failed.
- Windows (no manifest): 3 failed (memory_usage /proc, Toast-equivalent) / 12667 passed / 420 skipped.
- go fmt/vet/test ./... green.
- Not done: no new conformance YAML pinning ONLY_32_BITS (harness probe already exercises it).

## Next (original)
1. Triage remaining Windows-only failures.
2. Worktree: add ONLY_32_BITS -> #-1 to Barn's server_version options + unit test.
3. Conformance test pinning server_version("options/ONLY_32_BITS") on Toast (separate repo commit).
4. Rerun Windows suite (no oracle, no checkpoint flag) + Linux suite; PR.
