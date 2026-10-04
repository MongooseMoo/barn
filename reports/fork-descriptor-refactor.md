# Typed fork descriptor (#350)

Base: `56302cdc4914af319aa54a6ecc6868b3db4ceb90`.

The active slice is the fork handoff payload and its producers, consumers,
metadata, and validation. Other control-flow and task capabilities remain with
their existing owners. This is a bounded representation refactor.

Target architecture:

- `bytecode.ForkBody` owns the parent program and named integer offset/length.
- `types.ForkBody` exposes only variable-name and first-line metadata, preserving
  the existing `bytecode` to `types` dependency direction.
- The engine performs the single concrete descriptor assertion for execution.
- Bytecode owns fork-range validation and extraction; snapshots use metadata.
- Source-restored queued forks retain their program/source/variables with no
  live body descriptor, as required by the existing persistence boundary.

Forbidden surfaces:

- Positional `[3]interface{}` or `[3]any` fork bodies.
- An unconstrained `ForkInfo.Body`, duplicated tuple decoding, or a tuple shim.
- Importing `bytecode` into `types` or changing the serialized task format.

Disposition of active surfaces:

| Surface | Disposition | Owner and purpose |
| --- | --- | --- |
| `types/result.go` opaque fork payload | Delete | Recreate the typed handoff contract in `types/fork.go`; ordinary result/control-flow declarations stay in `types` |
| `bytecode.Program.ExtractForkBody` | Consolidate | Keep extraction/rebasing in bytecode; centralize nil/range/instruction-boundary validation |
| `bytecode.ForkBody` | Create | Runtime descriptor for an existing bytecode range |
| `vm.executeFork` payload construction | Rewrite | VM emits the owner descriptor and retains existing source/variable capture |
| `engine.forkBodyProgram` decoding | Rewrite | One execution boundary assertion followed by owner extraction |
| `task.PersistenceSnapshot` tuple decoding | Delete | Snapshot calls typed metadata methods and copies names |
| `engine.loadQueuedTask` source restoration | Keep | Persistence rebuilds source-only tasks without a live body |
| Live fork, retry, wide-body, snapshot tests | Rewrite/extend | Exercise owner descriptors and existing runtime paths |

Search gates:

```sh
rg -n '\[3\](interface\{\}|any)|Body\s+interface\{\}' . --glob '*.go'
```

The gate must report zero matches; no compatibility reader or alternate tuple
representation may remain. Inspect remaining concrete body assertions to ensure
production execution has one boundary and task snapshots have none.

Runtime gates: invalid descriptor rejection; live/wide fork execution; first-run
conflict retry; snapshot metadata copying; source-only queued task restoration;
focused race, vet, staticcheck, and exact-head full CI.

## Iteration 1

The invalid-parent regression failed against the original tuple representation:

```text
--- FAIL: TestCreateForkedTaskRejectsNilParentProgram
panic: runtime error: invalid memory address or nil pointer dereference
bytecode.(*Program).ExtractForkBody
NIL_PARENT_BASELINE_EXIT=1
```

Deleting the opaque handoff declaration exposed the required migration through
the compiler gate before the typed replacement was introduced:

```text
types/result.go:27:13: undefined: ForkInfo
types/result.go:64:17: undefined: ForkInfo
DELETION_GATE_EXIT=1
```

## Iteration 2

The owner descriptor replaces every producer and consumer. A repository-wide
search also found three queued-task writer fixtures, which now use descriptors;
one length-one fixture needed a real return instruction to satisfy the existing
instruction-boundary contract. No tuple reader or compatibility shim remains.

```text
FORBIDDEN_FORK_TUPLES=0
TYPES_UPWARD_IMPORTS=0
```

The only production concrete body assertion is in `engine.forkBodyProgram`.
Snapshots obtain metadata through the lower-layer interface. Descriptor tests
cover nil parents, typed nil, invalid and overflowing ranges, operand boundaries,
wide offsets and lengths, and extraction independence. Runtime tests retain
fork retries and prove source-only queued-task execution and metadata copying.
An initial empty-range test incorrectly assumed that EOF was an accepted start
boundary; the fixture was corrected to an existing accepted boundary without
changing validation behavior.

```sh
go test ./bytecode ./task ./vm ./engine ./db/format -run 'Fork|QueuedTask' -count=1 -timeout=180s
go test -race ./bytecode ./task ./vm ./engine ./db/format -run 'Fork|QueuedTask' -count=1 -timeout=180s
go vet ./...
staticcheck ./...
```

```text
FORK_FOCUSED_EXIT=0
FORK_RACE_AND_STATIC_OK
```

Changed Go files were formatted and `git diff --check` succeeded. Full repository
CI remains the publication and merge gate. This slice has reached its fixed
point: execution, metadata, validation, and persistence have distinct owners,
and all old tuple surfaces are gone. The kept implementation commit is recorded
below after committing this slice. Subsequent work will select another open
issue rather than extend fork scope.

Kept implementation commit: `17946c59add386c6db5f4668529e21ba9f4bacce`
(`refactor(fork): use a typed bytecode body descriptor`). This documentation-only
follow-up records that commit without changing the verified implementation.
