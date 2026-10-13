## 2026-09-17 issue 238 checkpoint
- Isolated implementation: C:/Users/Q/code/barn-issue-238, branch fix/issue-238-generate-json; publish PR, no merge authority.
- Managed canonical WSL Toast probe: 9 passed; unchanged Barn probe: 6 failed, 3 passed. Go regression also fails with E_ARGS and accepted invalid modes.
- Probe is .tmp/issue238.yaml; clean harness checkout C:/Users/Q/code/moo-conformance-issue-238 at 9bdfd2e; WSL uv environment /tmp/barn-238-conformance-venv.
- Implement strict case-insensitive modes and recursive third-argument binary-escape switch, then focused checks, final CI, and PR. No active blocker.
