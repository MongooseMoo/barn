# Spec Draft: Verb Language Ownership

## Target Files

- `spec/vm.md`
- `spec/statements.md`
- `parser/AGENTS.md`

## Section to Modify: `spec/vm.md` Section 1, Compilation Pipeline

## Proposed Content

```markdown
## 1. Compilation Pipeline

```text
Original MOO Source -> Source Compiler -> Bytecode Program -> VM
                         | orchestrates
                         +-> MOO Parser -> Verb IR -> Bytecode Compiler
```

| Stage | Owner | Input | Output |
|-------|-------|-------|--------|
| Source persistence | `db/store`, `db/format` | Original MOO source lines | Original MOO source lines |
| Source compiler | `compiler` | Original MOO source plus builtin registry | Cached `bytecode.Program` or structured diagnostics |
| MOO parser | `parser`, called by `compiler` | Original MOO source | `verb.Program` semantic IR |
| Bytecode compiler | `bytecode`, called by `compiler` | `verb.Program` | `bytecode.Program` |
| Runtime | `task`, `scheduler`, `vm` | `bytecode.Program` | Execution result |

The artifacts in this pipeline are distinct:

- **Original source** is the exact persisted sequence of MOO source lines. It is
  returned by `verb_code()` and written to database verb-program sections.
- **Verb IR** (`verb.Program`) is the language-neutral executable meaning of a
  verb. It contains semantic statements, expressions, operators, literal
  payloads, and source locations. It does not contain runtime values or exact
  whitespace, comments, spelling, or redundant parentheses.
- **Compiled program** (`bytecode.Program`) contains VM instructions, constants,
  local-variable metadata, source-line mappings, and original source needed for
  runtime diagnostics and fork persistence.

Canonical MOO formatting deterministically renders a `verb.Program` as MOO
source. It preserves semantic meaning, precedence, and associativity; it does
not preserve the exact original text. Formatting never replaces the original
source used by `verb_code()` or database persistence.

Runtime tasks, scheduler state, and VM frames carry compiled bytecode programs.
They do not carry parser syntax trees or verb IR. Persisted queued-task source is
an IO artifact used to compile a bytecode program once during restoration; it
does not make task or scheduler runtime state an owner of syntax or IR.

`compiler` is the only callable source-to-bytecode boundary. The parser and
bytecode compiler are its internal stages; runtime callers do not compose those
stages independently.
```

## Section to Modify: `spec/vm.md` Section 12, Compilation

## Proposed Content

```markdown
## 12. Compilation

### 12.1 Source Compilation

The `compiler` package owns complete MOO source compilation:

1. Parse original MOO source through `parser` into `verb.Program`.
2. Preserve structured diagnostics with source locations.
3. Compile the verb IR through `bytecode`.
4. Attach original source to the resulting bytecode program.
5. Cache the immutable bytecode program by source content.

No runtime caller performs an independent subset of these steps.

### 12.2 Bytecode Compiler Structure

```go
type Compiler struct {
    program    *Program
    constants  map[ConstantKey]int
    variables  map[string]int
    loops      []LoopContext
    scopes     []Scope
}
```

The bytecode compiler accepts semantic nodes owned by `verb`. It does not
import or interpret MOO lexer tokens, concrete syntax, or parser-owned nodes.
`ConstantKey` represents semantic runtime-value equality and never source-text
spelling.

### 12.3 Expression Compilation

Expression compilation dispatches on semantic expression variants. Operators
are `verb` semantic operators rather than MOO token kinds. Literal payloads are
converted to Barn runtime values only at the bytecode boundary.

The implementation must exhaustively handle every sealed `verb.Expr` variant.
Representative cases include semantic literals, binary expressions, and
variable references; omitted cases in this specification are not invalid
expressions.

### 12.4 Statement Compilation

Statement compilation dispatches on semantic statement variants and emits
bytecode control flow. Statement nodes do not execute themselves.

The implementation must exhaustively handle every sealed `verb.Stmt` variant.
Representative cases include conditionals and the distinct semantic range-loop
and collection-loop forms; omitted cases in this specification are not invalid
statements.
```

## Section to Modify: `spec/vm.md` Section 13, VM State Serialization

## Proposed Content

Add this normative paragraph immediately after the opening sentence:

```markdown
Serialized and live runtime execution state refer to compiled bytecode programs,
instruction positions, values, and control-flow stacks. They contain no parser
syntax tree and no verb IR. A queued-task database record retains its persisted
`code` source artifact so restoration compiles that artifact exactly once before
recreating runtime state. Queued-task `code` is distinct from live or serialized
VM state and from the separately preserved original source of a database verb.
```

## Section to Modify: `spec/statements.md` Section 13, Implementation Boundary

## Proposed Content

Replace the complete current Section 13 with:

```markdown
## 13. Implementation Boundary

### 13.1 Semantic Statements

Language-level statement meaning is represented by statement variants owned by
the `verb` package. These nodes are compiler input: they do not contain VM
methods and do not execute themselves.

The MOO parser constructs these semantic statements directly. There is no
parser-owned executable AST, parser-to-IR adapter, or parallel old/new statement
path.

### 13.2 Compiler Control Flow

Loop labels, break and continue targets, exception handlers, and finally
regions are resolved by the bytecode compiler while lowering semantic
statements to bytecode. Compiler control-flow bookkeeping is not parser syntax
and is not retained as a semantic statement object in runtime task state.

### 13.3 Try/Finally Guarantee

The bytecode emitted for a try/finally statement must run the finally body on
normal completion, return, loop control transfer, or exception propagation, as
specified in Section 10. The VM enforces this guarantee through compiled
handler metadata and control flow; statement nodes do not recursively execute
try or finally bodies.
```

## Section to Modify: `parser/AGENTS.md`

## Proposed Content

Replace the complete file with:

```markdown
# Parser Package Rules

The `parser` package owns the MOO language frontend: grammar, tokens, parsing,
source diagnostics, and canonical MOO formatting.

The parser constructs `verb.Program` directly but does not own the semantic
node types. The `barn/verb` package is the language-neutral semantic owner; it
is not a Barn runtime package.

Hard boundaries:

- Do not import `barn/types` or any other Barn runtime package from `parser`.
- Do not construct runtime values in `parser`.
- Do not encode truthiness, map-key validity, VM behavior, database behavior,
  server behavior, or builtin behavior here.
- Keep MOO token spelling, keyword recognition, `ANY`, and error-name syntax in
  `parser`; construct their language-neutral semantic representation in
  `verb.Program`.
- Parse MOO literal spelling directly into semantic literal kinds and payloads
  owned by `verb`. Runtime conversion remains outside both `parser` and `verb`.
- Canonical formatting consumes `verb.Program` and emits deterministic MOO
  source. It does not promise exact whitespace, comments, spelling, or redundant
  parenthesis preservation and never replaces stored original source.
- Do not add compatibility adapters, senders, helpers, or wrapper APIs to
  preserve deleted parser-owned semantic APIs.

Deletion-first rule:

- When an old parser API exposes executable semantic or runtime concepts, move
  callers to the real owner and delete the old parser surface.
- A parser cleanup is not complete until one parser path constructs
  `verb.Program` directly, with no parser-owned semantic AST, AST-to-IR adapter,
  or parallel old/new path.
```

## Cross-References

- `spec/grammar.md` remains the concrete MOO syntax and token contract.
- `spec/database.md` Sections 6 and 7 remain the original-source and persisted
  queued-task IO contracts.
- `spec/README.md` already indexes all modified specifications; no index change
  is required.
- `plans/multilingual-verb-language-cleanup-plan.md` remains the execution
  control surface.

## Open Questions

None.
