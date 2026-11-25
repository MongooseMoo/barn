# Ordered parent lookup for callable verbs

**Goal** Match MOO's left-to-right depth-first callable lookup for ordinary calls and `pass()`.

**State** The focused Go regression passed direct call, pass, and memoized call-site cases with the fix. Focused managed sessions selected capability admission and the new audit case. Stock Toast and fixed Barn each reported `2 passed`. Baseline Barn reported `1 failed, 1 passed` after returning `[right, right, right, right]` where the test requires `[deep, right, deep, right]`. The pinned Elle workflow exited 0 after accepting both healthy models for seeds 412, 395 and 396 and rejecting all 20 deliberate isolation faults. The manifests reported 454, 421 and 453 verb memo observations and one dispatch-reparent observation per seed. The checkpoint replay and lifecycle control also produced their expected verdicts.

**Decisions and why** Both walkers now reuse a stack and push parents in reverse list order. Keep visit deduplication, execute filtering, scan marks, resolved-verb reads, and memo paths in their existing owners. Parsed commands have a separate search and argument-matching path outside the callable regression.

**Surprises** The existing parent-order cases use immediate parents only and cannot expose a grandparent losing precedence to a sibling.

**Next step** Publish the fix branch for review.
