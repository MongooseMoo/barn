# Barn

Barn is a Go implementation of a MOO server. It includes a parser, bytecode VM,
ToastStunt-format database reader/writer, TCP server, task scheduler, built-in
function registry, and conformance tooling for comparing behavior with existing
MOO implementations.

## What is MOO?

MOO (MUD Object Oriented) is a programmable virtual world server where
everything is an object with properties and verbs. Users connect over a text
protocol, interact with objects, and write MOO-language code to extend the
world.

## Requirements

- Go 1.24.6, matching `go.mod`
- PowerShell for the checked-in conformance runner
- `uv` when running the Python conformance suite
- A local `..\moo-conformance-tests` checkout when using the `uv` project setup
  in this repository

## Getting Started

Build and run the server:

```powershell
go build -o barn.exe ./cmd/barn/
.\barn.exe -db Test.db -port 7777
```

Send commands from another terminal:

```powershell
go build -o moo_client.exe ./cmd/moo_client/
.\moo_client.exe -port 7777 -cmd "connect wizard" -cmd "; return 1 + 1;"
```

`cmd/barn` defaults to `Test.db`, port `7777`, and a 3600-second checkpoint
interval.

## Server Options

The main server binary is built from `./cmd/barn/`.

Common runtime flags:

| Flag | Purpose |
|------|---------|
| `-db <path>` | Database file path, default `Test.db` |
| `-port <n>` | TCP listen port, default `7777` |
| `-checkpoint-interval <seconds>` | Periodic checkpoint interval, default `3600`; use `0` to disable |
| `-trace` | Enable execution tracing |
| `-trace-filter <glob[,glob...]>` | Limit tracing to matching verb names |

Database and inspection flags exit after completing the requested operation:

| Flag | Purpose |
|------|---------|
| `-dump <path>` | Load the database and write a ToastStunt-format dump |
| `-verb-code #obj:verb` | Print verb source and metadata |
| `-list-verbs #obj` | List verbs defined on an object |
| `-obj-info #obj` | Print object metadata, properties, and verbs |
| `-eval <expr>` | Parse and evaluate a MOO expression against the database |
| `-dump-obj-raw #obj` | Print raw object fields for debugging |
| `-verb-lookup #obj:verb` | Show where a verb resolves in the parent chain |
| `-ancestry #obj` | Print an object's parent chain |

## Command-Line Tools

Build any tool with:

```powershell
go build -o <tool>.exe ./cmd/<tool>/
```

| Tool | Purpose |
|------|---------|
| `barn` | Main server plus database inspection commands |
| `moo_client` | Send commands to a running MOO server using `-cmd` or `-file` |
| `dump_verb` | Print a verb directly from a database object |
| `dump_prop` | Print a property directly from a database object |
| `db_roundtrip` | Load, write, reload, and compare a database file |
| `check_player` | Local diagnostic for wizard `#2` in a database |
| `toast_oracle` | Local ToastStunt expression diagnostic with hard-coded local paths |
| `gen_builtin_signatures` | Generate built-in function signature data |
| `test_crypt` | Local crypt/hash diagnostic |

## Conformance Workflow

The preferred managed conformance entrypoint in this repo is:

```powershell
.\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788
```

The script builds Barn when `-Build` is supplied, copies the source database to a
run database, starts the server, waits for the TCP listener, runs
`uv run pytest --pyargs moo_conformance`, stops the server, removes the run
database unless `-KeepRunDb` is set, and writes logs under `reports/runs/`.

Useful script flags:

| Flag | Purpose |
|------|---------|
| `-K <pattern>` | Pass a pytest `-k` selector |
| `-ExtraPytestArgs <args>` | Append pytest arguments |
| `-KeepRunDb` | Preserve the copied run database after the run |
| `-NoFreshDb` | Use the existing run database instead of copying `-SourceDb` |
| `-ReportsRoot <path>` | Change the report output directory |

The repository also contains a Go `conformance` package. Its loader currently
looks for legacy YAML tests under `..\cow_py\tests\conformance`; the managed
PowerShell runner above is the current repo-level workflow.

## Architecture

```text
barn/
|-- cmd/             # CLI entrypoints
|-- vm/              # Bytecode compiler and evaluator
|-- builtins/        # Built-in function implementations and registry
|-- parser/          # MOO lexer, parser, AST, and unparser
|-- db/              # ToastStunt-format database I/O
|-- server/          # TCP server, connection management, scheduler integration
|-- types/           # MOO value types and task context
|-- task/            # Task state, queues, and tracebacks
|-- conformance/     # Go-side conformance loader and runner
|-- scripts/         # Managed conformance runner
`-- spec/            # Local MOO behavior notes and reference specs
```

## Current Implementation Surface

Implemented areas visible in the current code:

- MOO lexer/parser and AST for expressions and statements
- Bytecode compiler and stack-based VM
- Object, property, verb, parent-chain, player, and waif support
- Task scheduling with suspend/resume, forked tasks, traceback formatting, and
  task-local builtins
- TCP connection handling, login hooks, user connection hooks, listener support,
  and connection-option builtins
- ToastStunt database load, checkpoint, dump, and round-trip support
- Built-in categories for types, strings, lists, maps, math, objects,
  properties, verbs, JSON, network, crypto, regex, file I/O, SQLite, exec,
  server/system behavior, time, tasks, and GC diagnostics

## Specification Documents

See [`spec/`](spec/) for local behavior documentation:

- Core: [Grammar](spec/grammar.md), [Types](spec/types.md),
  [Operators](spec/operators.md), [Statements](spec/statements.md),
  [Errors](spec/errors.md), [Objects](spec/objects.md),
  [Tasks](spec/tasks.md), [VM](spec/vm.md), [Server](spec/server.md),
  [Database](spec/database.md)
- Builtins: [Types](spec/builtins/types.md), [Math](spec/builtins/math.md),
  [Strings](spec/builtins/strings.md), [Lists](spec/builtins/lists.md),
  [Maps](spec/builtins/maps.md), [Objects](spec/builtins/objects.md),
  [Properties](spec/builtins/properties.md), [Verbs](spec/builtins/verbs.md),
  [Tasks](spec/builtins/tasks.md), [Time](spec/builtins/time.md),
  [JSON](spec/builtins/json.md), [File I/O](spec/builtins/fileio.md),
  [Network](spec/builtins/network.md), [Crypto](spec/builtins/crypto.md),
  [Regex](spec/builtins/regex.md), [SQLite](spec/builtins/sqlite.md),
  [Exec](spec/builtins/exec.md), [Server](spec/builtins/server.md)

## Resources

- [moo-conformance-tests](https://github.com/mongoosemoo/moo-conformance-tests)
- [ToastStunt](https://github.com/lisdude/toaststunt)
- [LambdaMOO Programmer's Manual](https://www.hayseed.net/MOO/manuals/ProgrammersManual.html)
