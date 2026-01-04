# Barn - Go MOO Server

## CRITICAL: What The Spec Is

**The spec documents ToastStunt behavior. Period.**

- Toast is the reference implementation
- If Toast has a function → spec documents it
- If Toast doesn't have a function → spec should NOT document it
- Barn's implementation status is IRRELEVANT to the spec
- "[Not Implemented]" is MEANINGLESS in the spec - remove it
- The spec is NOT a Barn status document
- The spec is NOT aspirational features nobody has built

**When auditing specs:**
- Test against Toast to find what Toast ACTUALLY does
- If spec says something Toast doesn't do → FIX THE SPEC (remove it)
- If Toast does something spec doesn't say → FIX THE SPEC (add it)
- Barn matching or not matching is a separate concern

**Barn's job:** Implement what the spec says (which is what Toast does)

---

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
| moo-conformance-tests | `~/code/moo-conformance-tests/` | YAML-based conformance test suite |
| moo_interp | `~/code/moo_interp/` | Python MOO interpreter |
| cow_py | `~/code/cow_py/` | Python MOO server (no longer has conformance tests) |
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

Tests live in `~/code/moo-conformance-tests/` as a standalone pytest plugin package.

**Test YAML files:** `~/code/moo-conformance-tests/src/moo_conformance/_tests/`

### Running Tests Against Any MOO Server

```bash
# From barn directory (has moo-conformance-tests as dependency)
cd ~/code/barn

# Run against Toast (port 9501)
uv run pytest --pyargs moo_conformance --moo-port=9501 -v

# Run against Barn (port 9500)
uv run pytest --pyargs moo_conformance --moo-port=9500 -v

# Stop on first failure
uv run pytest --pyargs moo_conformance --moo-port=9501 -x -v

# Run specific test by name
uv run pytest --pyargs moo_conformance -k "arithmetic::addition" --moo-port=9501 -v

# Run specific category
uv run pytest --pyargs moo_conformance -k "arithmetic" --moo-port=9501 -v
```

### Test Options

| Option | Description |
|--------|-------------|
| `--moo-port PORT` | Port to connect to (required) |
| `--moo-host HOST` | Host to connect to (default: localhost) |
| `-x` | Stop on first failure |
| `-v` | Verbose output |
| `-k "pattern"` | Filter tests by name pattern |
| `--tb=long` | Show full tracebacks |

### Test Database

Both Toast and Barn use `Test.db`. Connect as wizard with `connect wizard`.

### Current Test Status

- 1465 total tests
- 1233 pass on both Toast and Barn
- 67 fail on both (test bugs, not server bugs)
- 165 skipped

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
