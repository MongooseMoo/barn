# Retain/fix checkpoint 15

- moo-conformance-tests PR #75 normal-merged as `49f08f5b8b025109ab9885ea90f60756b0faae16`; verified two parents and issue #71 closed.
- Barn PR #96 head `47aefbbecd1cbbc13996c879176e606e18a2c132` remains CODE BLOCKED: logical `TaskRunning` ends inside `suspend()` before VM execution/handoff ends, so GC collectors can still scan or omit an executing VM.
- Required repair is a scheduler-owned physical execution lease, distinct from logical task state, held under `s.mu` for the full `runTask` VM-mutation interval. Global GC must no-op around an active sibling; per-task GC must skip it; deferred flush must fail closed.
- Replace the manufactured running-state test with a real barrier during suspend intent before VM handoff, and prove no sweep during the lease plus ordinary sweep after release.
- PR #115 must not absorb #96 until this repair passes red/green, race/full gates, and a fresh exact-head independent review.
- Current legacy PR #96 conformance failure is 82 failures: 71 `ONLY_32_BITS` admission cascades, 8 bcrypt cases, and 3 cyclic-GC cases. The corrected integration branch owns those base repairs.

