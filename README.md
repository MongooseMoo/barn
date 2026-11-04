# Barn

Barn is a MOO server written in Go. It loads [ToastStunt](https://github.com/lisdude/toaststunt)
databases and runs their MOO code, and the goal is for it to behave exactly
like ToastStunt, so an existing MOO can change servers without touching its
database.

What changes is underneath. ToastStunt runs MOO tasks one at a time. Barn
runs them in parallel across CPU cores: each task reads a snapshot of the
database, and if another task changed something it read before it finished,
Barn discards that attempt and runs it again. MOO code still sees the world
as if tasks ran one after another.

Barn is developed against Mongoose, a large, long-running MOO, as its real
workload.

## What's a MOO?

A MOO is a multi-user text world where everything (rooms, players, objects)
is an object with properties and verbs, and the world is programmed from
inside itself in the MOO language. LambdaMOO made the idea popular, and
ToastStunt is its actively maintained descendant.

## Status

Barn is under active development. ToastStunt compatibility is tested, not
assumed: CI runs the [moo-conformance-tests](https://github.com/MongooseMoo/moo-conformance-tests)
suite against Barn on every change. The suite's expected results are
ToastStunt's actual behaviour.

One difference is deliberate: Barn strings are Unicode text, while ToastStunt
strings are bytes. In Barn, `length("é")` is 1.

## Quick start

You need Go 1.27 or newer (see `go.mod`).

```sh
go build -o bin/ ./cmd/barn
bin/barn -db Test.db -port 7777
```

Connect any MOO client or telnet to `localhost:7777` and type
`connect wizard`. `Test.db` is the small test database that ships in the
repository.

To run MOO code without starting a server:

```sh
go build -o bin/ ./cmd/moo
bin/moo -e 'return 1 + 2;'
bin/moo program.moo
```

`make build` builds every tool into `bin/`. On Windows, Go adds `.exe`.

## Running a server

Common flags:

| Flag | Purpose |
|------|---------|
| `-db <path>` | Database file (default `Test.db`) |
| `-port <n>` | TCP port (default `7777`) |
| `-listen <url>` | Listener URL such as `tcp://:7777`; can be repeated |
| `-config <path>` | Server options file (examples in `profiles/barn/`) |
| `-checkpoint-interval <s>` | Seconds between checkpoints (default `3600`, `0` disables) |
| `-log-level`, `-log-dir` | Log verbosity and where JSON logs go (default `logs/`) |

`bin/barn -h` lists everything else.

Barn can also inspect a database and exit without opening any ports:

```sh
bin/barn -db Test.db -eval 'return #0.name;'
bin/barn -db Test.db -verb-code '#0:do_login_command'
bin/barn -db Test.db -obj-info '#2'
bin/barn -db Test.db -dump copy.db     # write, reload and compare
```

There are also `-list-verbs`, `-verb-lookup`, `-ancestry`, `-dump-obj-raw`,
`-eval-file` and `-memory-report`.

## Tools

| Command | What it does |
|---------|--------------|
| `barn` | The server, plus the inspection flags above |
| `moo` | Runs MOO code against a database with no network |
| `moo_client` | Sends commands to a running server (`-cmd`, `-file`) |
| `barn_logs` | Reads Barn's run logs; `barn_logs -level error` shows what failed |
| `toast_oracle` | Runs an expression on a ToastStunt binary (`-bin`, `-db`) |
| `mongoose_tz` | Mongoose's timezone helper program, for Windows |

## Development

```sh
go test ./...
make conformance     # Linux or WSL: the managed conformance suite
```

`make conformance` needs a checkout of
[moo-conformance-tests](https://github.com/MongooseMoo/moo-conformance-tests)
next to this one and [uv](https://docs.astral.sh/uv/). On Windows,
`scripts/run-conformance.ps1` does the same job:

```powershell
.\scripts\run-conformance.ps1 -Build -Binary .\bin\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788
```

When Barn and ToastStunt disagree, ToastStunt is right. A new behaviour is
first written as a conformance test that passes on ToastStunt, then Barn is
fixed to pass it. [AGENTS.md](AGENTS.md) has the full rules.

Where things are:

| Path | Contents |
|------|----------|
| `parser/`, `compiler/`, `bytecode/`, `vm/` | MOO source to bytecode, and the interpreter |
| `builtins/` | Builtin functions |
| `engine/`, `task/` | Running verbs and tasks: fork, suspend, retry |
| `db/store/` | The in-memory object store and its snapshots |
| `db/format/` | Reading and writing ToastStunt database files |
| `server/`, `internal/listener/` | Connections, login, TCP/TLS/WebSocket |
| `types/` | MOO values |
| `spec/` | How ToastStunt behaves, written down |
| `docs/` | Design notes and operating guides |

The Windows DLLs at the repository root (SQLite, PCRE, Argon2, Nettle and the
MinGW runtime) are required there; leave them tracked.

## Further reading

- [`spec/`](spec/README.md): the MOO language and builtins as ToastStunt implements them
- [`docs/TRACING.md`](docs/TRACING.md): execution tracing
- [`docs/operator-http.md`](docs/operator-http.md): liveness and readiness probes
- [`docs/pgo.md`](docs/pgo.md): the profile-guided build and how to refresh it
- [LambdaMOO Programmer's Manual](https://www.hayseed.net/MOO/manuals/ProgrammersManual.html)
