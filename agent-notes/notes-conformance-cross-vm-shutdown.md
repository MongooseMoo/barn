# Cross-VM shutdown conformance work

- Scope: add one moo-conformance-tests regression for two VMs holding opposite members of one anonymous cycle across shutdown/restart.
- Ownership: conformance-suite files only, in a new isolated worktree from current `origin/master`.
- Constraints: managed pinned WSL Toast first; no manual servers or tracked-database repros; preserve unrelated work; do not merge.
- Current state: task accepted; no repository inspection or edits performed yet.
- Blocker: none.
- Next action: locate the documented managed conformance entrypoint from current code/config, create the isolated worktree, and identify existing anonymous-finalization cases.

## 2026-08-06 checkpoint

- Isolated suite worktree: `C:\Users\Q\code\.worktrees\moo-conformance-cross-vm-shutdown`, branch `agent/cross-vm-shutdown-conformance`, based on suite `origin/main` at `3cc62c8`.
- Managed pinned WSL Toast command is working through Barn's `scripts/run_toast_wsl.sh` with `/root/src/toaststunt/build-release/moo`.
- Rejected design 1: explicit `dump_database()` plus managed restart/resume passed Toast but serializes suspended tasks before shutdown, so it does not cover shutdown finalization.
- Rejected design 2: an early-exit fixture that creates A/B in `server_started` passed Toast and PR115 `f00082e`; Barn's shutdown caller VM retained/masked the anonymous roots.
- Target design: generate a fixture checkpoint under managed Toast where A/B begin in two #0 properties. On boot, separate no-arg holder verbs load and delete one property each, then suspend. `server_started` never receives either anon, waits for readiness, runs GC, and calls shutdown. Assertions require exact newline-delimited `1 values pending finalization`, exact `0 suspended tasks`, and unique A/B recycle payloads.
- Current generation state: managed Test.db generator successfully creates the prototype, A/B cycle, persistent roots, and holder verbs. Final setup failed with `E_VERBNF` because Test.db lacks `#0:server_started`; generator now adds that verb when missing before setting code.
- Barn comparison heads: current PR115 `f00082e` must be red; green must use the forthcoming PR96 lifecycle-repair head, not the existing store-normalization head.
- Blocker: none; one generator rerun remains.
- Next action: rerun managed Toast generator, adopt its disposable checkpoint as Anon7, prove Toast green/current PR115 red/new lifecycle head green, then run suite fixture/unit/ruff/mypy/duplicate gates and create focused issue/draft PR without merging.
