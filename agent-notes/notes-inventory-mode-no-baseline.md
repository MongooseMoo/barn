# Inventory mode ledger prerequisite

- Finding: The existing strict schema-v2 inventory loader was sound, but the CLI still required a baseline and accepted inventory plus baseline.
- Current state: `--inventory` and `--baseline` are now a required mutually exclusive group. Inventory mode derives the exact candidate identity surface and supplies an empty exception map; legacy mode still discovers the local surface and loads the required baseline. Three explicit fail-closed CLI contracts were added while tampered-inventory validation remains exercised in inventory-only mode.
- Verification: The intended tests were red before implementation (4 failures), then green (4 passed). The ledger suite is 56 passed; the full unit suite is 368 passed; focused Ruff is clean; focused mypy is clean after syncing the locked dev extra; `git diff --check` is clean.
- Blocker: None.
- Next action: Report the exact two-file diff and verification evidence to the parent without committing or pushing.
