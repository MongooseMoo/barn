# perf-sourcekey-memo — in progress

Branch: perf-sourcekey-memo (worktree agent-a06d9fcc76d54aa24)

## Design chosen (Option A)
New leaf package `barn/sourcekey`: `type Key struct{hash [32]byte; set bool}`,
`Of(lines []string) Key`, `Key.IsSet()`. Zero Key = "not computed" sentinel,
never produced by `Of`.

- compiler/cache.go: program cache map keyed by `sourcekey.Key`; local
  `sourceKey()` deleted (moved to the shared package so store and compiler can
  never disagree on the key for the same source).
- compiler/compiler.go: `CompileMOOWithKey(lines, key, registry)`; `CompileMOO`
  = `CompileMOOWithKey(lines, sourcekey.Of(lines), registry)`. Unset key falls
  back to hashing.
- db/store Verb gains unexported `codeKey`; VerbView gains `CodeKey`.
  All writes funnel through `Verb.setCodeOwned` / `Verb.setCodeCopy`.

Rejected: Option B (backing-array identity) — a recycled/reused backing array
would serve a stale program. Correctness beats speed.

## Write sites being funneled through setCodeOwned/setCodeCopy
1. db/store/object.go NewVerb (key computed in constructor)
2. db/store/object.go Verb.SetCode (loader, db/format)
3. db/store/builder.go ObjectBuilder.SetVerbCodeByIndex (loader)
4. db/store/store_cow.go buildImageWithVerbCode
5. db/store/store_txn.go:~420 read-txn image rebuild verb-write overlay
6. db/store/store_txn.go:~2232 commit apply
7. db/store/store_txn.go stageVerbCode (txn SetVerbCode/SetVerbCodeByIndex)
8. db/store/store_verbs.go Store.SetVerbCode
9. db/store/store_verbs.go Store.SetVerbCodeByIndex

## State
All 9 write sites funneled. compiler + sourcekey + db/store tests GREEN.
Call sites switched to CompileMOOWithKey: vm/op_verb.go (call, pass),
scheduler/call_verb.go x2, scheduler/task_factory.go x2,
scheduler/task_runtime.go, scheduler/waif_lifecycle.go, builtins/verbs.go
(disassemble), cmd/dump_verb. `go build ./...` clean.

## Gates
- gofmt -l on all touched files: no output
- go build ./...: exit 0
- go test ./compiler/ ./sourcekey/ ./vm/ ./db/store/ ./scheduler/ -count=1: all ok
- go test ./db/store/ ./scheduler/ -count=1 -race: ok (store 2.756s, scheduler 231s)
- go test ./... -count=1: all ok
- go vet: only pre-existing vm/stack.go ReadByte signature complaint (untouched file)

## Still hashing per call (left alone, no stored key available)
scheduler/eval.go, scheduler/task_load.go (saved task source), vm/registry.go,
server/input_processor.go, builtins/verbs.go set_verb_code compile check,
cmd/barn/main.go -eval.
