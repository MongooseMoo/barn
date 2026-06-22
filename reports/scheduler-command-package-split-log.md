# Cleanup Refactor Fixed-Point Log - 2026-06-22

Target architecture:
- `barn/command` owns command-language parsing and command dispatch matching: `ParsedCommand`, `PrepSpec`, `ParseCommand`, `CommandWordList`, `MatchObject`, `VerbMatch`, and `FindVerb`.
- `barn/scheduler` owns scheduler runtime orchestration, not command parsing or command matching.
- `barn/server` consumes those owners and keeps connection/transport/runtime hosting code.

Forbidden surfaces:
- Command parser types or functions owned by `server`.
- Verb dispatch matcher types or functions owned by `server`.
- Compatibility aliases or wrappers in `server` for moved command or matcher APIs.

Search gates:
- `rg -n "commandWordList|func ParseCommand|type ParsedCommand|type PrepSpec|type VerbMatch|func MatchObject|func FindVerb" server -g "*.go"`
- `rg -n "barn/scheduler" command -g "*.go"`
- `rg -n "barn/server" command scheduler -g "*.go"`

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
