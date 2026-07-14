# Current Barn implementation map

## Status

This document is non-normative. It maps the current Go packages and important
ownership boundaries so readers can find the implementation. Observable MOO
behavior is defined by freshly verified Toast behavior, durable conformance
tests, and the normative language sections in this directory, in that order.
Historical sketches and illustrative Go snippets are not design authority.

## Package ownership

| Package | Owns | Does not own |
|---|---|---|
| `parser` | MOO source parsing and syntax diagnostics | bytecode execution |
| `verb` | semantic verb AST nodes and source positions | parser tokens or VM state |
| `compiler` | the complete parse/lower/source-attach pipeline, diagnostics, and program cache | bytecode instruction execution |
| `bytecode` | AST-to-bytecode lowering, opcode definitions, programs, and disassembly | task scheduling |
| `types` | tagged MOO runtime values and value operations | object persistence |
| `db/store` | synchronized mutable objects, properties, verbs, inheritance, topology, anonymous objects, and snapshots | text database syntax |
| `db/format` | v4/v17 text parsing and checkpoint output | live object mutation policy |
| `kernel` | the shared task execution context passed through VM and builtins | task queue ownership |
| `vm` | frames, stacks, opcode execution, direct property access, verb dispatch, anonymous-object GC, and explicit outcomes | socket lifecycle |
| `task` | explicit in-memory MOO task state and the task-manager index used by builtins | runnable-task selection |
| `scheduler` | task creation, the global ready-time heap, serialized execution, suspension/resumption, and deferred GC coordination | transport reads and writes |
| `builtins` | explicit builtin registry, argument validation, direct store operations, limits, and host-operation seams | parser syntax |
| `command` | player-input tokenization, intrinsic commands, object matching, and command-verb lookup | bytecode execution |
| `server` | listeners, connections, login, command ingress, and output | language semantics |
| `config` | runtime option types, parsing, defaults, and validation | CLI process lifecycle |
| `profile` | conformance profile manifests and profile-registry validation | runtime scheduling |
| `logging`, `metrics`, `trace` | structured-log setup, expvar counters, and optional execution tracing | MOO semantics |
| `cmd/barn` | main-server flags, composition, startup, checkpoint, and shutdown | reusable domain ownership |
| `cmd/*` | operational and diagnostic executables built on the owned packages | alternate domain implementations |

## Task and concurrency boundary

A MOO task is explicit in-memory data in `task/task.go`, including its VM
handle, limits, identity, permissions, stack metadata, wake state, and
task-local value. `scheduler/task_queue.go` orders Barn's one global ready-time
heap; `scheduler/scheduler.go` selects and executes at most one runnable MOO
task per call; and `server/input_processor.go` owns the serialized outer loop.

Forking allocates the child ID and binds the optional fork variable before the
parent continues. The child is queued, including for delay zero. Suspension
preserves explicit task and VM state in memory; it is not modeled as one blocked
goroutine per MOO task. `db/format/writer_task.go` currently emits zero
suspended and interrupted tasks, so this live state is not yet durable across a
checkpoint and restart. A previously foreground task resumes with the cached
background limits.

Barn currently uses goroutines for server input, connection handling, and
selected host builtins. Host completions update explicit task state and are
observed by the serialized scheduler loop. No package or type boundary enforces
a general rule that host goroutines cannot access `db/store`.

## Compilation and VM boundary

`compiler` is the single owner of complete MOO source compilation: parse through
`parser`, produce semantic `verb` nodes, lower through `bytecode`, preserve
diagnostics and source, and cache `*bytecode.Program` values. Production runtime
callers use `compiler.CompileMOO()` rather than reconstructing a subset of that
pipeline. `bytecode.Program` contains mutable Go slices and fields; callers
treat cached programs as shared executables, but the type does not enforce
immutability.

`vm` executes cached bytecode programs against explicit frames and returns
outcomes to the scheduler. It also reads and mutates `db/store` directly for
property opcodes, performs verb lookup, and coordinates anonymous-object
collection;
store-backed builtins mutate the same store through `kernel.TaskContext`.
Suspended tasks retain explicit bytecode position, stacks, locals, limits, and
task-local state in memory, but the current database codec does not serialize
and restore that full state.

## Database boundary

`db/format` owns the textual v4/v17 codec. `db/store` owns the in-memory world
and produces snapshots consumed by the writer; it does not own text database
syntax. Toast source and live oracle behavior are the format authority.
`lambdamoo-db-py` is useful for structural inspection and differential round
trips, but does not decide behavior when it differs from freshly verified
Toast.

## Verification

The shared gate is `../../moo-conformance-tests`. Focused Go tests may prove local
mechanics, but they do not replace a named conformance row or a fresh Toast
oracle observation. Package paths in this document should be updated when code
moves; normative language documents should change only when the authority
evidence changes.
