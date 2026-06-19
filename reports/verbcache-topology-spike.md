# VERBCACHE TOPOLOGY SPIKE (throwaway, disposable branch `spike/verbcache`)

**Verdict: Design B is CLEAN, with exactly ONE narrow interface (`bytecode.Registry`) that
sits OFF the per-instruction hot path.** Item 2's dedicated-package topology is viable: a new
`barn/bytecode` package owns the bytecode `Program` type, the compiler, `VerbProgram`+`CompileVerb`,
and the cache entry point, imported by vm/builtins/server with NO import cycle and fully concrete
execution dispatch — and `db/store` becomes parser-free.

Branch: `spike/verbcache` off master `0bc99b3`. Worktree: `C:/Users/Q/code/barn-verbcache-spike`.
This branch is to be DISCARDED — it exists only to answer the topology question.

---

## 1. Chosen package name + what moved

**Package name: `barn/bytecode`** (preferred over `verbcache`/`compiler` because what physically
moves is the bytecode `Program` type + the compiler that produces it; the cache is a thin layer on top).

Moved INTO `barn/bytecode`:
| From | Symbol(s) |
|------|-----------|
| `vm/program.go` | `Program`, `LineEntry`, `LineForIP`, `ExtractForkBody`, `LoopType/LoopState`, `HandlerType/Handler`, `Matches` |
| `vm/opcodes.go` | `OpCode`, all `OP_*` consts, `OpCodeNames`, `MakeImmediateOpcode`, `IsImmediateInt`, `GetImmediateValue`, `CountsTick` |
| `vm/compiler.go` | the entire `Compiler` + `Compile`/`CompileStatements`/`CompileVerbBytecode` |
| `vm/parser_literals.go` | `valueFromLiteral`, `errorNameToCode`, `lowerErrorNames` (compile-time helpers) |
| `db/store/verbs.go` (deleted) | `CompileVerb` -> `bytecode/verbcache.go` |
| `db/store/object.go` | `VerbProgram` -> `bytecode/verbcache.go` |

New in `barn/bytecode`:
- `type Registry interface { GetID(name string) (int, bool) }` — the narrow compile-time dependency.
- `CompileVerbBytecode(code []string, registry Registry) (*Program, error)` — parser-free signature
  (takes source lines, not a `*db/store.Verb`), so bytecode does NOT import db/store.

Removed from `db/store.Verb`: the runtime-derived fields `Program *VerbProgram` and `BytecodeCache any`.
`db/store.Verb` now holds only persistent state (`Code []string` + metadata). `SetVerbCode`/`SetVerbCodeByIndex`
lost their `*VerbProgram` parameter. `object_bytes()` dropped its AST-size term.

`vm/bytecode_aliases.go` (new): re-exports `Program`/`OpCode`/`Compiler`/`LoopState`/`Handler` as
type aliases plus all `OP_*` consts and opcode helpers, so the VM execution engine (control.go,
op_*.go, vm.go, traceback.go, stack.go, registry.go) compiles UNCHANGED. In a real landing the
execution engine would reference `bytecode.X` directly; aliasing keeps the spike diff minimal while
still proving the topology. `vm/op_verb.go` now calls `bytecode.CompileVerbBytecode(verb.Code, vm.Builtins)`.

---

## 2. Before / after import graph

BEFORE (master 0bc99b3):
```
db/store  -> parser            (via VerbProgram.Statements + CompileVerb)
vm        -> builtins, db/store, parser, kernel, types   (owns Program + Compiler)
builtins  -> db/store, parser, ...   (reads verb.Program.Statements, calls dbstore.CompileVerb)
server    -> vm, db/store, parser, builtins ...
```
(`vm -> builtins` already existed; therefore builtins could NOT import vm.)

AFTER (spike):
```
bytecode  -> parser, types                      (LEAF-ish; owns Program + Compiler + VerbProgram + CompileVerb)
db/store  -> types                              (PARSER-FREE — proven below)
vm        -> bytecode, builtins, db/store, kernel, types
builtins  -> bytecode, db/store, parser, kernel, types
server    -> bytecode, vm, builtins, db/store, parser, ...
```
`bytecode` imports NONE of {vm, builtins, db/store} -> no cycle is even possible.

---

## 3. Cycle resolution (the crux)

Two hazards from the brief, and how each was resolved:

**(a) bytecode owns `Program`, but vm uses `Program` everywhere.**
Resolved one-directionally: the VM execution engine only READS `Program` (`frame.Program.Code[ip]`,
`switch op`), and the compiler only WRITES it. So `Program`+`OpCode`+compiler move wholesale into
`bytecode`; `vm -> bytecode` is the only edge; `bytecode` never needs anything from `vm`. Confirmed
the compiler references nothing from the VM execution engine (only parser, types, opcodes, and the
two compile-time literal helpers, which moved too).

**(b) Compilation needs `builtins.Registry`; builtins must call bytecode.**
This is the would-be cycle (`bytecode -> builtins` AND `builtins -> bytecode`). Resolved by
**`bytecode` defining its own narrow interface** `Registry interface { GetID(string)(int,bool) }`.
`*builtins.Registry` already has a `GetID(string)(int,bool)` method, so it satisfies the interface
STRUCTURALLY with zero changes to builtins and with bytecode NOT importing builtins. Callers
(`vm.Builtins`, `s.registry`, `builtins.NewRegistry()`) are passed straight through; Go's implicit
interface satisfaction does the binding. Verified: `go list -deps ./bytecode | grep -E 'builtins|vm|db/store'`
returns nothing.

Where the one indirection sits: `c.registry.GetID(name)` at `bytecode/compiler.go:1369`, reached only
from `compileBuiltinCall` — i.e. ONCE per builtin-call site at COMPILE time. The VM's per-instruction
execution loop never touches an interface for dispatch (see §4). So the single interface is off the hot path.

---

## 4. Evidence

**PROOF 1 — builds, no cycle:**
```
$ go build ./...        -> exit 0 (no errors)
$ go list ./...         -> exit 0   (import cycles make `go list` FAIL; it succeeded)
```

**PROOF 2 — db/store is parser-free:**
```
$ go list -deps ./db/store | grep parser
(no output)   grep exit 1   => barn/parser is NOT a transitive dependency of db/store
```

**PROOF 3 — bytecode has no back-edge (cycle impossible):**
```
$ go list -deps ./bytecode | grep -E 'barn/(builtins|vm|db/store)'
(no output)   grep exit 1
$ go list -f '{{range .Imports}}{{println .}}{{end}}' ./bytecode | grep barn/
barn/parser
barn/types
```

**PROOF 4 — concrete (non-interface) hot-path dispatch:**
```
vm/vm.go:34   Program *Program            // concrete *bytecode.Program (type alias), not an interface
vm/vm.go:363  op := OpCode(frame.Program.Code[frame.IP])   // direct struct-field + slice index
vm/vm.go:384  switch op { ... }           // byte switch on concrete OpCode
$ grep -rln 'bytecode.Registry|GetID' vm/ server/ (non-test) -> (none)
```
The only `Registry` interface call is `bytecode/compiler.go:1369` (`c.registry.GetID`), compile-time only.

**PROOF 5 — tests of every touched package pass:**
```
ok  barn/bytecode   (no test files; compiler tests live in vm and pass there)
ok  barn/db/store
ok  barn/vm
ok  barn/builtins
ok  barn/server
```
Pre-existing, unrelated failures only: `barn/conformance` (missing external `../cow_py/tests/conformance`
dir — fails identically on clean master) and `barn/db/format` `TestLoadMongooseSnapshot` (missing
`mongoose7_snapshot.db` file in the worktree). Neither touches the spike's code.

---

## 5. Verdict

**Design B — CLEAN (B clean-with-one-interface).**

A dedicated `barn/bytecode` package can own `VerbProgram` + `CompileVerb` + the bytecode `Program`
type + the cache entry point, be imported by vm/builtins/server with:
- NO import cycle (`bytecode` imports only `parser`+`types`; the only would-be cycle, the
  builtins Registry, is broken by a structurally-satisfied interface defined IN `bytecode`), and
- free, concrete hot-path access (execution dispatches on a concrete `*Program`/`OpCode byte`; the
  single interface call is compile-time, once per builtin-call site, never per instruction),

while `db/store` becomes parser-free (`go list -deps ./db/store | grep parser` is empty).

What this spike did NOT build (and does not need to, to answer the topology question): the
content-hash / `(objID,verbIndex,epoch)`-keyed cache itself and its invalidation wiring. The spike's
`CompileVerbBytecode` always parses+compiles. Adding the real keyed cache + epoch invalidation is the
follow-on implementation work; the scout report (§4, §5, §8) covers the key-stability/eviction risks.
Those are orthogonal to topology and equally apply to fallback Design A.

---

## 6. Commit
Commit hash recorded below (to be DISCARDED — do not merge).
