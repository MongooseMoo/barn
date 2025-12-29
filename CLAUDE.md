# Barn - Go MOO Server

## CRITICAL: Subagent File Writing Failures

**If Edit/Write fails with "file unexpectedly modified", follow this procedure:**

1. Try `./relative/path.py` (relative with dot)
2. Try `C:/Users/Q/absolute/path.py` (forward slashes)
3. Try `C:\Users\Q\absolute\path.py` (backslashes)
4. Try `relative/path.py` (bare relative)

**If ALL FOUR fail:**
- STOP IMMEDIATELY
- Report "I cannot continue - all path formats failed for [filename]"
- DO NOT use cat, echo, sed, or any bash workaround
- TERMINATE

Using bash commands to write files when Edit/Write fail DESTROYS FILES.
The path format workaround works. Bash workarounds do not. Try paths first, then stop.

---

## CRITICAL: Test Against Toast Before Blaming External Code

When Barn produces an error running MOO code from toastcore.db or other reference databases:

**ASSUME YOUR CODE IS WRONG.** That MOO code has worked for years/decades.

Before concluding external code is broken:
```bash
# Test the expression against Toast oracle
./toast_oracle.exe 'toint("[::1]")'
# Returns: 0

# If Toast returns a value and Barn returns an error, YOUR CODE IS WRONG
```

The toast_oracle tool exists for exactly this purpose. Use it.

---

## CRITICAL: Test.db IS THE SAME DATABASE TOAST USES

**Test.db is the SAME database that ToastStunt uses for its conformance tests.**

This means:
- If a test passes on Toast with Test.db, **the database is CORRECT**
- If the same test fails on Barn with Test.db, **BARN'S CODE IS BROKEN**
- **NEVER blame the database** - $waif, $anon, prototype properties, EVERYTHING EXISTS
- **NEVER say "need to set up database"** - it's already set up, Toast proves it works
- **NEVER say "database missing X"** - if Toast works, X exists

When tests fail on Barn but pass on Toast: **THE BUG IS IN BARN. PERIOD.**

Stop making excuses. Fix Barn's code.

---

## Project Overview

Barn is a Go implementation of a MOO (MUD Object Oriented) server. Currently in **spec-first phase** - no Go code until spec + tests are complete.

## Key Principle

**Zero lines of Go code until spec + tests are complete.**

## Reference Implementations

| Name | Path | Description |
|------|------|-------------|
| ToastStunt | `~/src/toaststunt/` | C++ MOO server (primary reference) |
| moo_interp | `~/code/moo_interp/` | Python MOO interpreter |
| cow_py | `~/code/cow_py/` | Python MOO server with conformance tests |
| lambdamoo-db-py | `~/src/lambdamoo-db-py/` | LambdaMOO database parser |

## Directory Structure

```
barn/
├── spec/           # MOO language specification
│   ├── builtins/   # 17 builtin category specs
│   └── *.md        # Core spec documents
├── prompts/        # Subagent prompts for spec auditing
└── CLAUDE.md       # This file
```

## Conformance Tests

Tests live in `~/code/cow_py/tests/conformance/` and are shared between Python and Go implementations.

### Running Tests Against Barn (Go Server)

```bash
# 1. Build barn
cd ~/code/barn
go build -o barn_test.exe ./cmd/barn/

# 2. Start barn server on a free port
./barn_test.exe -db Test.db -port 9300 > server.log 2>&1 &

# 3. Wait for server to start
sleep 2

# 4. Run conformance tests via socket transport
cd ~/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9300 -v

# Stop on first failure (recommended for debugging):
uv run pytest tests/conformance/ --transport socket --moo-port 9300 -x -v

# Run specific test category:
uv run pytest tests/conformance/ --transport socket --moo-port 9300 -k "arithmetic" -v

# Skip known problematic tests:
uv run pytest tests/conformance/ --transport socket --moo-port 9300 -k "not property and not crypt" -x -v
```

### Running Tests Against cow_py (Python Server - Direct)

```bash
cd ~/code/cow_py
uv run pytest tests/conformance/ -v  # Uses direct transport by default
```

### Test Options

| Option | Description |
|--------|-------------|
| `--transport socket` | Connect to external MOO server via TCP |
| `--transport direct` | Use cow_py's Python implementation directly (default) |
| `--moo-port PORT` | Port for socket transport (default: 7777) |
| `--moo-host HOST` | Host for socket transport (default: localhost) |
| `-x` | Stop on first failure |
| `-v` | Verbose output |
| `-k "pattern"` | Filter tests by name pattern |

### Manual Testing with moo_client

Use the `moo_client` tool for interactive testing:

```bash
# Build the client
go build -o moo_client.exe ./cmd/moo_client/

# Send commands and capture output
./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return 1 + 1;"

# With longer timeout for slow operations
./moo_client.exe -port 9300 -timeout 5 -cmd "connect wizard" -cmd "look"

# Commands from a file
./moo_client.exe -port 9300 -file test_commands.txt
```

**Do NOT use printf/nc for testing** - it's unreliable and loses output.

### Test Database

Barn uses `Test.db` which creates new wizard players on `connect wizard`. Each connection gets a fresh wizard player object.

## Database Inspection Tools

### dump_verb - Display Verb Code

```bash
# Build
go build -o dump_verb.exe ./cmd/dump_verb/

# Dump a specific verb from an object
./dump_verb.exe 0 do_login_command    # #0:do_login_command
./dump_verb.exe 2 look                # #2:look

# Lists available verbs if verb not found
./dump_verb.exe 0 nonexistent
```

### check_player - Inspect Player Objects

```bash
# Build
go build -o check_player.exe ./cmd/check_player/

# Inspect wizard object (default)
./check_player.exe

# With custom database
./check_player.exe -db MyGame.db
```

### cow_py Database Tools (Reference)

For more advanced database inspection, use cow_py's CLI:

```bash
cd ~/code/cow_py
uv run cow_py db obj #0              # Show object info
uv run cow_py db verbs #0            # List verbs on object
uv run cow_py db verb #0 do_login_command  # Show verb code
uv run cow_py db props #2            # List properties
uv run cow_py db ancestry #2         # Show parent chain
```

## Spec Audit Workflow

Two-agent loop for finding and fixing specification gaps:

1. **blind-implementor-audit.md** - Audits spec as if implementing from scratch, documents gaps
2. **spec-patcher.md** - Takes gaps, researches implementations, patches spec

See `prompts/README.md` for details.

## Current Phase

Phase 1: Specification (complete)
Phase 2: Test suite completion (in progress)
Phase 3: Go implementation (in progress)

## Go Tools Available

| Tool | Install | Usage |
|------|---------|-------|
| gorename | `go install golang.org/x/tools/cmd/gorename@latest` | Type-safe renaming: `gorename -from '"barn/vm".Evaluator.evalFoo' -to foo` |

Use these instead of manual string replacement for refactoring.
