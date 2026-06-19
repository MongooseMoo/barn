# SPIKE: package topology for a typed WorldStore seam with no import cycle

Branch: `spike/world-seam` off HEAD `2f39aadc9212d06be0bb4ead912ed01c8cdfd63b`.

## TL;DR

**Chosen topology: Candidate A (consumer-side typing via a concrete-returning helper).**
It is the ONLY candidate that satisfies both hard constraints (no cycle + free reads).
`ctx.Store` stays `interface{}` in `types`; the typing happens at the point of use in
`builtins` (which already imports `db/store`) via `storeFromCtx(ctx) (*dbstore.Store, bool)`.
The helper returns the **concrete** type, so reads stay static dispatch, and the compiler
**inlines the helper at all 11 converted call sites** (proven with `-gcflags=-m`).

- `go build ./...` -> PASS (exit 0).
- `go test ./db/... ./builtins/...` -> PASS (all ok).
- Inlining -> PROVEN (`can inline storeFromCtx`, `inlining call to storeFromCtx` x11).
- Goal 3 (typed field on `types.TaskContext.Store`) is **unreachable without violating a
  hard constraint** — the field must stay `interface{}`. Per the prompt, constraint 2 wins.

## 1. Confirmed cycle facts (file:line)

- `types/context.go:53-54`: `Store interface{}` with comment
  `// Import cycle prevention: This is stored as interface{} (should be *store.Store)`. CONFIRMED.
- `db/store/object.go:1-5`: `package store` imports `barn/parser` and `barn/types`. CONFIRMED.
- `db/store/object.go:69`: `VerbProgram.Statements []parser.Stmt`. CONFIRMED.
- `types` imports NOTHING from barn (pure leaf) — `go list` shows no barn imports. CONFIRMED.

### CORRECTION to a prompt-stated fact

The prompt asserts "**parser imports `barn/types`**" and that moving the world model below
`types` "recreates the cycle, because that package needs `types` (and `parser`)".

**The parser->types edge does not exist.**
- `go list -f '{{.Imports}}' barn/parser` returns no barn imports.
- `grep "types\." parser/*.go` -> 0 matches; `grep '"barn/' parser/*.go` -> 0 matches.
- Only `parser/AGENTS.md` (docs) mentions types.

`parser` is a pure leaf with zero barn dependencies. This does not change the chosen
topology, but it means the cycle risk for Candidate C/D comes ONLY from the
`db/store -> types` edge plus the `VerbProgram.Statements []parser.Stmt` field — NOT from
any parser->types relationship.

## 2. Import graph (via `go list`, exact)

BEFORE (and AFTER — unchanged; Candidate A adds no new edges):

```
barn/types     -> (leaf, no barn imports)
barn/parser    -> (leaf, no barn imports)
barn/db/store  -> barn/parser, barn/types
barn/builtins  -> barn/db/store, barn/parser, barn/task, barn/trace, barn/types
barn/vm        -> barn/db/store, ... (also imports db/store)
```

The decisive fact: **`builtins` (and `vm`) ALREADY import `db/store`.** Consumer-side
typing therefore needs no new import and cannot introduce a cycle.

## 3. Candidate evaluation

### A. Helper / consumer-side typing — CHOSEN

Keep `ctx.Store interface{}`. Add `storeFromCtx(ctx) (*dbstore.Store, bool)` in `builtins`
returning the concrete type. Convert `ctx.Store.(*dbstore.Store)` sites to it.

- Cycle: none (no new edges; builtins already imports db/store).
- Reads: FREE. Helper inlines; downstream calls are concrete static dispatch
  (`(*store.Store).ObjectExists(store, objID)`), not itab dispatch.
- Cost: the `ctx.Store` field itself stays `interface{}`; typing is at point of use.
  This is a single, greppable seam per consumer package.

### B. Interface handles in `types` — REJECTED (violates hard constraint 2)

Define `WorldStore`/`ObjectHandle`/`VerbHandle` as interfaces in `types`; make
`ctx.Store WorldStore`. Removes the cycle, BUT every store read becomes a Go interface
method call = dynamic dispatch through the itab. Interface method calls in Go are never
inlined across the interface boundary and cannot be devirtualized here (the concrete type
is hidden behind the interface by construction). This directly violates hard constraint 2
("no per-access dynamic dispatch"). Not implemented; rejected on Go-semantics grounds.

### C. Collapse world model into `types` — REJECTED (out of scope, and load-bearing edge)

Move `Object`/`Verb`/`Store` into `types`. Requires breaking the `db/store -> parser` edge
(`VerbProgram.Statements []parser.Stmt`) by either moving the parser AST into `types` or
abstracting `Statements`. Even though parser is a leaf (so it COULD move under types
without a cycle), this collapses the whole storage layer into the value/type package — a
massive blast radius (every `db/store` symbol and ~all consumers) for a spike. Assessed
only, per prompt. Feasible in principle (no fundamental cycle, since parser is a leaf), but
not justified and not implemented.

### D. Interface in a leaf package + type-asserting helper — considered, folds into A

A `WorldStore` interface in a new leaf package (no concrete `Object` reference) that `types`
imports would let `ctx.Store` be a typed interface — but using it for reads is still
dynamic dispatch (same as B). The only way to get free reads from such a package is to
type-assert back to the concrete `*dbstore.Store` at the point of use — which is exactly
Candidate A. So D provides no advantage over A for the free-reads constraint.

### Why goals 2 and 3 conflict (the core truth)

To make `types.TaskContext.Store` a concrete `*dbstore.Store` you must import `db/store`
into `types`; but `db/store` imports `types` -> cycle (constraint 1). To make it a typed
interface in `types` avoids the cycle but forces dynamic dispatch (constraint 2). **There
is no topology where the FIELD itself is both typed AND read with free reads.** Free reads
require the concrete type at the read site, which can only live in a package that imports
`db/store` — i.e. a consumer, not `types`. Constraint 2 wins: the field stays `interface{}`
and typing moves to the consumer seam.

## 4. Implementation (proof it compiles end-to-end)

New file `builtins/worldstore.go`:

```go
func storeFromCtx(ctx *types.TaskContext) (*dbstore.Store, bool) {
	s, ok := ctx.Store.(*dbstore.Store)
	return s, ok
}
```

Converted all 11 `ctx.Store.(*dbstore.Store)` sites in `builtins/verbs.go` to
`storeFromCtx(ctx)` (lines 80,137,173,242,307,359,514,551,619,681,855).

## 5. Evidence (exact commands + output)

### Build

```
$ go build ./...
(no output; exit 0)
```

### Tests

```
$ go test ./db/... ./builtins/...
ok  	barn/db/format	(cached)
ok  	barn/db/store	(cached)
ok  	barn/builtins	0.528s
(exit 0)
```

Conformance harness: NOT run in this spike. Reason: the spike's claim is a compile-time
topology + codegen property (no cycle, inlined reads), fully proven by `go build`,
`go test`, and `-gcflags=-m`. The conformance harness needs a managed server + Test.db and
would not add signal about the topology question. Stated explicitly per task 3.

### Inlining (`go build -gcflags=-m ./builtins/`)

```
builtins/worldstore.go:30:6: can inline storeFromCtx
builtins/verbs.go:80:27: inlining call to storeFromCtx
builtins/verbs.go:137:27: inlining call to storeFromCtx
builtins/verbs.go:173:27: inlining call to storeFromCtx
builtins/verbs.go:242:27: inlining call to storeFromCtx
builtins/verbs.go:307:27: inlining call to storeFromCtx
builtins/verbs.go:359:27: inlining call to storeFromCtx
builtins/verbs.go:514:27: inlining call to storeFromCtx
builtins/verbs.go:551:27: inlining call to storeFromCtx
builtins/verbs.go:619:27: inlining call to storeFromCtx
builtins/verbs.go:681:27: inlining call to storeFromCtx
builtins/verbs.go:855:27: inlining call to storeFromCtx
```

The helper inlines at EVERY converted site. The downstream read renders as a
concrete-receiver static call, confirmed by `-gcflags='-m -m'`:

```
from (*store.Store).ObjectExists(store, objID) (call parameter) at builtins/properties.go:...
```

i.e. `(*store.Store).ObjectExists(...)` — a direct method call on the concrete pointer, NOT
an interface dispatch. Free reads confirmed.

## 6. Full-rollout shape

Total remaining `Store.(*dbstore.Store)` cast expressions after this spike: **43**
(excludes the helper's own comment/signature lines in worldstore.go).

Per-file counts (sites still using the raw assertion):

| File | Sites |
|------|-------|
| builtins/objects_hierarchy.go | 15 |
| builtins/properties.go | 7 |
| builtins/objects.go | 4 |
| builtins/system.go | 3 |
| builtins/objects_players.go | 3 |
| builtins/objects_misc.go | 3 |
| builtins/objects_movement.go | 2 |
| builtins/tasks.go | 2 |
| builtins/crypto.go | 1 |
| builtins/registry.go | 1 |
| builtins/signatures.go | 1 |
| vm/registry.go | 1 |

(verbs.go's 11 are already converted.)

Rollout work:
- `builtins`: a mechanical `sed` of `X.Store.(*dbstore.Store)` -> `storeFromCtx(X)` per file,
  same as verbs.go. ~42 sites across the listed builtins files. Each is the identical
  two-line idiom, so conversion is low-risk and greppable.
- `vm/registry.go`: 1 site. `vm` also imports `db/store`, so it needs its OWN
  `storeFromCtx` (an unexported copy in `vm`), OR `builtins.storeFromCtx` could be exported
  as `builtins.StoreFromCtx` and reused — but `vm` importing `builtins` may itself be a new
  edge; check direction before reusing. Simplest: a private copy in `vm`.
- Field encapsulation: `ctx.Store` itself CANNOT be retyped (see section 3). The field stays
  `interface{}`. "Full encapsulation" here means: route 100% of reads through the seam helper
  so the raw assertion appears in exactly one place per consumer package, and the comment at
  `types/context.go:53-54` can be updated to point at the seam instead of promising
  `*store.Store`.

## 7. Landmines / surprises

- **Biggest landmine:** the prompt's premise that goal 3 (typed `ctx.Store` field) is
  achievable is FALSE under the hard constraints. Any attempt to type the field either
  reintroduces the `db/store <-> types` cycle (concrete) or forces dynamic dispatch
  (interface). The honest result is exactly the prompt's allowed fallback: **only Candidate A
  (helper) gives free reads.** The field stays `interface{}`; the win is a single typed seam
  per consumer, not a typed field.
- The prompt's "parser imports types" fact is wrong (parser is a leaf). Documented above.
  This does not change the conclusion.
- `vm` is a second consumer of the seam (1 site) and is NOT covered by a `builtins` helper;
  it needs its own. Cross-package reuse risks a new import edge.
- The `//go:inline` pragma I added is cosmetic/documentary — Go has no such directive; the
  inliner decided on cost alone (and did inline). It is harmless (unknown pragmas are
  ignored) but should be removed or replaced with a plain comment in a real rollout to avoid
  implying a guarantee.

## 8. Commit

Commit hash on `spike/world-seam`: see git log (recorded after commit of this report +
worldstore.go + verbs.go only).
