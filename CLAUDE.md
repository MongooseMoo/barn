# Barn - Go MOO Server

## RULE ZERO: WHEN SOMETHING FAILS ON BARN

**STOP. DO NOT DEBUG. DO NOT MAKE TOOL CALLS.**

Before ANY debugging action, you MUST:

1. **SAY WORDS FIRST** - Explain to Q what failed and what you will test against Toast
2. **TEST AGAINST TOAST** - Run the exact same operation on Toast to see correct behavior
3. **COMPARE** - Only after seeing Toast's behavior, identify where Barn diverges

**PRE-DEBUG CHECKLIST (must complete before any Barn debugging):**
```
□ Did I test this exact operation against Toast?
□ Do I know what Toast returns/does?
□ Have I explained to Q what I'm about to verify?
```

If any box is unchecked: **DO NOT PROCEED WITH DEBUGGING.**

Your instinct on failure is to immediately start investigating Barn code. **THAT INSTINCT IS WRONG.** Override it. Test Toast first. Every single time.

**If you catch yourself debugging Barn without having tested Toast: STOP IMMEDIATELY.**

---

## CRITICAL: NEVER Touch The Conformance Tests

**The moo-conformance-tests are SACRED. They are the spec. They define correct behavior.**

- **NEVER add skip markers** to conformance test YAML files
- **NEVER modify test expectations** to match Barn's broken behavior
- **NEVER weaken assertions** in any test
- **NEVER remove tests** that Barn fails
- **NEVER celebrate "0 failures"** if tests are being skipped that shouldn't be

**When a conformance test fails, there is exactly ONE correct response: FIX BARN.**

Not "skip it." Not "it needs complex infrastructure." Not "legitimate skip." **FIX. BARN.**

The conformance tests live in `~/code/moo-conformance-tests/`. They are a **read-only dependency**. If you find yourself editing files in that directory, you are doing the wrong thing.

---

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

**SEE RULE ZERO ABOVE. This is not optional.**

When Barn produces an error running MOO code from toastcore.db or other reference databases:

**ASSUME YOUR CODE IS WRONG.** That MOO code has worked for years/decades.

Before concluding external code is broken:
```bash
# Test the expression against Toast oracle
./toast_oracle.exe 'toint("[::1]")'
# Returns: 0

# If Toast returns a value and Barn returns an error, YOUR CODE IS WRONG
```

For server-level testing (login flows, connections, etc.):
```bash
# Start Toast with the database
C:/Users/Q/src/toaststunt/test/moo.exe mongoose.db mongoose.db.new 9451
# (Toast syntax here is: input-db output-db port)

# Test against Toast
./moo_client.exe -port 9451 -cmd "..."

# Compare with Barn on different port
./moo_client.exe -port 9450 -cmd "..."
```

The toast_oracle tool exists for expression testing. For server behavior, run Toast as a server and compare.

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

## CRITICAL: Fix Tooling First

When a tool doesn't work, **fix the tool** - don't work around it with debug logging or manual inspection. Time spent fixing tooling pays dividends. Time spent on workarounds compounds into more workarounds.

Examples:
- dump_verb doesn't load mongoose.db → Fix dump_verb, don't add printf debugging
- cow_py fails to parse database → Fix the parser or use barn's own loader
- Test harness unreliable → Fix harness, don't run tests manually

---

## CRITICAL: Bash Commands on MSYS/Windows

This environment runs MSYS (Git Bash). Common gotchas:

**sleep**: Takes `NUMBER[SUFFIX]`, not flags.
```bash
sleep 3      # Correct - sleeps 3 seconds
sleep 3s     # Correct - explicit seconds
sleep -3     # WRONG - "-3" interpreted as invalid flag
```

When a command fails with "unknown option", STOP and figure out the correct syntax before proceeding.

---

## Project Overview

Barn is a Go implementation of a MOO (MUD Object Oriented) server. Currently in **spec-first phase** - no Go code until spec + tests are complete.

## Key Principle

**Zero lines of Go code until spec + tests are complete.**

## Reference Implementations

| Name | Path | Description |
|------|------|-------------|
| ToastStunt | `~/src/toaststunt/` | C++ MOO server (primary reference), binary at `test/moo.exe` |
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

Tests live in `~/code/moo-conformance-tests/` and are run via its CLI tool.

**Test YAML files:** `~/code/moo-conformance-tests/src/moo_conformance/_tests/`

### Running Tests Against Any MOO Server

```bash
# From barn directory (has moo-conformance-tests as dependency)
cd ~/code/barn

# Run against Toast (port 9501)
uv tool run ..\moo-conformance-tests --moo-port=9501

# Run against Barn (port 9500)
uv tool run ..\moo-conformance-tests --moo-port=9500
```

### Managed Server Lifecycle (Preferred With Local Checkout)

Use the local `moo-conformance-tests` checkout to get the latest CLI features (`--server-command`, `--server-db`):

```powershell
go build -o barn.exe ./cmd/barn/
uv run --project ..\moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
```

This auto-starts/stops Barn and runs against a temp copy of `Test.db`.

Note: `uv tool run ..\moo-conformance-tests ...` may point at a packaged version that does not yet expose new flags.

### Test Options

| Option | Description |
|--------|-------------|
| `--moo-port PORT` | Port to connect to (required) |
| `--moo-host HOST` | Host to connect to (default: localhost) |
| `--server-command CMD` | Start/stop server automatically (`{port}` and `{db}` placeholders) |
| `--server-db PATH` | DB file used in managed mode (defaults to bundled `Test.db`) |

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
