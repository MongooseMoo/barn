# Current campaign checkpoint 24

- Isolated conformance worktree: `C:\Users\Q\code\.worktrees\moo-conformance-shutdown-gc-edges`, branch `agent/toast-shutdown-gc-edges`, based on conformance `origin/main` at `edb753d22fd5011e24a48da957d8d8aca2ffa361`.
- Managed WSL oracle only: `/root/src/toaststunt/build-release/moo` launched by the conformance pytest managed-server command; no manual Toast or Barn process was launched.
- Generated three disposable startup DB fixtures through managed Toast checkpoints, then removed the generator suite from the intended diff.
- Confirmed case 1: a direct pending WAIF's `:recycle` hook logs entry, `shutdown()` returns to the hook, logs return, exits cleanly, and writes `0 values pending finalization`.
- Confirmed case 2: panic via `shutdown(message, 1)` while a suspended task owns a WAIF and anonymous object writes a panic DB with `0 values pending finalization` and `1 suspended tasks`; this is publicly expressible. Emergency command-line mode was not used.
- Confirmed case 3: a single direct pending WAIF containing a nested WAIF writes `1 values pending finalization`; the DB topology is one top-level `c 0` WAIF with nested `c 1`, not two top-level pending roots.
- Focused managed oracle result: the three final regression cases passed (`3 passed in 10.50s`) on the 64-bit outbound WSL Toast profile.
- Intended files: three startup DB fixtures, checksum manifest updates, and three server YAML suites. No Barn changes and no PR 22 changes.
- Current blocker: none. Full harness checks, complete three-profile Toast CI, issue creation, commit, push, and draft PR remain.
- Next action: run checksum/schema/duplicate/unit/type/lint gates; inspect exact diff; open a narrow issue only for these confirmed behaviors; commit/push explicit paths; open draft PR with `Closes #N`; wait for all Toast profiles and report exact head/evidence.
