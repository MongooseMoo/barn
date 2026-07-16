# Specification Discovery: Verb Language Ownership

## Scope and governing contract

Phase 1 is a specification-only ownership slice: it precedes implementation
movement and requires an independent commit. Its required subjects are the
source-to-VM pipeline, separation of original source from semantic IR,
task/scheduler exclusion from syntax and IR ownership, canonical formatting,
removal of tree-walker descriptions, and the parser package charter.
(`plans/multilingual-verb-language-cleanup-plan.md:144-162`)

The target owners are explicit: `parser` owns MOO grammar, tokens, parsing,
diagnostics, and canonical MOO formatting; `verb` owns language-neutral
executable meaning and source locations; `bytecode` owns IR-to-bytecode
compilation; and task, scheduler, and VM consume compiled bytecode without
carrying parser syntax or verb IR.
(`plans/multilingual-verb-language-cleanup-plan.md:59-70`)

## Exact sections that must change

### `spec/vm.md`

1. **Section 1, “Compilation Pipeline,” lines 9-20.** Replace the current
   `MOO Source -> Lexer -> Parser -> AST -> Compiler -> Bytecode -> VM` diagram
   and its `Parser -> AST nodes` / `Compiler <- AST` table rows with the Phase 1
   pipeline `MOO source -> MOO parser -> verb IR -> bytecode compiler -> VM`.
   (`spec/vm.md:9-20`; `plans/multilingual-verb-language-cleanup-plan.md:148-152`)

2. **Section 1, immediately following the pipeline table.** Add the normative
   ownership boundary: original stored source and semantic verb IR are distinct
   artifacts; canonical formatting produces MOO source from semantic IR without
   preserving exact source text; task and scheduler execution state carry
   compiled bytecode, not parser syntax or verb IR.
   (`plans/multilingual-verb-language-cleanup-plan.md:154-156`;
   `plans/multilingual-verb-language-cleanup-plan.md:92-93`;
   `plans/multilingual-verb-language-cleanup-plan.md:69-70`)

3. **Section 12, “Compilation,” lines 541-600.** Rewrite the compiler examples
   so their input is explicitly the language-neutral verb IR rather than the
   unqualified `Expr`, `Stmt`, `IfStmt`, and `ForStmt` forms currently presented
   as AST input. This section must remain consistent with the revised Section 1
   pipeline and with `bytecode` owning IR-to-bytecode compilation.
   (`spec/vm.md:541-600`;
   `plans/multilingual-verb-language-cleanup-plan.md:63-65`)

4. **Section 13, “VM State Serialization,” lines 605-625.** Retain its
   bytecode-oriented `ProgramID`/instruction-pointer model and add the Phase 1
   constraint that serialized runtime execution state contains no parser syntax
   or verb IR. The current section already identifies a program and instruction
   position instead of an AST node.
   (`spec/vm.md:605-625`;
   `plans/multilingual-verb-language-cleanup-plan.md:69-70`)

### `spec/statements.md`

1. **Section 13.1, “Statement Interface,” lines 684-692.** Delete the
   tree-walker `Execute(vm *VM) error` method. Executable statements are semantic
   compiler input in the target architecture, not runtime-executing objects.
   (`spec/statements.md:684-692`;
   `plans/multilingual-verb-language-cleanup-plan.md:157-158`;
   `plans/multilingual-verb-language-cleanup-plan.md:63-65`)

2. **Section 13.3, “Try/Finally Guarantee,” lines 704-716.** Delete or rewrite
   the `TryFinallyStmt.Execute` example because it describes direct recursive
   statement execution through `t.Finally.Execute` and `t.Try.Execute`. Preserve
   the language-level finally guarantee in Section 10; remove only the obsolete
   implementation mechanism.
   (`spec/statements.md:558-610`;
   `spec/statements.md:704-716`;
   `plans/multilingual-verb-language-cleanup-plan.md:157-158`)

3. **Section 13.2, “Loop Labels,” lines 694-702.** Label the shown
   `LoopContext` as compiler/bytecode control-flow state, not parser syntax or a
   runtime statement object, or remove the implementation note. The target
   boundary assigns executable statement meaning to `verb` and lowering state
   to `bytecode`.
   (`spec/statements.md:694-702`;
   `plans/multilingual-verb-language-cleanup-plan.md:63-65`)

### `parser/AGENTS.md`

1. **Package ownership statement, line 3.** Expand “owns MOO syntax only” to
   say that the parser owns MOO grammar, tokens, parsing, diagnostics, and
   canonical formatting, and that it constructs `verb.Program` without owning
   the semantic node types.
   (`parser/AGENTS.md:1-4`;
   `plans/multilingual-verb-language-cleanup-plan.md:61-63`;
   `plans/multilingual-verb-language-cleanup-plan.md:159-160`)

2. **Runtime-import boundary, lines 5-8.** Preserve the prohibition on Barn
   runtime packages and runtime values, while explicitly distinguishing the
   language-neutral `barn/verb` semantic package from a runtime package. The
   target `verb` literal representation is also prohibited from importing Barn
   runtime values.
   (`parser/AGENTS.md:5-8`;
   `plans/multilingual-verb-language-cleanup-plan.md:159-160`;
   `plans/multilingual-verb-language-cleanup-plan.md:169-175`)

3. **Literal rule, line 10.** Replace “parser-owned syntax nodes” with a rule
   that MOO literal spelling is parsed directly into language-neutral semantic
   literal kinds and payloads owned by `verb`; runtime conversion remains
   outside `parser` and `verb`.
   (`parser/AGENTS.md:9-10`;
   `plans/multilingual-verb-language-cleanup-plan.md:169-177`)

4. **Deletion-first completion rule, lines 13-15.** Replace the final
   “syntax-only path” wording with the exact convergence condition: one parser
   path constructs `verb.Program` directly, with no parser-owned semantic AST,
   adapter, or parallel old/new path.
   (`parser/AGENTS.md:13-15`;
   `plans/multilingual-verb-language-cleanup-plan.md:166-180`)

## Cross-references that must remain consistent

- `spec/grammar.md` remains the MOO concrete-syntax contract: it defines the
  EBNF program as statements and separately documents lexical elements and
  operator tokens. The Phase 1 ownership text must not move grammar or token
  ownership out of `parser`.
  (`spec/grammar.md:1-7`; `spec/grammar.md:26-44`;
  `spec/grammar.md:341-381`;
  `plans/multilingual-verb-language-cleanup-plan.md:61-62`)

- `spec/database.md` Section 6 remains the persistence contract for verb source:
  a verb program is stored as lines terminated by `.`. The revised VM spec must
  identify these lines as the original persisted source artifact, not semantic
  IR or bytecode.
  (`spec/database.md:173-185`;
  `plans/multilingual-verb-language-cleanup-plan.md:40-41`;
  `plans/multilingual-verb-language-cleanup-plan.md:81-82`)

- `spec/database.md` Section 7 remains the persistence contract for queued and
  suspended tasks: queued tasks include a `code...` block, while suspended VM
  state contains activations. The Phase 1 task/scheduler rule must distinguish
  persisted source needed for restoration from executable runtime state, which
  consumes compiled programs without carrying parser syntax or verb IR.
  (`spec/database.md:189-222`;
  `plans/multilingual-verb-language-cleanup-plan.md:69-70`;
  `plans/multilingual-verb-language-cleanup-plan.md:273-278`)

- `spec/README.md` already identifies `grammar.md`, `statements.md`, and `vm.md`
  as the grammar, control-flow, and VM-architecture specifications and declares
  the repository spec-first. No index change is required for Phase 1 because no
  new specification document is introduced.
  (`spec/README.md:5-18`; `spec/README.md:42-47`;
  `plans/multilingual-verb-language-cleanup-plan.md:144-162`)

## Current statements contradicted by the target architecture

- `spec/vm.md` currently names AST nodes as parser output and compiler input;
  the target names `verb.Program`/verb IR as parser output and bytecode-compiler
  input.
  (`spec/vm.md:12-20`;
  `plans/multilingual-verb-language-cleanup-plan.md:148-152`)

- `spec/vm.md` Section 12 currently presents compilation over unqualified
  syntax-shaped `Expr` and `Stmt` types; the target assigns semantic node
  ownership to `verb`, not `parser` or VM runtime state.
  (`spec/vm.md:555-600`;
  `plans/multilingual-verb-language-cleanup-plan.md:63-70`)

- `spec/statements.md` currently requires statements to execute themselves and
  demonstrates recursive tree-walker execution for try/finally; Phase 1
  explicitly orders those descriptions removed.
  (`spec/statements.md:684-692`; `spec/statements.md:704-716`;
  `plans/multilingual-verb-language-cleanup-plan.md:157-158`)

- `parser/AGENTS.md` currently calls literals parser-owned syntax nodes and says
  runtime packages lower them; the target parser constructs semantic literals
  owned by `verb`, whose payloads do not import runtime values.
  (`parser/AGENTS.md:9-10`;
  `plans/multilingual-verb-language-cleanup-plan.md:169-177`)

- `parser/AGENTS.md` currently defines successful cleanup as coexistence ending
  in a “syntax-only path”; the target successful path parses MOO directly into
  `verb.Program` and forbids an AST-to-IR adapter or two ASTs.
  (`parser/AGENTS.md:13-15`;
  `plans/multilingual-verb-language-cleanup-plan.md:166-180`)

## Terms that require definitions in the specification slice

- **Original source:** the exact persisted list of MOO source lines used by
  `verb_code()` and database round trips; it is not canonical formatter output.
  (`plans/multilingual-verb-language-cleanup-plan.md:81-82`;
  `plans/multilingual-verb-language-cleanup-plan.md:326-328`)

- **MOO syntax:** concrete grammar, tokens, keyword/error-name spelling, and
  source-level notation owned by `parser`.
  (`plans/multilingual-verb-language-cleanup-plan.md:61-62`;
  `parser/AGENTS.md:3-10`)

- **Verb IR / semantic IR / `verb.Program`:** the language-neutral executable
  meaning of one verb, including semantic statements, expressions, operators,
  literal payloads, and source locations, but excluding Barn runtime values and
  exact whitespace/comments/redundant parentheses.
  (`plans/multilingual-verb-language-cleanup-plan.md:63-63`;
  `plans/multilingual-verb-language-cleanup-plan.md:92-93`;
  `plans/multilingual-verb-language-cleanup-plan.md:169-175`)

- **Canonical MOO formatting:** deterministic MOO source generation from
  semantic IR; semantic round-trip equality excludes source locations and does
  not promise preservation of whitespace, comments, spelling, or redundant
  parentheses.
  (`plans/multilingual-verb-language-cleanup-plan.md:306-318`)

- **Compiled program:** a `bytecode.Program` consumed by task, scheduler, and VM
  runtime execution; it is distinct from original source and verb IR.
  (`plans/multilingual-verb-language-cleanup-plan.md:63-70`;
  `spec/vm.md:24-35`)

- **Own versus carry:** ownership identifies the package that defines and
  interprets an artifact. Runtime task execution carries compiled bytecode;
  database task persistence retains source needed for one-time restoration
  compilation without making task/scheduler the owner of parser syntax or verb
  IR.
  (`plans/multilingual-verb-language-cleanup-plan.md:59-70`;
  `plans/multilingual-verb-language-cleanup-plan.md:273-278`;
  `spec/database.md:189-222`)

## Open questions

None. The plan resolves the Phase 1 ownership boundary, formatting/source
distinction, runtime-state constraint, parser charter, and queued-task
restoration relationship explicitly.
(`plans/multilingual-verb-language-cleanup-plan.md:59-70`;
`plans/multilingual-verb-language-cleanup-plan.md:144-160`;
`plans/multilingual-verb-language-cleanup-plan.md:273-278`)
