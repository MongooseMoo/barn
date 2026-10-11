# PR #96 repair review checkpoint

- Scope: read-only review of local repair commit `ff95ff75b7da18ee5cb077a4c26c42f73e42f2b8` over PR head `0450cf22c84d0c3ebdb96a9172b6072db1a7f56a` in `C:\Users\Q\code\.worktrees\barn-pr96-pending-roots-repair`.
- Repair diff is limited to `vm/anonymous_gc.go` and `vm/anonymous_gc_test.go`; `git diff --check` is clean.
- Repair retains every anonymous identity found in frame locals and live operand stack, sorted by identity. This fixes the staged edge-removal loss caused by minimizing roots against the pre-commit Store graph.
- Actual shutdown timing supports that failure mode: `shutdown()` collects while the task transaction may still contain staged writes; task cancellation/completion collectors run around transaction completion.
- Current collector does not inspect other VM value-bearing fields: frame `ThisValue`, `Args`, `SavedThisValue`, `PendingError` payload, VM task-context `ThisValue`, or pending waifs. The claim that every direct VM root is retained is therefore not yet established and appears false for anonymous receivers/arguments.
- Toast source confirms pending-finalization entries are not passive serialization roots: on startup they are queued, then `recycle_anonymous_objects()` marks each pending anonymous object recycled, runs `recycle`, destroys it, and assumes queue uniqueness. This makes retaining currently persistent roots and multiple members of one cycle potentially unsafe, not merely redundant.
- Barn snapshot traversal deduplicates serialized anonymous objects by identity, but the pending-finalization list itself preserves distinct roots. Existing round-trip test covers one root for a two-object cycle, not the repair's new two-root cycle case.
- Blocker under investigation: determine whether Toast can ever emit persistent VM roots or multiple members of one cycle into the pending queue, and whether the repair would cause duplicate/invalid finalization on reload. Then run focused/race/full Go gates.
- No repair files were edited, staged, committed, or pushed.

## Re-review at `25d689eeb6817970f0332c3e1ee5db61f03af4c5`

- New commit moves persistent filtering and canonical root selection into `Store.Snapshot()` under exclusive `s.mu.Lock`, while retaining the raw live candidate queue unchanged.
- Staged persistent-edge addition and removal are now both covered. Persistent roots are excluded from pending output but remain serialization roots; unreachable split graphs retain all raw candidates for later snapshots. Cycles normalize to one deterministic root and serialize each anonymous object once.
- Lock audit so far: coarse writers take `s.mu.Lock`; decentralized commits take `s.mu.RLock` through validation/publication; anonymous writes route coarse; pending queue writers take `s.mu.Lock`. Snapshot takes only `s.mu` and never `commitGate`, slot, history, recycled, or task locks, so no inversion found. Transaction ID reservation is atomic outside the graph lock but publishes no object and cannot overlap the anonymous serialization-id range because high-water allocation stays above all live anonymous identities.
- VM collector now covers locals, active stack slots, frame receiver/args/saved receiver, wrapped `VMException.Value`, `PendingWaifs`, yield value/fork receiver+variables, and several context values. It still misses real task-local state: `set_task_local()` writes `(*task.Task).TaskLocal`, while `TaskContext.TaskLocal` is unused outside its declaration/tests. The new test fabricates `Context.TaskLocal`, so it does not cover the actual root.
- Additional loaded-pending concern: snapshot normalization extracts anonymous refs from every queued value and emits only bare anonymous roots. A loaded pending WAIF (supported by the format and Toast startup) would be dropped. Need decide whether this is in exact PR acceptance scope and report it precisely.
- Potential additional live root: normal return values are popped below `SP` and stored in `task.Result`, not `vm.yieldResult`; shutdown completion collection scans only the VM. Evaluate caller timing before final verdict.
- Gates at exact head: focused Go tests PASS; full `go test ./... -count=1` PASS; `go build ./...` PASS. Focused race is still running.
- Next: finish race, run focused managed startup cases through the documented harness, verify final status/diff, and return MERGE/NO-MERGE.
