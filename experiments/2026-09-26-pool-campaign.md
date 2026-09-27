# Temporary allocation reuse campaign

Baseline: `3c0f5105e30dd557734deac200ff0fec51faae8f`.

User scope: investigate all four opportunities and retain every independently worthwhile improvement, then measure their combination. This is not a competition selecting a single winner. PR publication is authorized; merging is not.

Candidates: decentralized commit scratch, unique bookkeeping, file_read byte buffers, and finalization scratch. Prefer removing unnecessary scratch or owner-local reuse when simpler than sync.Pool. Exclude whole StoreTxn pooling and published object/result storage.

Prior-art search: searched experiments, reports, and plans for sync.Pool and candidate names. The main checkout's untracked 2026-06-24 commit-dominated ledger records historical VM/transaction experiments and rejected finalizer-clear changes; these used obsolete scheduler paths and are not current measurements. Current vm/pool.go already pools call-local VMs; transaction ancestry and frame scratch already have owner-local reuse. No current measured records for these four specific proposed changes were found in that search.

Budget: four independent candidate experiments, with at most one evidence-driven refinement per candidate. Stop each candidate on a safety failure or a fully diagnosed failed metric gate. No unrelated optimization expansion.

Each worker commits benchmark and correctness contracts before a frozen preregistration and source changes. Ten interleaved baseline/candidate samples; allocation bytes are the primary metric (minimum 10 percent reduction on preregistered representative rows), runtime regression guard of 5 percent with significance, before/after profiles, bounded retention, and correctness tests. Exact commands and evaluator hashes belong to each candidate record. Small workload regressions count; large-input improvements alone do not justify broad unconditional pooling.

Measurement runs are serialized by the parent. Windows Go 1.26.0, AMD Ryzen 9 5950X; no claim of fixed clock frequency or production latency. Record machine noise and distinguish microbenchmarks from application behavior.

Reserved holdouts: existing commit object-ordering benchmark; unique mixed Unicode 257-element case; file_read binary 8192 high escape density; existing VM frame-scan benchmark. Independent verifier runs holdouts only after source and development results are frozen. Existing older campaign holdouts remain untouched.

Combined evaluation: fresh allocation and CPU profiles of the existing real Mongoose workload using a disposable database copy; paired baseline/combined runtime results and focused benchmark regression checks. Application evidence limits will be reported explicitly. Independent review checks source ownership, reset/retention, frozen evaluator integrity, metric calculations, and failure paths before publication.
