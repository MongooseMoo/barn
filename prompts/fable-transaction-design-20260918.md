# Challenge the research-backed Barn transaction scheduling design

Use Claude Fable as requested. Read-only source/design review; write ONLY
`reports/fable-transaction-design-review-20260918.md`. Do not modify code,
other reports, notes, or Git. You are not alone in this workspace.

Read `plans/barn-transaction-scheduling-design.md` in full. Read companion
`reports/research-barn-cooperative-mvcc.md` and, if present,
`reports/research-barn-transaction-contention.md`. Current baseline e86ecd0.
Earlier gate proposals are superseded by the new design.

This is a bounded architectural challenge, not another broad investigation.
Inspect source only to resolve a concrete design issue. No runtime/conformance
tests, no independent agents, no implementation. Finish the report in one
review pass. Review the actual protocols, not just whether names sound good.

Questions to answer with explicit timelines/counterexamples:

1. FIFO compatible shared cohorts: can new gate traffic starve an older
   background/promotion/checkpoint request? What assumptions are needed?
2. Reserved executor before a known-exclusive ticket, provisional physical
   lease before claiming, offer not ownership: does this prevent worker/gate
   inversion? Can a GC hook and unclaimed offer deadlock? Evaluate the proposed
   maintenance handoff including active attempts requiring later gate requests.
3. Weighted normalized occupancy, one active slice per principal, class shares:
   can dependencies/self-calls or uncharged waits violate progress? Is any
   claimed fairness guarantee too strong? Be precise about same-principal work.
4. Nested capability borrowing and cancellation: can cancellation release an
   active holder, can a stale claimant start, can release happen twice?
5. Current MVCC boundary/read-only/direct/WAIF limitations: does the design
   honestly distinguish preserved behavior from stronger safety claims? Which
   integration contracts are missing before implementation?
6. Does it answer the user's question with an implementable researched design,
   or unnecessarily invent a new algorithm? Suggest the smallest corrections.

Report: decisive recommendation, ranked concrete findings with section/source
pointers, a timeline per serious issue, and corrected contracts. Distinguish
first fair-admission implementation from later rollback/fine-grained STM.
Do not endorse hard real-time bounds, exactly-once external delivery, or
whole-server serializability without proof. No runtime changes are requested.
