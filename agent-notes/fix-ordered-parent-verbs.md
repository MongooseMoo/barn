# Ordered parent lookup for callable verbs

**Goal** Match MOO's left-to-right depth-first callable lookup for ordinary calls and `pass()`.

**State** The focused Go regression passed direct call, pass, and memoized call-site cases with the fix. Focused managed sessions selected capability admission and the new audit case. Stock Toast and fixed Barn each reported `2 passed`. Baseline Barn reported `1 failed, 1 passed` after returning `[right, right, right, right]` where the test requires `[deep, right, deep, right]`. The pinned Elle workflow exited 0 after accepting both healthy models for seeds 412, 395 and 396 and rejecting all 20 deliberate isolation faults. The manifests reported 454, 421 and 453 verb memo observations and one dispatch-reparent observation per seed. The checkpoint replay and lifecycle control also produced their expected verdicts.

**Decisions and why** Both walkers now reuse a stack and push parents in reverse list order. Keep visit deduplication, execute filtering, scan marks, resolved-verb reads, and memo paths in their existing owners. Parsed commands have a separate search and argument-matching path outside the callable regression.

**Surprises** The existing parent-order cases use immediate parents only and cannot expose a grandparent losing precedence to a sibling.

**Merge-queue investigation** Barn PR #1171's `Jepsen checkpoint` merge-group run 45706772819 failed only on the `hot-memo-guard` negative control. The ordinary campaign and the stage schedule passed. The server recorded five fault firings, but Elle accepted both resulting histories. The pinned Elle workflow passed on the same merge group and separately requires a witness from its held hot-memo renewal pair. Raw Jepsen evidence is available in the failed run's stage artifact.

**Control decision** The Nov. 23 scheduled run 45203664480 failed on `master` with the same missing witness and seven fault firings. A firing records renewal before the outcome of the task is known. The Jepsen schedule gives a randomly selected observer a chance to read across a relevant change. The stage job now calls the existing pinned Elle control, which holds the reader across its mutator's publication and requires Elle to identify the stale dispatch in the retained two-row history. The live control is still available in the Jepsen harness, and the other live controls still run in CI.

**Next step** Verify the amended PR through the Jepsen and pinned Elle workflows.
