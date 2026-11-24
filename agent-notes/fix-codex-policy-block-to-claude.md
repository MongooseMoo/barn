# Codex policy block hands the repair to Claude

## Goal

Q 2026-11-24 chose "Hand off to Claude": a Codex policy block on a repair,
conformance or investigation item dispatches Claude unless Claude already
failed on it; only then does the item get the attention label.

## State

- Cause: scripts/repair_worker.py treated `policy-block` as terminal and
  labelled `repair-needs-attention`. #1124 (Codex cyber-policy refusal,
  2026-11-22) got no repair for two days; a hand-dispatched Claude repair is
  run 45437185717.
- Change: `policy-block` takes the outage path (codex-unavailable label,
  codex-<stage> removed, claude-fix.yml or alarm-investigate.yml dispatched
  with provider=claude). With `claude-unavailable` already on the item it
  gets the attention label as before.
- scripts/test_repair_worker.py: 3 tests; with test_repair_provider 8 OK in
  WSL.

## Next step

After merge: fast-forward /opt/barn-repair in WSL to master and restart the
worker process (python3 /opt/barn-repair/scripts/repair_worker.py, running
since 2026-11-21), so the new code is live.
