# GC Barrier Design Audit Checkpoint

- State: Read-only adversarial audit assigned for Barn PR96 at current head `cc436313`.
- Scope: Enumerate all scheduler VM execution-entry paths and all anonymous/waif sweep paths from current source only; no historical Markdown and no manual runtime launch.
- Design under review: `gcSweep`/start mutex handshake, reentrant `run_gc`, execution leases around command and verb entrypoints, inherited nested-call leases, recycle-hook sweep ownership, and per-task floor GC under concurrent siblings.
- Current observation: The audit must establish an actual happens-before boundary between root accounting and recycle, not merely avoid Go data races.
- Blocker: None yet; code-path enumeration has not begun.
- Next action: Inspect current branch identity and use `rg` over Go sources to map scheduler entrypoints, VM execution, GC/recycle calls, locks, and task ownership.
