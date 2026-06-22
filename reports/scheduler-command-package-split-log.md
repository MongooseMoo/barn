# Cleanup Refactor Fixed-Point Log - 2026-06-22

Target architecture:
- `barn/command` owns command-language parsing and command dispatch matching: `ParsedCommand`, `PrepSpec`, `ParseCommand`, `CommandWordList`, `MatchObject`, `VerbMatch`, and `FindVerb`.
- `barn/command` owns the command/input event shape consumed by server input processing.
- `barn/scheduler` owns task and VM runtime orchestration, not command parsing, command matching, connection I/O, websocket/listener transport, or server session input flow.
- `barn/server` owns connection, transport, listener, websocket, and input/session processing code.

Forbidden surfaces:
- Command parser types or functions owned by `server`.
- Verb dispatch matcher types or functions owned by `server`.
- Connection, transport, websocket, listener, or connection-manager references owned by `scheduler`.
- `server.Scheduler` as a renamed host for mixed connection/session/task behavior.
- Compatibility aliases or wrappers in `server` for moved command or matcher APIs.

Search gates:
- `rg -n "commandWordList|func ParseCommand|type ParsedCommand|type PrepSpec|type VerbMatch|func MatchObject|func FindVerb" server -g "*.go"`
- `rg -n "barn/scheduler" command -g "*.go"`
- `rg -n "barn/server" command scheduler -g "*.go"`
- `rg -n "package server|barn/server|connManager|Connection|ConnectionManager|Transport|WebSocket|listener|evalConnection" scheduler -g "*.go"`
- `rg -n "type Scheduler|func NewScheduler" server -g "*.go"`

Runtime gates:
- `go test ./command ./scheduler ./server`
- `git diff --check`

## Iteration 1 - `server command parsing and dispatch matching`

Slice read:
- `server/command.go`
- `server/matcher.go`
- `server/verbs.go`
- relevant callers in `server/scheduler.go`, `server/scheduler_login.go`, `server/scheduler_task_factory.go`, and `server/scheduler_task_runtime.go`

Surfaces:
- `ParsedCommand`, `PrepSpec`, `ParseCommand`, `CommandWordList`
  - Disposition: move
  - Owner after cleanup: `barn/command`
  - Action: moved parser and preposition vocabulary out of `server`.
  - Evidence: parser depends only on `types`, strings, and unicode.
- `PrepSpecForAlias`
  - Disposition: move
  - Owner after cleanup: `barn/command`
  - Action: exposed parser-owned preposition vocabulary lookup for dispatch matching.
  - Evidence: scheduler needs to compare verb prep specs without duplicating the parser table.
- `MatchObject`
  - Disposition: move
  - Owner after cleanup: `barn/scheduler`
  - Action: moved object resolution used by command dispatch out of `server`.
  - Evidence: resolves command targets against live store state for scheduler dispatch.
- `VerbMatch`, `FindVerb`
  - Disposition: move
  - Owner after cleanup: `barn/scheduler`
  - Action: moved parsed-command verb dispatch matching out of `server`.
  - Evidence: scheduler task creation consumes `VerbMatch`; server only hosts connections.

Gate results:
- Pass: `go test ./command ./scheduler ./server`
- Pass: `rg -n "commandWordList|func ParseCommand|type ParsedCommand|type PrepSpec|type VerbMatch|func MatchObject|func FindVerb" server -g "*.go"`
- Pass: `rg -n "barn/scheduler" command -g "*.go"`
- Pass: `rg -n "barn/server" command scheduler -g "*.go"`
- Pass: `git diff --check`

Commit:
- Not committed in this turn.

Next slice:
- Move temporary command dispatch matching out of `barn/scheduler` and into `barn/command`.

## Iteration 2 - `command dispatch matching owner correction`

Slice read:
- `scheduler/matcher.go`
- `scheduler/verbs.go`
- `command/command.go`
- server callers in `server/scheduler.go`, `server/scheduler_task_factory.go`, and `server/scheduler_task_runtime.go`

Surfaces:
- `MatchObject`
  - Disposition: move
  - Owner after cleanup: `barn/command`
  - Action: moved object resolution from `barn/scheduler` to `barn/command`.
  - Evidence: this resolves noun phrases from parsed command text; it is command semantics, not task scheduling.
- `VerbMatch`, `FindVerb`, prep/argspec matching
  - Disposition: move
  - Owner after cleanup: `barn/command`
  - Action: moved parsed-command dispatch matching from `barn/scheduler` to `barn/command`.
  - Evidence: this selects a command verb from player/location/dobj/iobj candidates; scheduler only consumes the chosen match to run a task.

Gate results:
- Pass: `go test ./command ./server`
- Pass: `rg -n "barn/scheduler|scheduler\\.MatchObject|scheduler\\.FindVerb|scheduler\\.VerbMatch" server command -g "*.go"`
- Pass: `rg -n "package scheduler" scheduler -g "*.go"`
- Pass: `git diff --check`

Commit:
- Pending in this turn.

Next slice:
- Move the concrete `Scheduler` runtime owner out of `server`.

## Iteration 3 - `scheduler runtime and server input split`

Slice read:
- `server/scheduler.go`
- `server/connection_manager.go`
- `server/server.go`
- `server/scheduler_login.go`
- `server/scheduler_task_factory.go`
- `server/scheduler_task_load.go`
- `server/scheduler_task_runtime.go`
- `server/scheduler_call_verb.go`
- `server/scheduler_traceback.go`
- `server/scheduler_eval.go`
- `server/waif_lifecycle.go`
- `server/task_queue.go`

Surfaces:
- `InputEvent`
  - Disposition: move
  - Owner after cleanup: `barn/command`
  - Action: moved input event shape out of `server` into `command`.
  - Evidence: the event is command/input data consumed by server input processing; it is not scheduler runtime state.
- `Scheduler`, task queue, task creation/load/run, call-verb runtime, eval runtime, traceback logging, waif lifecycle
  - Disposition: move
  - Owner after cleanup: `barn/scheduler`
  - Action: moved task/VM runtime implementation to `barn/scheduler`.
  - Evidence: these surfaces own queued tasks, VM execution, fork/resume/suspend, task snapshots, and runtime traceback state.
- connection/session input flow, login hooks, programming mode, command dispatch from a connection
  - Disposition: rewrite
  - Owner after cleanup: `barn/server.InputProcessor`
  - Action: deleted `server.Scheduler` and recreated only server-owned input/session behavior as `InputProcessor`.
  - Evidence: these paths require `Connection`, `ConnectionManager`, listener object state, output prefixes/suffixes, and connection lifecycle state.
- task output, traceback delivery, and task output flushing
  - Disposition: rewrite
  - Owner after cleanup: `server` delivery through concrete scheduler callbacks.
  - Action: scheduler no longer imports or names server/connection/transport types; server wires concrete send/traceback/flush functions during database load.
  - Evidence: scheduler owns runtime events, while server owns connections and performs delivery.
- `evalConnection`
  - Disposition: delete
  - Owner after cleanup: none.
  - Action: removed the connection-shaped eval interface from scheduler; eval runtime returns formatted output lines and server sends them.
  - Evidence: a connection-like interface in scheduler would recreate transport ownership under another name.

Gate results:
- Pass: `go test ./command ./scheduler ./server`
- Pass: `rg -n "package server|barn/server|connManager|Connection|ConnectionManager|Transport|WebSocket|listener|evalConnection" scheduler -g "*.go"`
- Pass: `rg -n "NewScheduler\\(|type Scheduler|\\*Scheduler|InputEvent" server command scheduler -g "*.go"` shows `Scheduler` only in `scheduler`, `InputEvent` defined in `command`, and server using `command.InputEvent`.
- Pass: `go test ./builtins ./bytecode ./cmd/barn ./command ./db/... ./kernel ./parser ./scheduler ./server ./task ./types ./vm`
- Pass: `go build -o barn.exe ./cmd/barn`
- Pass: `git diff --check`
- Pass: `uv run --project ..\moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"` - 3871 passed, 131 skipped.
- Expected external-fixture failure: `go test ./...` fails only in `barn/conformance` because `../cow_py/tests/conformance` is not present.

Commit:
- Pending in this turn.

Next slice:
- Fixed point reached for this requested scheduler extraction slice.
