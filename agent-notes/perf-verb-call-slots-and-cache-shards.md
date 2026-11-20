# perf/verb-call-slots-and-cache-shards

## Goal

Remove two hot spots the lead measured on the 16-player Mongoose workload:
name-based command-variable initialization in `vm.(*VM).frameFrom`
(`SetLocalByName` -> `strings.EqualFold`), and the single mutex in
`compiler.(*programCache)`. Approved design: A1 (restored programs get
BuiltinSlots), A2 (frameFrom writes command variables by slot), B (sharded
program cache).

## State

All three parts built and committed locally on this branch; not pushed, no PR.
The lead runs the real perf gate.

- A1: `db/format/reader_vm.go` fills `Program.BuiltinSlots` from `VarNames`
  after either frame branch (Toast-shaped and Barn metadata), using the same
  `strings.ToLower(strings.ToUpper(name))` key the compiler uses. Tests in
  `db/format/reader_vm_builtin_slots_test.go`.
- A2: `vm/vm.go` frameFrom writes the six command variables by
  `commandSlots(program)`; slot 0 writes `CommandEnvironment[index]`.
  `commandSlots` (vm/op_verb.go) also replaces the two literal slot arrays in
  `copyCommandLocals` and `commandEnvironment`. Test
  `vm/frame_command_env_test.go`.
- B: `compiler/cache.go` is 64 shards, each with its own mutex, map, LRU and
  flight table; `sourcekey.Key.Uint64` gives the shard hash input. Tests in
  `compiler/cache_shard_test.go`, existing tests in
  `compiler/cache_compile_test.go` adjusted to address the key's shard;
  `compiler/cache_parallel_bench_test.go` measures parallel hits.

## Decisions and why

- 64 shards: 8192 / 64 = 128 entries per shard. Measured with
  BenchmarkProgramCacheParallelGet (1024 cached keys): 16 shards gave about
  50 ns/op at -cpu 16, 64 shards about 25 ns/op.
- Shard hash = first 8 bytes of the key's SHA-256 (`Key.Uint64`); the hash is
  private to sourcekey, so the accessor lives there.
- A positive capacity below the shard count gives each shard capacity 1, so it
  stays bounded (otherwise integer division would make it 0 = unbounded).
- The LRU eviction test now uses three keys that share a shard, with two
  entries per shard, since eviction is per shard.

## Surprises

- Restored frames are rebuilt by `vm.RestoreVMSnapshot` without frameFrom, so
  BuiltinSlots (including TypeConstants) on restored programs cannot overwrite
  restored locals. Fork children of a restored frame go through
  `PrepareVerbFrame` -> frameFrom, then `engine/task_factory.go` copies every
  parent local by name over the type-constant initial values.
- Before A1, `commandEnvironment()` on a restored Toast-shaped frame returned
  the zero `CommandEnvironment` (never set on that path) instead of the
  restored argstr/dobj locals; with slots it reads the locals.
- Single-goroutine cache hits are slower with 64 shards: side-by-side against
  a scratch copy of the old single-mutex get, 42.3-43.4 ns/op old vs
  45.1-48.4 ns/op sharded (-cpu 1, 5 runs each). With 1 shard the new code
  matched the old (41.6-44.1), so the cost is the 64 separate maps/lists, not
  the indirection. Not resolved here.

## Next step

Lead: real-workload perf gate (1p and 16p) and decide on the single-thread
cache-hit cost above.
