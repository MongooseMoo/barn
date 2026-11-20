# PR 96 GC transaction design review

## Current findings

- Architecture verdict: use a transaction-aware global anonymous-GC walker; do not flush all staged task writes to the live store before `run_gc`.
- The walker must see the `StoreTxn` view: valid numbered snapshot objects plus staged numbered creates minus staged recycles; effective staged property values/deletes on numbered and anonymous objects; and anonymous candidates visible/adopted by the transaction.
- Candidate discovery should be one transaction API returning a frozen candidate slice after expanding persistent and VM roots. Separate live-store calls can mix views.
- A future staged/atomic anonymous-recycle path also needs a phantom guard. A numbered object created after the sweep can add a persistent root that per-object property scan validation cannot see. Validate a persistent-root generation or conservatively the store commit clock.

## Critical observation

- Anonymous recycle is not currently transaction-atomic. `builtinRecycle` excludes anonymous objects from `StoreTxn.RecycleObject`, optionally calls `flushStagedBeforeCoarse`, then calls live `Store.Recycle` and marks the task live-mutated. Hooks receive the transaction, but destruction does not stage in it.
- Anonymous create is likewise a coarse live mutation and adopts the result into the transaction.
- Therefore a transaction-aware walker fixes stale reachability, but atomic rollback requires a staged anonymous lifecycle path as separate work.

## State and next action

- No source edits made. Existing unrelated dirty `uv.lock` and untracked scheduler test remain untouched.
- Next: report the exact API/validation requirements and the current coarse-recycle contradiction to the parent agent.
