# Language and value ownership survey

## State

P7 read-only survey is complete; the coordinator persisted its findings.
Applicable parser/AGENTS.md and the execution plan have been read.

## Keep compiler and source identity owners

Compiler owns an immutable builtin-ID snapshot and bounded sharded content
cache (compiler/compiler.go:42-72,106-162; cache.go:20-35,113-168).
sourcekey remains the shared leaf owner with line-length-prefixed SHA256.
Parsing/lowering occurs outside the cache shard mutex. In-flight compilation
releases all waiters on success, error or panic, and failed compilations do not
persist. No runtime/server boundary replacement is justified by these owners.

## Consolidate activation list formatting

Existing ActivationFrame.ToList constructs six fields. Seven call sites then
copy/append to omit line numbers or include runtime variables: callers and
task_stack in builtins/tasks.go, vm/traceback.go, engine/lag_callback.go, and
two task_runtime callback paths. Move those two explicit flags into ToList on
the existing owner, allocate the exact final 5/6/7-field slice once and call
RuntimeVariableMap only when requested. Keep filtering, order, eval exclusions,
anonymous invalidation and authority redaction at their current callers.
Existing task_builtins, lazy_runtime_variables, anonymous traceback, authority,
eval callers and traceback integration regressions exercise these surfaces.

## Delete literal forwarding wrapper

compiler/literal_values.go errorNameToCode forwards types.ErrorFromString with
one caller. Call the existing types owner directly and delete the wrapper.

## Keep mutable execution and immutable value boundaries

Parser produces IR; runtime literal conversion remains compiler-owned.
Source identity, compiled cache identity and transaction lookup identity differ.
Retry clones mutable stack/local/argument/handler/native-continuation storage
while sharing immutable programs/values. Persistence separately validates IP and
stack heights and reacquires recycle guards. Frame installation/cache remains
pushFrame/popFrame; pooled storage is cleared, yielded VMs remain unpooled and
dead operands are scrubbed. Lists expose borrowed read-only elements and claim
an atomic append frontier; WAIF publication clones immutable property images
and retains history for old readers. No new correctness defect or measured
performance conclusion was established. Scout completed and stopped.
