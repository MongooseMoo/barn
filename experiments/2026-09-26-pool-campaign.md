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

## Application measurement plan (before combined source)

Use unchanged `engine/TestMongooseRealWorkload`, disposable snapshot SHA256 `489FF8D14884392DCFBA6CD88D407E53A2140C03D4F0CC4B1B19CF2AFD8A031E`, default command mix and repairs, GOMAXPROCS=4. Diagnostic baseline profile: players=1, warmup=1s, measure=5s, memory profile only; a separate CPU profile avoids combining instrumentation. Confirmation: five alternating baseline/candidate pairs, players=1,4,16, warmup=1s and measure=3s, fresh process each side. Report every failure/uncaught count and raw output. Compare per-player median goodput, bytes/command and latency; treat statistically clear >5 percent goodput loss or new correctness failures as a stop for diagnosis. Short-window p99 is diagnostic, not a production latency claim. Baseline/candidate fixtures must be identical. If the existing workload cannot run faithfully, record the exact limitation and use microbenchmark evidence only without claiming application speedup.

File-read profiling also identified returned-string builder growth as a larger allocator than the input buffer. Permit one separate preallocation follow-on experiment after the buffer experiment, with its own preregistration and gate; no further scope expansion without measured evidence.

## Initial combined source

At `5fb70c77af24a3ee16d7bfcde9d9cdcea8467f17`, independently verified production source exactly matches unique `7e0363d`, file-read `0e5fdba`, and finalization `6b72d0a`. Reviewer ownership/race tests and all ten-pair holdouts passed; see the independent review record. Commit scratch pooling was rejected from its measured allocation ceiling before implementation. Both combined text/binary preallocation and its separately preregistered binary-only refinement failed runtime guards and are excluded.

Application evaluator seals: `engine/mongoose_real_bench_test.go` blob `3ac942cb3d5589b7a588dfb354ec0d0ac0627045`; `experiments/pool-application-measure.ps1` blob `dc78471a52cb7c385ffdd5c7c762b6ecaf7a76b2`. The baseline binary was compiled before any production source integration. Application fixture processes get fresh private directories and identical input databases.

## Final narrowed source

The initial application runs raised a throughput concern. Broader finalization
benchmarks then exposed overhead in anonymous-only Value collection, which
already allocated nothing. One preregistered refinement keeps that original
path for anonymous/list/map Values and pools only direct WAIF Values plus
Frames. Source `ea251f7` passes all fourteen focused workload guards; independent
review `074bb8d` verifies the calculations and ownership/race tests. See the
diagnostic, refinement, and refinement-review records for the rejected initial
design and the remaining oversized-frame allocation tradeoff.

The final source is frozen for a separate ten-pair, longer sixteen-player
confirmation. Its preregistration and binary/evaluator seals are in
`2026-09-26-pool-application-confirmation.md`. Original application observations
and profiles remain preserved and explicitly identify their older candidate.
