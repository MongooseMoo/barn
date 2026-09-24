# Independent whole-branch merge gate

Verdict: MERGE for whole-branch source review at `0d5254ad74f059932c9bac7321cca159effd3732`, conditional on green remote CI for the exact PR head before the actual merge. The branch may now be published for that final CI gate. No unresolved source defect was identified by this verifier. Remote CI has not yet been established; the parent owns that final check and normal merge.

Reviewed the full change inventory against `83c3e8d7ef3546ce2ae82d550684790f77706975`, initially at `ae8fafb` with the pending WebSocket repair and then at `0d5254ad74f059932c9bac7321cca159effd3732`. Independent source review concentrated on persistent maps, compact bytecode locals, transaction and WAIF isolation, history retirement, checkpoint ownership, VM root collection, and the two defects found in the separate scheduling audit. Parent and analyst cover the remaining complete branch scope; this record does not imply every line received three independent reviews.

## Source challenges

- Persistent map updates copy published trie paths and collision buckets. Constructor mutation is confined to unpublished nodes. Deletion rebuilds only its order prefix without tombstones. Existing randomized alias tests, full-hash collision tests, bulk constructor alias tests, and concurrent alias tests exercise these invariants.
- Local compaction rewrites every local-bearing operand form in the current opcode table, including optional one-based fork/catch bindings and wide forms. Constants and branch widths remain unchanged. `OP_SCATTER` carries shape counts, not local indexes. Compilation alone invokes compaction; persisted bytecode is not rewritten.
- WAIF property images are detached for transaction writes, validate against one store domain and publication clock, and share the coarse publication lock with object writes. Renewal preserves WAIF dependencies and rebases own publications. Snapshot cloning retains WAIF identity while freezing payloads and external task aliases.
- History pruning advertises pending work before sampling readers. Every publication invalidates the prune watermark; equality of a nonzero floor is the only fast-path condition. The full scan stamps the watermark after completion. No floor lock is held while acquiring the publication lock. Historical images contribute semantic GC roots, while checkpoint ownership follows only committed edges.
- VM pending-root identity sets are reset when roots are consumed. Persistent map finalization cache publication uses `sync.Once`. Existing tests cover old snapshot children, discarded retries, weak history bookkeeping, and checkpoint pending roots.
- The standalone hook repair initializes foreground tick and seconds limits plus its execution deadline; nested hooks retain parent budgets. The WebSocket repair rejects invalid UTF-8 both before notification queueing and at the writer boundary while leaving TCP arbitrary-byte output intact.
- Commit-gate grants are owner-held capabilities, cancellation removes waiting requests or releases an acquired grant, and the store verifies exclusive ownership before bypassing the shared gate. Admission and checkpoint barriers are separate abstractions rather than inline scheduler exceptions.

## Independent verification

Executed in the correctness worktree after worker source edits were complete:

```text
go test -race ./types ./bytecode ./compiler ./db/store -count=1 -timeout=3m
ok  github.com/MongooseMoo/barn/types     1.714s
ok  github.com/MongooseMoo/barn/bytecode  1.630s
ok  github.com/MongooseMoo/barn/compiler 1.378s
ok  github.com/MongooseMoo/barn/db/store 3.933s
exit_code: 0
```

The separate analyst record resolves both its P1 standalone-hook budget finding and P2 invalid WebSocket text finding with independent race regressions. No manual conformance server, database copy, credential, raw live artifact, push, or merge was produced by this verifier.

## Final source gate

Independently inspected the terminal artifacts for the clean Linux candidate at the exact source commit above. These are local reproductions of CI commands and the documented managed conformance flow, not a claim about GitHub check status.

```text
git -C /tmp/barn-review-20260921.FC2Z3J/whole-audit rev-parse HEAD
0d5254ad74f059932c9bac7321cca159effd3732
whole-audit-ci.log:
OK
PYTHON_TEST_PASS
CI_COMMANDS_PASS
whole-audit-candidate.log:
11 passed, 1 warning in 7.62s
whole-audit-candidate.exit: 0
whole-audit-full.log:
12524 passed, 420 skipped, 1 warning in 329.20s (0:05:29)
whole-audit-full.exit: 0
```

The full managed suite reached its terminal summary. Both analyst findings are repaired, independent targeted races are green, and final source validation is green. Scope includes the complete branch against master, with reviewer coverage divided as documented above. The accepted earlier performance experiment remains evidence for its measured source and workload; this audit does not invent new whole-branch speed claims. The only remaining merge condition is exact PR-head remote CI and any new review findings that arise during babysitting.
