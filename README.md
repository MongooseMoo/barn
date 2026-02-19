# Barn

A Go implementation of a MOO server, built conformance-first against a standardized test suite. Barn validates its implementation against [moo-conformance-tests](https://github.com/mongoosemoo/moo-conformance-tests) and cross-checks behavior with the ToastStunt reference implementation.

## What is MOO?

MOO (MUD Object Oriented) is a programmable virtual world server where everything is an object that can have properties and verbs (methods). Users connect via telnet, interact with objects, and write code in the MOO language to extend the world. Originally created at Xerox PARC in the early 1990s, MOO servers still power text-based virtual communities today.

## Conformance-Driven Development

Barn's defining characteristic: **every language feature is validated against a portable test suite**.

The [moo-conformance-tests](https://github.com/mongoosemoo/moo-conformance-tests) repository provides 1,465 YAML test cases covering:
- Parser correctness (operators, expressions, statements)
- Builtin functions (18 categories: math, strings, lists, objects, crypto, etc.)
- VM behavior (scoping, exceptions, task suspension/resumption)
- Object system (properties, verbs, inheritance, permissions)
- Edge cases and error handling

Tests run against any MOO server via telnet, enabling direct comparison:

```bash
# Test Barn
uv tool run ..\moo-conformance-tests --moo-port=9500

# Test ToastStunt for reference
uv tool run ..\moo-conformance-tests --moo-port=9501
```

The conformance suite now supports managed server lifecycle (`--server-command`) in the local repo checkout, which can auto-start/stop Barn and isolate DB writes in a temp copy.

When Barn diverges from expected behavior, the same test case runs against ToastStunt to determine the correct interpretation. This methodology catches subtle semantic differences that manual testing misses.

See [moo-conformance-tests documentation](https://github.com/mongoosemoo/moo-conformance-tests) for test structure and contributing tests.

## Conformance Runbook

Preferred workflow:

1. Build Barn.
2. Run conformance in managed-server mode (auto start/stop, temp DB copy):

```powershell
go build -o barn.exe ./cmd/barn/
uv run --project ..\moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
```

Manual mode still works:

```powershell
# Terminal 1
go build -o barn.exe ./cmd/barn/
.\barn.exe -db Test.db -port 9500

# Terminal 2
uv tool run ..\moo-conformance-tests --moo-port=9500
```

Note: `uv tool run ..\moo-conformance-tests` uses the packaged tool version and may not include the newest CLI flags yet. Use `uv run --project ..\moo-conformance-tests moo-conformance ...` to use the local checkout's latest features.

## Getting Started

```bash
# Build server
go build -o barn.exe ./cmd/barn/

# Run (uses Test.db by default)
./barn.exe

# Connect and test (in another terminal)
go build -o moo_client.exe ./cmd/moo_client/
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return 1 + 1;"
```

Barn uses `Test.db` which creates new wizard players on `connect wizard`. Each connection gets a fresh wizard object for testing.

## Command-Line Tools

| Tool | Purpose |
|------|---------|
| `barn` | Main MOO server |
| `moo_client` | Send commands and capture output (use this, not nc/telnet) |
| `dump_verb` | Display verb code from objects (`dump_verb 0 do_login_command`) |
| `check_player` | Inspect player object properties |
| `db_roundtrip` | Test database load/save cycles |
| `toast_oracle` | Query ToastStunt reference implementation for expected behavior |

Build any tool: `go build -o <tool>.exe ./cmd/<tool>/`

## Architecture

```
barn/
├── vm/              # Bytecode compiler and evaluator (~8,800 lines)
├── builtins/        # 18 builtin categories (~9,200 lines)
├── parser/          # MOO language lexer and AST (~4,000 lines)
├── db/              # Database I/O (ToastStunt .db format)
├── server/          # Network server, connection management, task scheduler
├── types/           # MOO value types (int, list, map, obj, error, etc.)
├── task/            # Task/coroutine management
└── spec/            # MOO language specification (31 markdown docs)
```

The `spec/` directory contains the reference specification developed through systematic auditing of ToastStunt source code. Each builtin category and language feature has detailed documentation of expected behavior, edge cases, and error conditions.

## Current Status

**Working:**
- Full MOO language parser and bytecode compiler
- Stack-based VM with control flow (if/for/while/fork/try-catch)
- Object system with properties, verbs, inheritance
- 18 builtin categories (math, strings, lists, objects, crypto, json, tasks, etc.)
- Network server with concurrent connection handling
- Task scheduler with suspend/resume
- Database persistence (load/save ToastStunt .db files)

**Active development:**
- Conformance test coverage improvements
- Edge case fixes discovered through cross-validation
- Performance optimization for large object counts

## Specification Documents

See [`spec/`](spec/) for complete MOO language documentation:

- **Core:** [Operators](spec/operators.md), [Control Flow](spec/control_flow.md), [Exceptions](spec/exceptions.md), [Tasks](spec/tasks.md), [Object System](spec/object_system.md)
- **Builtins:** [Math](spec/builtins/math.md), [Strings](spec/builtins/strings.md), [Lists](spec/builtins/lists.md), [Objects](spec/builtins/objects.md), [Crypto](spec/builtins/crypto.md), and [13 more](spec/builtins/)

Each document includes function signatures, behavior specifications, error conditions, and cross-references to reference implementations.

## Resources

- [moo-conformance-tests](https://github.com/mongoosemoo/moo-conformance-tests) - Portable test suite for MOO servers
- [ToastStunt](https://github.com/lisdude/toaststunt) - C++ reference implementation
- [LambdaMOO Programmer's Manual](https://www.hayseed.net/MOO/manuals/ProgrammersManual.html) - Original language documentation
