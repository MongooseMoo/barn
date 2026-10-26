# Execution deletion exploration

## Goal

Read-only exploration requested by Q on 2026-10-25: find removals of code and complexity without removing functionality or slowing Barn. This subtask covers compiler, parser, bytecode, VM, engine/task execution. No implementation authorization; no source edits, tests, builds, server launches, commits or publication performed. This note is the only intended write.

## State

Static source survey completed. This area does **not** currently support a 5,000-line deletion claim. Concrete smaller opportunities exist; larger ideas remain hypotheses. Counts below are physical production Go lines, including their associated comments/blank lines, not measured patch results. No generated-code, test, or documentation removal is counted toward production savings.

Read parser/AGENTS.md and docs/elle-isolation.md. Preserve sealed semantic ownership in verb, parser-only language syntax, persisted bytecode compatibility, explicit hot paths, and the existing isolation-check obligations.

Largest production files surveyed: vm/vm.go 1611 lines; engine/task_runtime.go 1216; task/task.go 1128; parser/parser.go 956; compiler/lower_assignments.go 861; parser/parser_stmt.go 780; compiler/lower_expressions.go 739. Size alone was not treated as a removal finding.

## Candidates grounded in source

### E1. Delete compiler scope bookkeeping that has no consumer

Evidence:

- compiler/lower_locals.go:8-12 defines scope with Variables and Parent.
- compiler/lower_locals.go:22-37 allocates a scope map, links Parent, and pushes/pops it.
- compiler/lower_locals.go:73-75 writes each declared local into the scope map.
- compiler/lower_locals.go:42-47 resolves existing declarations through c.variables; :106-109 resolves reads through the same c.variables map.
- compiler/lower.go:31,53,69,101 stores/allocates/starts/ends scopes.
- `rg -n 'beginScope|endScope|\.scopes|\.Parent|\.Variables' compiler --glob '*.go'` showed only those bookkeeping uses plus compiler/builtin_slots_test.go:12 calling beginScope. No lookup reads Variables or Parent.

Smallest change: delete scope type, scope field/allocation, begin/end methods, declaration-side map writes, two compileProgram calls, and the unnecessary test setup call. Keep c.variables and internalVariables unchanged.

Estimate: remove 35-40 production lines, add 0, net 35-40. One test setup line removed, no test coverage removed. Eliminates an allocation and map writes at compilation; VM bytecode and execution need not change. Runtime/performance equivalence remains untested. Existing compiler identifier/slot coverage and PR CI are the appropriate verification surface.

### E2. One parser for the two verb-argument lists

parser/parser.go:200-219 ($name call) and :499-518 (object:verb call) contain matching argument loops, comma/error handling, and close-paren consumption. Real entrypoints are the TOKEN_DOLLAR prefix branch and TOKEN_COLON infix branch of ParseExpression. Both build verb.VerbCallExpr.

Smallest change: a parser-local verb-argument function invoked after the opening parenthesis is consumed. Preserve identical error strings and token advancement. Do not generalize it to builtin arguments: :380-410 uses a different loop and diagnostic behavior plus registry callbacks.

Estimate: remove 42-46, add 27-31 (helper and caller error checks), net 11-19 production lines. No tests removed. This is modest but removes a real second owner. Compiler/parser CPU effect of a function call is unknown; no claim of improvement.

### E3. Share the scatter-binding emission sequence, keep the two traversal orders

compiler/lower_assignments.go:636-861 lowers destructuring. The suffix loop :771-796 and prefix loop :798-827 repeat optional-value branching, take emission, default emission, and jump patching. They intentionally differ in traversal order, cursor, direction and required-reserve count. Existing helpers :740 and :759 already own optional condition/default emission.

Smallest change: extract only one compile-time binding emission helper accepting binding, cursor, direction and reserve; retain existing suffix and prefix loops and their countRequired calls. No new runtime opcode, protocol, AST, intermediate representation, or iterator.

Estimate: remove 45-55, add 25-35, net 10-30 production lines. No runtime instruction changes intended; establish byte-for-byte compiled output for existing real scatter cases before claiming identical ticks and speed. No tests removed. This is not a 200-line opportunity despite the large enclosing function.

### E4. Remove duplicate identifier resolution branches

compiler/lower_expressions.go:46-75 has a resolved-variable path, a builtin-constant path and a fallback, all emitting GET_VAR. declareVariable at compiler/lower_locals.go:42-47 already returns the existing slot; :59-64 performs builtin-name canonicalization for new variables. Thus compileIdentifier can call declareVariable once and emit GET_VAR once. resolveVariable then has no other production caller according to `rg -n 'resolveVariable' compiler`.

Estimate: remove 30-34, add 5-8, net 22-29 production lines, zero test deletion. Retain builtinConstants itself: it still supplies type-constant slot initialization metadata in declareVariable. Removes repeated map lookup for new identifiers; no bytecode change intended. Confirm existing identifier and builtin slot tests through PR CI.

### E7. Compile argument lists once for pass, builtins and verb calls

Parent identified this missed opportunity; all three implementations were subsequently read directly:

- compiler/lower_expressions.go:263-297 handles pass arguments and emits OP_PASS.
- :302-335 handles ordinary builtin arguments and emits the builtin call.
- :479-511 handles verb arguments; :528-532 selects the same argument-count marker.

Each scans for a top-level SpliceExpr, rejects more than 254 fixed arguments, and either pushes each argument directly in source order or emits MAKE_LIST 0 followed by LIST_EXTEND for splice operands and LIST_APPEND for normal operands. A compiler-local compileCallArguments helper can own that emission and return the byte argument count (0xFF for splice). Existing callers retain opcode selection. This removes three maintained copies of the argument lowering algorithm with no new runtime machinery.

Smallest conservative shape: retain the existing hasSplice and overflow checks in each caller, because builtin/pass use `too many arguments (max 254)` while verbs use `too many verb arguments (max 254)`. Pass the known splice mode into the shared helper; moving that scan into the helper is optional, not necessary. Return errors from compileNode unchanged. Do not pass an emitter callback or introduce a new semantic argument representation.

Ordering constraints from source: compileBuiltinCall resolves registry membership at :251-257 before argument compilation. compileVerbCall compiles the receiver at :462, evaluates/stores the dynamic verb name at :469-476, compiles arguments, then reloads the name at :513-516 before emitting the call. The helper must be called exactly where the existing argument loop is; it must not evaluate the receiver/name or move overflow checks ahead of those expressions. Existing no-static/no-dynamic-name error at :525 remains after argument compilation.

Estimate: remove 85-100 production lines, add 45-50 (helper plus three checked invocations and marker writes), net 40-55. No tests/generated/docs removed. This does not overlap E1-E4. Preservation target is identical emitted bytecode, local allocation and line metadata for fixed/spliced pass, builtin and static/dynamic verb calls. Runtime cost should then be unchanged, but no bytecode equivalence or compile-time performance was measured. Existing caller, splice, argument-overflow and dynamic-name evaluation coverage must establish the claim through the authorized verification workflow.

## Structural leads needing design validation

### E5. Keep exception operand decoding in one existing bytecode owner

Repeated layout decoding appears in:

- bytecode/opcodes.go:365-384, instructionOperandCount variable clause lengths.
- bytecode/verify.go:95-120, variableOperandsTruncated repeats count traversal with bounds checks.
- bytecode/verify.go:250-277, verifyExcept parses optional local and absolute handler address.
- bytecode/compact_locals.go:65-81, walks clauses to rewrite local operands.
- bytecode/program.go:251-281, walks clauses to rebase absolute handler addresses.

The shared facts are code-count byte, code payload, optional local width (1 or 2 bytes), and address width (2 or 4 bytes). Existing decodedInstruction already owns opcode and operand slice (verify.go:65-69). A bounded internal clause iterator could expose slices/offsets into existing code to the non-runtime walkers. This would remove multiple handwritten descriptions of the same persisted format. Keep numeric opcode identities and serialized bytes unchanged. Do not move VM execution through allocated decoded instructions: vm/control.go:178 directly executes the encoding and its hot-path cost matters.

Tentative envelope: replace 110-145 duplicated layout lines with 60-90 shared traversal/call-site lines, net 20-85 production lines. This is a hypothesis, not an accepted design or measured patch. Consumer error behavior differs (length count, explicit rejection, tolerant loop breaks), so simply replacing all loops is not proved equivalent. Existing verify/disassemble/fork/compact-local tests and persisted task roundtrips are necessary. No tests should be deleted. Decode allocation and compile/fork performance require measurement.

### E6. Share task setup, not task lifecycles

engine/task_factory.go:116-138 and :184-203 initialize real server-verb and login-hook tasks with foreground budgets, task/context owner, wizard flag, verb identity, args, and fork creator. These are actual production paths, not dead alternatives. Admission ownership, error wrapping, no-code handling, callbacks, flushing and completion differ at :89-151 versus :163-220.

Only a shared initialization function accepting the already-resolved verb/program and call context is plausible. Do not merge whole entrypoints or add a broad options/protocol layer. Estimate remove 40-45, add 25-35, net 5-20 production lines. Another modest ownership cleanup. Need caller/lifecycle review and existing task tests; any retry/admission change invokes the pinned Elle workflow. No evidence justifies changing those lifecycles for a line target.

## Rejected tempting cuts

- VM fast path versus generic Execute: vm/vm.go:465 fastIntBinary and :687 executeLoop are not dead duplicates of :1086 Execute. Removing them would discard specialized execution. No performance proof supports it.
- Arithmetic flattening: vm/op_arith.go:42-114 explicitly checks int/int and float/float before strings/lists and promotion; :75-78 documents the StrAppend frontier requirement. A generic numeric operator callback/switch would add dispatch on hot paths and could change coercion/error precedence. Do not count the 409-line file as removable.
- Frame value visitors: vm/value_roots.go:17 full inventory and :61 finalizable fast path differ intentionally. The latter excludes constants and filters MayHoldFinalizable before callbacks. Consolidation can slow every frame pop; not a safe deletion candidate from source alone.
- Legacy opcodes/dynamic-name markers: bytecode/opcodes.go:144-147 and vm/op_property.go:15-20 preserve persisted suspended tasks. Absence of current compiler emission is insufficient deletion evidence. Preserve compatibility.
- Replace compiler internal-local allocation with a prepass to avoid CompactInternalLocals: adds another semantic traversal and risks changing declaration order/builtin slots. The 87-line compactor is not a free cut.
- task/task.go getters/setters: many own mutexes, callback ordering and task-state transitions. Line count is not proof of redundant state or removable locks.
- cloneActivationFrames in task/snapshot.go:228 and engine/task_runtime.go:1069 are genuinely identical 11-line copy routines, but exporting a new shared API saves little and touches persistence/retry ownership. Not a substantive target.
- executionContextOwner at engine/runtime.go:314 is only called by tests, but tests use it to assert GC execution provenance. Moving it into _test.go saves four production lines, not functionality/complexity, so excluded from the useful savings tally.
- GetPropNamed and executeStringAppend are tiny forwarding wrappers; direct routing preserves opcode support but saves only a handful of lines. Excluded as noise.

## Searches and limitations

A repository-wide token frequency scan of every Go file found no production function declared in these six directories with its name occurring only once. Excluding test files found only executionContextOwner. This is a crude negative heuristic, not a reachability proof: same-name methods and recursive-only functions can evade it, interfaces/reflection/public use can invalidate naive deletion proposals.

One initial regex-per-function scan was stopped because it repeatedly scanned all source; it was replaced with a single token Counter. Two read probes named nonexistent files (compiler/lower_variables.go and vm/op_logical.go); subsequent file discovery resolved actual compiler/lower_locals.go and no claims rely on either failed read. Those errors were read-only. All evidence above comes from successfully read current source.

No conformance, race, isolation, timing or allocation results exist for these proposals. Static evidence does not establish the requested no-break/no-slow condition. No paper, historical note, or unverified agent claim was used to establish behavior.

## Decisions and why

Prefer deleting the unused compiler scope representation first: it removes an owner and allocations with no semantic reader. Keep substantive bytecode decoding consolidation as a separate design lead; its savings are small enough that a new framework would defeat the purpose. Do not inflate totals by removing tests, comments alone, generated files, compatibility, or optimized execution.

## Next step

Parent should verify these source claims, combine with other area explorations, and decide whether a larger architectural deletion elsewhere reaches the goal. If implementation is later authorized, start with E1, E4 and E7 (approximately 97-124 production lines), then judge E5 by the actual smallest patch. The figures here must not be presented as a 5,000-line plan.
