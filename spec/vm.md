# Barn VM implementation and Toast authority

## Status and authority

This document separates observable MOO behavior from Barn's current Go
implementation. Observable language, task, and error behavior is normative only
when established by the verified Toast source or the managed Toast oracle and
recorded in the durable conformance suite. Barn's packages, bytecode encoding,
and internal structs are non-normative implementation details.

The verified WSL source identity, executable, profile, wrapper, and exact
managed command are recorded in
`../../banteng/docs/reports/toast-oracle-identity-2026-07-14.md`. The managed
workflow is owned by
`plans/barn-toast-mongoose-convergence-workstreams.md`. In that verified Toast
checkout:

- `src/parser.y` parses MOO source;
- `src/code_gen.cc` compiles the parsed program;
- `src/include/program.h` and `src/program.cc` own `Program`, bytecode vectors,
  literal tables, fork vectors, variable names, and program lifetime;
- `src/include/opcode.h` owns ordinary and extended opcode encodings and their
  tick classification;
- `src/include/execute.h` owns `activation` and `vmstruct`;
- `src/execute.cc` owns dispatch, activation stacks, builtin continuations,
  limits, exceptions, finalizers, suspension capture, and resumption;
- `src/tasks.cc` owns task IDs, queues, fork and suspended-task records, task
  persistence, and task builtins; and
- `src/unparse.cc` renders parsed programs as source.

These paths are relative to `/root/src/toaststunt` at the recorded source SHA.
Barn's similarly named operations do not imply bytecode or struct compatibility
with Toast.

## 1. Current Barn compilation path

| Stage | Current owner | Result |
|---|---|---|
| Parse | `parser` | sealed semantic nodes in `verb` |
| Orchestrate | `compiler/compiler.go` | structured diagnostics or a cached `*bytecode.Program` |
| Lower | `bytecode/compiler.go` | Barn bytecode, constants, locals, and line information |
| Disassemble | `bytecode/disassemble.go` | deterministic text for Barn bytecode inspection |
| Execute | `vm` | a `types.Result` returned to `scheduler` |

`compiler.CompileMOO()` is the production source-to-bytecode entry point. It
parses through `parser`, lowers through `bytecode`, attaches a copy of the
original source lines, and caches the resulting program by source content.
Production callers in `builtins`, `scheduler`, `server`, `vm`, and `cmd` use
that entry point rather than composing the parser and bytecode compiler
themselves.

`verb/ir.go` owns the sealed expression, assignment-target, statement, and
binding families consumed by `bytecode/compiler.go`. Parser tokens and concrete
source spelling do not become VM nodes. The stored verb `Code` in `db/store` is
the source returned by `verb_code()` and written by `db/format`; the compiler
also attaches a copy to `bytecode.Program.Source` for runtime diagnostics and
fork handling. `parser.FormatMOO()` currently has no production caller.

## 2. Current Barn program and opcode representation

`bytecode/program.go` defines the current executable artifact:

```go
type Program struct {
    Code      []byte
    Constants []types.Value
    VarNames  []string
    LineInfo  []LineEntry
    NumLocals int
    Source    []string
}
```

The fields are mutable Go slices and values. The compiler cache returns shared
program pointers; callers treat them as compiled executables, but the type does
not enforce immutability.

`bytecode/opcodes.go` is the sole current Barn opcode list and name table.
Instructions use a one-byte opcode plus the operands read by the corresponding
compiler and VM cases; operands are not governed by one universal 1/2/4-byte
schema. Immediate integers from -10 through 143 occupy opcode values directly.
`OP_BREAK`, `OP_CONTINUE`, `OP_CATCH`, and `OP_RAISE` remain named dead opcodes
and are not emitted by the current compiler.

Do not duplicate the opcode enum in this document. The earlier hand-maintained
tables were stale: current loops use `OP_LOOP`, fused range/list operations, and
iteration preparation; collections use the exact operations in
`bytecode/opcodes.go`; and one `OP_FORK` carries both the optional variable
index and fork-body length.

`bytecode.CountsTick()` currently charges only `OP_CALL_BUILTIN`,
`OP_CALL_VERB`, `OP_LOOP`, `OP_FOR_RANGE_NEXT`, and `OP_PASS`. This is a known
implementation difference from Toast's broader `COUNT_TICK` and
`COUNT_EOP_TICK` classifications in `src/include/opcode.h`; Barn's current
classification is not normative MOO tick behavior.

## 3. Current Barn VM state and execution

`vm/vm.go` owns the current `VM` and `StackFrame` types. A VM contains a dynamic
shared operand stack, a stack of frame pointers, direct `db/store` and builtin
registry references, a `kernel.TaskContext`, tick and frame limits, pending
WAIFs, a cached top frame, and an explicit yield result. Each frame contains its
program and instruction pointer, base stack position, locals, receiver/player/
caller/verb data, loop and exception stacks, pending-finally error, debug mode,
and saved context for verb and eval frames.

`vm/stack.go` owns push, pop, operand reads, and return unwinding. The stack
grows with `append`; Barn does not use the fixed 1,024-value stack or frame pool
shown in the deleted historical snippets.

`VM.executeLoop()` fetches from the cached current frame and dispatches through
`VM.Execute()`'s switch. The compiler guarantees a terminal `OP_RETURN` or
`OP_RETURN_NONE` for every executable program, and fork extraction appends
`OP_RETURN_NONE`; the hot loop relies on that invariant. `VM.Step()` retains a
defensive falloff-to-zero path for single-step callers.

The VM returns explicit `types.Result` flows:

- `FlowReturn` for normal completion;
- `FlowException` for an uncaught error;
- `FlowSuspend` for a suspending builtin; and
- `FlowFork` for a fork statement.

`vm/error.go` and `VM.HandleError()` implement Barn's current typed errors,
exception values, handler search, finalizer continuation, and cross-frame
unwind. `vm/control.go` captures fork state and returns `FlowFork`;
`scheduler/task_runtime.go` creates the child, binds its ID, and resumes the
parent before the scheduler finishes that parent run.

The VM is not isolated from the world store. `vm/op_property.go` reads and
mutates `db/store` directly, `vm/op_verb.go` performs store-backed verb lookup,
and `vm/anonymous_gc.go` coordinates anonymous-object collection. Store-backed
builtins also mutate the same store through `kernel.TaskContext`. There is no
transaction or committed-publication boundary in current Barn.

## 4. Suspension and database persistence

A live suspended Barn task retains its `*vm.VM` through `task.Task.BytecodeVM`
and preserves the operand stack, frames, instruction positions, locals,
handlers, and task-local value in memory. `VM.Resume()` continues that retained
instance after the scheduler supplies any resume value.

This live state is not currently durable. `task/snapshot.go` copies task
metadata, call-stack descriptions, task-local data, and fork source/variables;
it does not snapshot the live VM operand stack, frames, instruction positions,
locals, or handler stacks. `db/format/writer_task.go` emits zero suspended and
interrupted tasks, and `db/format/reader_task.go` skips those records instead of
restoring them. No `VM.Serialize()` or `VM.Deserialize()` API exists.

Queued forks are a narrower path: their saved source and runtime-variable map
are read by `db/format/reader_task.go`, compiled once through
`compiler.CompileMOO()` in `scheduler/task_load.go`, and recreated as queued
fork tasks. That path does not prove suspended-VM checkpointing.

Toast's durable contract is separate. `src/include/execute.h` defines explicit
activation and VM structs, while `src/tasks.cc` and Toast's database IO path
write and restore forked, suspended, and interrupted task state. Barn must match
that observable behavior; Barn's incomplete codec is not an alternative
specification.

## 5. Verification boundary

The shared behavioral gate is `../../moo-conformance-tests`, run against the exact
managed WSL command recorded above. Focused Go tests prove current Barn
mechanics such as compiler terminators, fork-body rebasing, disassembly, stack
unwind, and opcode dispatch. They do not replace a Toast-proven conformance row
for language, tick, exception, task-order, or persistence behavior.

When current Barn code, this document, a focused test, or another reference
implementation disagrees with verified Toast, correct the durable behavioral
record first and then correct Barn. Barn's internal opcode encoding and Go
struct layout remain implementation details, not compatibility targets.
