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

```bash
# Run all conformance tests
cd ~/code/cow_py
uv run pytest tests/conformance/ -v
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
