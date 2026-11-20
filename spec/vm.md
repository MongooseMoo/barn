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
The stock oracle is now ToastStunt 2.8.0_7, commit
`03af4fa2b53b418999f03566b646ee42bc7ea647`
(`profiles/toast/stock-wsl-testdb.json`); `file:line` citations below are to
upstream `origin/master`, which contains that commit, and every cited line is
the same at both. Barn's similarly named operations do not imply bytecode or struct compatibility
with Toast.

## 1. Current Barn compilation path

| Stage | Current owner | Result |
|---|---|---|
| Parse | `parser` | sealed semantic nodes in `ir` |
| Orchestrate | `compiler/compiler.go` | structured diagnostics or a cached `*bytecode.Program` |
| Lower | `compiler/lower*.go` | Barn bytecode, constants, locals, and line information |
| Verify | `bytecode/verify.go` | operand stack heights and control-flow checks |
| Disassemble | `bytecode/disassemble.go` | deterministic text for Barn bytecode inspection |
| Execute | `vm` | a `types.Result` returned to `engine` |

`(*compiler.Compiler).CompileMOO()` is the production source-to-bytecode entry
point. It parses through `parser`, lowers through `compiler/lower*.go`,
attaches a copy of the original source lines and each fork body's unparse, and
caches the resulting program by source content. Production callers in
`builtins`, `engine`, `server` and `vm` use that entry point rather than
composing the parser and lowering themselves.

`ir/ir.go` owns the sealed expression, assignment-target, statement, and
binding families consumed by the lowering. Parser tokens and concrete source
spelling do not become VM nodes. The stored verb `Code` in `db/store` is the
formatter's output when `set_verb_code()` installs it (floats spelled so they
read back exactly) and is what `db/format` writes; `verb_code()` reformats with
Toast's `%.15g` float spelling. The compiler also attaches the source to
`bytecode.Program.Source` for runtime diagnostics. The formatter
(`parser.FormatMOOWith`, `parser.FormatUncheckedWith`) also produces the
unknown-builtin `call_function` rewrite applied at database load and the saved
code of queued forks.

## 2. Current Barn program and opcode representation

`bytecode/program.go` defines the current executable artifact,
`bytecode.Program`. Its meaningful fields are `Code`, `Constants`, `VarNames`,
`LineInfo`, `NumLocals`, `Source`, `BuiltinSlots` (local slots of the built-in
variables, including `true` and `false`), `BuiltinLayout` (the builtin
registry layout its call IDs were compiled against), `Origin` (which compile of
`Source` produced it, including a fork number and a first-line shift for a
restored queued fork) and `ForkSources` (each fork body unparsed as Toast
unparses a fork vector). `CallSites` and `ForkBodies` are runtime caches, not
part of the program's meaning. The fields are mutable Go slices and values.
The compiler cache returns shared program pointers; callers treat them as
compiled executables, but the type does not enforce immutability.

`bytecode/opcodes.go` is the sole current Barn opcode list and name table.
Instructions use a one-byte opcode plus the operands read by the corresponding
compiler and VM cases; operands are not governed by one universal 1/2/4-byte
schema. Immediate integers from -10 through 143 occupy opcode values directly.
`OP_CONTINUE` and `OP_CATCH` remain named dead opcodes and are not emitted by
the current compiler. New opcodes take the numbers of dead ones or are
appended, so existing numbering does not move: `OP_PUSH_WIDE` has `OP_BREAK`'s
number and `OP_MAPHASKEY_KEYED` has `OP_RAISE`'s.

Toast's literal table has no fixed size: `code_gen.cc` widens every literal
reference to 1, 2 or 4 bytes as the table grows past 256 and 65,536 entries
(`ref_size()`, `src/code_gen.cc:1202-1210`). A verb or `eval()` program with
more than 256 distinct literals compiles and returns its values
(`language/literal_count_boundary.yaml`). Barn's `OP_PUSH` takes a one-byte
constant index; `OP_PUSH_WIDE` takes a two-byte index and is emitted only for
indexes from 256, so programs with at most 256 constants compile to the same
bytes. Barn refuses a program with more than 65,536 constants, where Toast
would use 4-byte references. Builtin IDs above 255 are called through
`OP_CALL_BUILTIN_WIDE` the same way.

Do not duplicate the opcode enum in this document. The earlier hand-maintained
tables were stale: current loops use `OP_LOOP`, fused range/list operations, and
iteration preparation; collections use the exact operations in
`bytecode/opcodes.go`; and one `OP_FORK` carries both the optional variable
index and fork-body length.

Ticks follow Toast's `COUNT_TICK` and `COUNT_EOP_TICK` classifications in
`src/include/opcode.h`: every executed opcode at or below `OP_G_PUT` costs one
tick, tested against the budget before the opcode runs (so the opcode that
exhausts the budget never executes), and every extended opcode at or above
`EOP_CATCH` costs one tick tested the same way (ToastStunt 2.8, fdfa9ae;
earlier releases subtracted it without the test).
`bytecode.InstructionTicks()` charges each Barn opcode what the Toast opcodes it
implements would charge (for example a call with arguments also pays for
Toast's `OP_MAKE_SINGLETON_LIST`). Compiler-synthesized helpers with no Toast
counterpart are tick-free (`OP_SET_LOCAL`, `OP_PUSH_INT`) or
carry an `OP_TICKS` prefix stating the charge. The conformance suite's
`vm/tick_accounting.yaml` pins the per-construct values.

`bytecode.VerifyProgram()` and `bytecode.StackHeights()` check a program at a
trust boundary; the VM's execution loop does not. They compute the operand
stack height on entry to every reachable instruction, following fallthrough,
jumps, loops, the range and list loop checks, `&&`/`||`, exception and finally
handlers, and fork bodies. They reject a pop below zero, an instruction reached
at two heights, control running past the end of the code, an opcode with no
known stack effect, a finally or except handler at or before its own `TRY`, a
negative fork-body length (an empty `fork ... endfork` body has length zero
and is valid), and a reachable control-flow cycle in which no instruction
charges a tick (a cycle with only unchecked ticks is accepted, because the
seconds checkpoint bounds it). Saved tasks no longer carry Barn bytecode (see
section 4); the checks run when a restored frame's position is validated
against its recompiled program.

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
`engine/task_runtime.go` creates the child, binds its ID, and resumes the
parent before the engine finishes that parent run. Two further flows let a
builtin push a frame on the calling VM: `FlowEvalPush` for `eval()` and
`FlowBuiltinPush` for a resumable verb frame such as a `move()` hook.

The VM reads and stages writes through `db/store`, an MVCC store:
`vm/op_property.go` and `vm/op_verb.go` read at the task's snapshot timestamp
and stage writes, and store-backed builtins do the same through
`kernel.TaskContext`. A commit validates the task's read set; a stale read
aborts the attempt and `engine` retries the task. `vm/anonymous_gc.go`
coordinates anonymous-object collection.

## 4. Suspension and database persistence

A live suspended Barn task retains its `*vm.VM` through `task.Task.BytecodeVM`
and preserves the operand stack, frames, instruction positions, locals,
handlers, and task-local value in memory. `VM.Resume()` continues that retained
instance after the scheduler supplies any resume value.

This state is durable. `VM.PersistenceVMSnapshot()` (`vm/persistence.go`)
captures each frame's program identity, instruction position, operand stack,
locals, handlers and pending finally reasons, and
`engine.Runtime.TaskSnapshots()` exports the snapshot to Toast's activation
form (`compiler/native_export.go`) so Toast can load and resume a Barn
checkpoint. The export maps a Barn frame paused at a call to Toast's pc, error
pc and operand stack through layout tables shared with the import
(`compiler/native_layout*.go`); it covers verb and builtin calls in verb and
fork bodies, inside `while`, `if`, `try`/`except` and `try`/`finally` bodies,
finally bodies themselves, catch expressions and their handlers, `for` loops
over lists, maps and ranges (including a pause in the range's upper bound),
spliced argument lists, and the `eval()` and `move()` builtin continuations,
which Toast keeps on the callee's activation. A `for` loop over a string is
written as a loop over the list of its characters, which binds the same
elements. A frame with no Toast form keeps the whole task in Barn's own frame
form, which carries source plus positions rather than bytecode; database.md
section 6 gives both layouts.

`vm.RestoreVMSnapshot()` rebuilds each frame's program by compiling its saved
source, checks its fingerprint (a mismatch drops the task at startup), then
validates every saved position and stack height against the rebuilt program
with `bytecode.StackHeights()`. Toast-form frames are converted first
(`compiler/native_import.go`).

Queued forks take a narrower path: their saved code (the unparse of the fork
body) and variables are read by `db/format/reader_task.go`, compiled through
the compiler in `engine/task_load.go`, shifted to the header's first line, and
recreated as queued fork tasks.

Toast's durable contract is defined by `src/include/execute.h`, `src/tasks.cc`,
`src/eval_vm.cc` and `src/execute.cc` (`write_activ()`/`read_activ()`), which
write and restore forked, suspended and interrupted task state. Barn must match
that observable behavior; Barn's own frame form is a fallback, not an
alternative specification.

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
