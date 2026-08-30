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

## CRITICAL: Preserve Conformance Truth

`C:/Users/Q/code/moo-conformance-tests` owns durable behavioral truth. Do not
weaken, skip, remove, or rewrite expectations to accommodate Barn. A genuine
behavioral discovery belongs in that repository as a Toast-passing conformance
test, committed separately from any Barn change.

Follow the Toast-first loop in
`plans/barn-toast-mongoose-convergence-workstreams.md`: prove the unchanged
scenario on the managed WSL Toast oracle, then run it on Barn. If Barn fails,
make the smallest Barn production fix. If Barn already passes, keep the test as
coverage and do not invent a Barn source change.

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

## CRITICAL: Test Against The Managed WSL Toast Oracle First

**SEE RULE ZERO ABOVE. This is not optional.**

When Barn produces an error running MOO code from a reference database, verify
the exact behavior through the managed conformance workflow before diagnosing
Barn or blaming the database. The only ordinary Toast server authority is the
WSL oracle documented in `plans/barn-toast-mongoose-convergence-workstreams.md`; do not substitute a
Windows Toast binary, an attached port, or a manually managed process.

---

## CRITICAL: Use The Exact Selected Fixture

The managed harness gives each engine a disposable copy of the selected input
database. Record and compare the source fixture's path, size, and freshly
computed SHA-256; a nearby database with a similar name is not a substitute.
When the unchanged test passes Toast and fails Barn on equivalent copies, the
delta is Barn-owned. When a candidate test passes both, retain the coverage and
do not invent a Barn change.

---

## CRITICAL: Fix Tooling First

When a tool doesn't work, **fix the tool** - don't work around it with debug logging or manual inspection. Time spent fixing tooling pays dividends. Time spent on workarounds compounds into more workarounds.

Examples:
- `barn -verb-code` doesn't load mongoose.db → Fix Barn's inspection path, don't add printf debugging
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
| ToastStunt | `/root/src/toaststunt/` in WSL | Primary reference; pinned release binaries and SHAs are in `plans/barn-toast-mongoose-convergence-workstreams.md` |
| moo-conformance-tests | `C:/Users/Q/code/moo-conformance-tests/` | YAML-based conformance test suite and managed server harness |
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

## Managed Conformance Workflow

Tests live in
`C:/Users/Q/code/moo-conformance-tests/src/moo_conformance/_tests/` and run via
the local checkout's `moo-conformance` CLI. Ordinary conformance work must use
its managed server lifecycle with explicit `--server-command` and `--server-db`;
do not attach to a pre-launched server or manage Barn or Toast manually.

For the exact stock and Mongoose WSL Toast commands, pinned engine identities,
disposable database behavior, login-script environment mechanism, required run
record, and cross-repository Toast-green/Barn-red/Barn-green loop, follow
`plans/barn-toast-mongoose-convergence-workstreams.md`.

Manual reproduction is exceptional and requires the user's explicit approval,
as specified in `AGENTS.md`.

## Reviewing What Happened On A Run

Every run writes a structured JSON log to `logs/latest.jsonl` (the previous run is
rotated to `logs/run-<timestamp>.jsonl`; the newest 10 are kept). This is on by
default — no flag needed.

**To find out what went wrong on the last run:**

```bash
go build -o barn_logs.exe ./cmd/barn_logs/
./barn_logs.exe -level error     # failures only; exits 1 if the run logged an error
./barn_logs.exe                  # warnings and errors
./barn_logs.exe -run list        # available runs
```

An uncaught MOO error is **one** log record carrying the rendered traceback, the
structured frames (verb, object, line, **source line**), and the error code. A
recovered panic carries a Go stack. `barn_logs` prints those indented under the
failure. The files are line-delimited JSON, so `jq` works too:

```bash
jq -c 'select(.level=="ERROR")' logs/latest.jsonl
```

**Caveat:** `Test.db` has genuinely broken object refs, so every boot logs ~30
`src=startup_repair` warnings (Toast repairs them too — conformance *asserts* these
messages). Use `-level error` to see what actually broke.

### Log flags and the debug endpoint

| Flag | Default | Purpose |
|------|---------|---------|
| `-log-level` | `info` | `debug`, `info`, `warn`, `error` |
| `-log-dir` | `logs` | Empty disables the JSON file sink |
| `-debug-addr` | `127.0.0.1:0` | pprof + expvar; `off` disables |

The debug endpoint binds an **ephemeral port**; find the real address in the log:

```bash
ADDR=$(jq -r 'select(.msg=="debug endpoint listening") | .addr' logs/latest.jsonl)
curl -s "http://$ADDR/debug/vars" | jq 'with_entries(select(.key|startswith("barn.")))'
curl -s "http://$ADDR/debug/pprof/heap" > heap.out
curl -X POST "http://$ADDR/debug/loglevel?level=debug"   # no restart needed
```

Counters: `barn.tasks_started`, `barn.tasks_killed`, `barn.uncaught_exceptions`,
`barn.panics_recovered` (nonzero = a Barn bug), `barn.checkpoints`,
`barn.checkpoint_last_ms`, `barn.gc_sweeps`, `barn.gc_sweep_last_ms`,
`barn.tasks_live`, `barn.connections_live`.

### Rules when touching logging

- **Log through `slog`, never the stdlib `log` package.** Use typed attrs
  (`slog.String`, `slog.Int64`) — the key/value variadic silently corrupts records on
  an odd arg count.
- **A MOO error code is not a Go error.** `slog.Any("err", E_TYPE)` emits `"err":1`.
  Use `slog.String("error", types.NewErr(code).String())` → `"error":"E_TYPE"`.
- **Some log message TEXT is a conformance contract.** `assert_log` pins
  `"CHECKPOINTING"` (dump_database), the startup-repair wording, and `server_log()`'s
  message. Keep the text byte-identical; structured attrs are *additive*. Grep the
  conformance suite for `assert_log` before rewording anything.
- Attr conventions: `task_id`, `player`, `this` (object a verb runs on), `verb`,
  `conn_id`, `error` (E_* name), `err` (Go error), `go_stack`, `traceback`, `frames`.

## Database Inspection Tools

Build the `barn` binary once; database inspection uses the same `-db` flag as
the server and exits without starting listeners.

```bash
go build -o barn.exe ./cmd/barn/

./barn.exe -db Test.db -verb-code '#0:do_login_command'
./barn.exe -db Test.db -list-verbs '#0'
./barn.exe -db Test.db -obj-info '#2'
./barn.exe -db Test.db -eval '1 + 2'
./barn.exe -db Test.db -dump-obj-raw '#2'
./barn.exe -db Test.db -verb-lookup '#2:look'
./barn.exe -db Test.db -ancestry '#2'
./barn.exe -db Test.db -dump copy.db  # writes, reloads, and compares persistence fields
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
