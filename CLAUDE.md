# Barn - Go MOO Server

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
Phase 3: Go implementation (not started)
