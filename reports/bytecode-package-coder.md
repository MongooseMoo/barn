# CODER report — item 2: barn/bytecode extraction + compiled-verb cache

Branch `feat/bytecode-package` off the proven spike topology commit `1be12b9`.
Worktree `C:/Users/Q/code/barn-bytecode`. Built on the spike's extraction — did not redo it.

---

## Task 1 — replace the alias shim with direct `bytecode.X` references
- Deleted `vm/bytecode_aliases.go` (the spike's diff-minimizing re-export layer). No aliases
  remain anywhere; nothing was kept as "load-bearing" — the whole shim is gone.
- Replaced every bare reference in the VM execution engine and its callers with `bytecode.X`:
  - `vm/`: control.go, op_iter.go, op_verb.go, op_property.go, registry.go, stack.go,
    traceback.go, vm.go, error.go (+ vm test files bytecode_execution_test.go,
    dump_persistence_test.go).
  - cross-package callers that used the `vm.*` aliases: `cmd/barn/main.go`,
    `cmd/dump_verb/main.go` (dropped its now-unused `barn/vm` import),
    `conformance/runner.go`, `server/scheduler_eval.go`,
    `server/scheduler_task_factory.go`, `server/scheduler_task_load.go`,
    `server/scheduler_task_runtime.go`.
- Method: `gofmt -r 'X -> bytecode.X'` (AST-aware, selector-safe — does NOT touch
  `frame.Program` field selectors). gofmt -r rewrites expression positions only, so
  type-position references (struct field types, params, returns, composite-literal element
  types) and struct-literal field KEYS (`Program: prog,`) were fixed by hand. Added
  `"barn/bytecode"` imports where newly required.

## Task 2 — content-addressed compiled-verb cache (in `barn/bytecode`)
- New `bytecode/cache.go`: a bounded LRU `map[uint64]*Program` (`container/list` for O(1)
  eviction) guarded by its own `sync.Mutex`, capacity 8192.
- Key = FNV-1a (`hash/fnv`) over the RAW stored source: the `[]string` Code joined with `\n`
  written BETWEEN lines (so `["ab","c"]` != `["a","bc"]`).
- `CompileVerbBytecode(code, registry)` now: hash code -> on hit return cached `*Program`
  (cheap map lookup, no parse/compile); on miss parse + compile, store, return.
  Correctness is automatic — changed source -> new hash -> recompile. The cached `*Program`
  is immutable (all per-execution state lives on the VM `StackFrame`), so sharing one
  `*Program` across executions is safe and matches master's compile-once-per-verb behavior.
- Eviction is memory-only (LRU cap); an evicted entry just recompiles, never serves stale
  code.
- `verb_cache_stats` untouched: every `Store.NoteVerbCacheClear` / `NoteVerbCacheMiss` /
  `ConsumeVerbCacheStats` call stays exactly where it was (they live in vm/op_verb.go,
  builtins/objects.go, builtins/system.go — outside bytecode — and are decoupled bookkeeping).

## Task 3 — gates (quoted output)

**go build ./...** — exit 0.

**go vet ./...** — only the 2 known pre-existing findings:
```
cmd\moo_client\main.go:53:25: address format "%s:%d" does not work with IPv6 (passed to net.Dial at L56)
vm\stack.go:49:15: method ReadByte() byte should have signature ReadByte() (byte, error)
```

**go test ./...** — touched packages pass:
```
ok  barn/bytecode   ok  barn/vm   ok  barn/builtins   ok  barn/server   ok  barn/db/store
```
Only the 2 known pre-existing fixture failures (identical on the spike base):
- `barn/conformance` — missing external `../cow_py/tests/conformance` dir
- `barn/db/format` `TestLoadMongooseSnapshot` — missing `mongoose7_snapshot.db` file

**db/store parser-free:**
```
$ go list -deps ./db/store | grep parser
(no output)   grep exit 1  => barn/parser is NOT a transitive dep of db/store
```

**Conformance (managed harness, from this worktree):**
```
================ 3871 passed, 131 skipped in 142.95s ================
```
EXACTLY 3871 passed / 0 failed / 131 skipped — matches the required gate.

**Cache benchmark (proves a hit is not a recompile):**
```
BenchmarkCompileVerbCold-32     752072     1677 ns/op    3750 B/op   27 allocs/op
BenchmarkCompileVerbWarm-32   44850423      27.05 ns/op     0 B/op    0 allocs/op
```
Warm path (second+ compile of the same source) is ~62x faster and ZERO allocations — a
cheap map hit, not a parse+compile. Correctness tests: `TestCompileVerbBytecodeCachesByContent`
(same `*Program` for same source, different for changed source) and `TestProgramCacheEviction`
(LRU cap bounds memory) both pass.

---

## Deviations from the spike inventory
- None structural. The spike's `CompileVerbBytecode` always recompiled (topology-only); this
  branch added the real cache as planned. The spike kept an alias shim "for diff size"; this
  branch removes it for the principled final state. No symbols moved beyond what 1be12b9 moved.

## Commit
`feat/bytecode-package` — see commit hash recorded by the final commit (below in git log).
NOT merged: a verifier gates the merge next.
