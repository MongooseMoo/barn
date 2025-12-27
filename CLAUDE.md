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

### Manual Testing

```bash
# Quick manual test against barn:
printf 'connect wizard\n; return 1 + 1;\n' | nc -w 3 localhost 9300

# Expected output:
# -=!-^-!=-
# {1, 2}
# -=!-v-!=-
```

### Test Database

Barn uses `Test.db` which creates new wizard players on `connect wizard`. Each connection gets a fresh wizard player object.

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
