# Unused task yield hook deletion

## Goal

Delete the unused TaskYielder host hook and YieldReadyTasks forwarding methods
without replacing them or changing actual ProcessReadyTasks scheduling.

## State

Deletion is complete in seven scoped Go files: builtins/tasks.go,
builtins/host.go, server/server.go, engine/runtime.go,
server/input_processor.go, engine/yin_commit_test.go and
engine/worker_pool_test.go. The byte-accounting PR and its six Go files remain
outside this bounded change. No replacement was added.

Source diff: one addition and twenty deletions. Removed the interface,
Host field/validation entry, server wiring, both forwarding methods and three
obsolete test assignments. The Yin helper now returns the existing constructor
directly after its only extra host wiring was removed. Real scheduling remains at engine/runtime.go:585
with its production Eval caller at engine/eval.go:152.

Raw check output:

```text
gofmt_exit=0
diff_check_exit=0
removed_symbol_search_exit=1
engine/eval.go:152: if s.ProcessReadyTasks() != 0 {
engine/runtime.go:585: func (s *Runtime) ProcessReadyTasks() int {
```

The symbol search was `rg -n 'TaskYielder|YieldReadyTasks' . --glob '*.go'`;
exit one means no remaining matching Go references. Formatting ran only against
the seven verified literal Go source paths. No local tests or builds ran.

## Decisions and why

The task has one source owner per responsibility. A declared and wired hook
without a production consumer adds an apparent scheduling surface without
behavior. Delete it directly; preserve the real ProcessReadyTasks/Eval path.

## Next step

Parent reviews this local source diff, waits for the preceding independent PR
to merge, then publishes this deletion against the default branch with PR CI.
No commit, push, PR, review or merge was performed by this worker.
