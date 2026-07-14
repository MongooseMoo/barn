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
| `verb` | semantic verb program representation | parser tokens or VM state |
| `compiler` | the complete source-to-bytecode pipeline and cache | alternate runtime compilation paths |
| `bytecode` | immutable executable programs and opcodes | task scheduling or database mutation |
| `vm` | frames, stacks, instruction execution, explicit VM outcomes | direct committed-world publication |
| `types` | tagged MOO runtime values and value operations | object persistence |
| `object` | object, property, verb, and inheritance behavior | socket and task lifecycle |
| `db` | database format and persistent world storage | language parsing |
| `task` | explicit durable MOO task state | one-goroutine-per-task execution |
| `scheduler` | ready/delayed task order, suspension, and resumption | transport reads and writes |
| `builtins` | builtin catalog, validation, and host-effect boundaries | reflection-based hidden registration |
| `server` | listeners, connections, login, command ingress, and output | language semantics |
| `cmd/barn` | flags, composition, startup, checkpoint, and shutdown | domain behavior |

## Task and concurrency boundary

A MOO task is explicit data in `task/task.go`, including its VM state, limits,
identity, permissions, and task-local state. `scheduler/scheduler.go` selects
and executes one runnable MOO task segment at a time. `server/input_processor.go`
serializes connection input into that scheduler.

Forking allocates the child ID and binds the optional fork variable before the
parent continues. The child is queued, including for delay zero. Suspension
preserves explicit task and VM state; it is not a blocked goroutine or a channel
receive. A previously foreground task resumes with Toast's background limits.

Goroutines are appropriate for socket transport and opaque host work that is a
real MOO suspension point. Host workers may not access or mutate the database;
their results return through the scheduler-owned resumption path.

## Compilation and VM boundary

`compiler` is the single owner of complete MOO source compilation: parse through
`parser`, produce semantic `verb` nodes, lower through `bytecode`, preserve
diagnostics and source, and cache immutable programs. Runtime callers do not
reconstruct a subset of that pipeline.

`vm` executes immutable bytecode against explicit frames and returns outcomes
to the task/runtime owner. VM code does not publish database mutations through
an alternate path. Suspended tasks retain bytecode position, stacks, locals,
handlers, limits, and task-local state as data suitable for persistence.

## Database boundary

`db/format` owns the textual v4/v17 codec. `db/store` owns the in-memory world
and its persistence boundary. Toast source and live oracle behavior are the
format authority. `lambdamoo-db-py` is useful for structural inspection and
differential round trips, but does not decide behavior when it differs from
freshly verified Toast.

## Verification

The shared gate is `../moo-conformance-tests`. Focused Go tests may prove local
mechanics, but they do not replace a named conformance row or a fresh Toast
oracle observation. Package paths in this document should be updated when code
moves; normative language documents should change only when the authority
evidence changes.
