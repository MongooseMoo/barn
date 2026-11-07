# test/fuzz-structured-source (plan item C6)

## Goal

Plan item C6 of the grammar-aware fuzzing plan: a structured MOO source
generator for the fuzz pipeline. Details filled in after reading the plan and
design (see Decisions).

## State

Read: the C6 brief, agent-notes/grammar-aware-fuzzing.md (design, points
1-6), common rules, fuzz-pipeline-expansion.md, AGENTS.md, ir/ir.go,
ir/validate.go, parser/format.go, parser/parser.go (ParseExpression, index
operands, assignment lowering, literals, lists, maps), parser/parser_stmt.go
(loops, fork), parser/diagnostics.go, compiler/compiler.go, compiler lowering
entry points, internal/fuzzseed/seeds.go, the three raw targets and
.github/workflows/fuzz.yml.

## Decisions and why

- Scope is design points 1-6 only: no parser cleanup, no grammar export.
- Generator constraints taken from the code, so intended-valid programs are
  accepted:
  - `^`/`$` only inside index/range brackets (parser.go:158-163, :225-231
    report outside indexDepth).
  - break/continue only inside a loop of the current task, labels must name an
    enclosing loop; a fork body resets the loop stack (parser_stmt.go:351-353,
    diagnostics.go checkLoopExit).
  - Splice only as a list element, call argument or catch/except code: the
    parser accepts `@` anywhere (parser.go:265-277), and the compiler lowers a
    stray one to OP_SPLICE (lower_expressions.go:516), so a stray splice is not
    an intended-valid program.
  - Assignment targets: variable, property, index/range over
    variable/property/index (parser.go:613-659); destructuring needs a
    nonempty binding list with at most one rest.
  - Builtin calls only to names the registry has (compiler.go:132-136 rejects
    others); constants <= 255, locals <= 256, args <= 254
    (lower_emit.go:114, lower_locals.go:29, lower_expressions.go:244).
- Invariants, one copy each: the parser structured target feeds the generated
  IR to the existing assertDirectProgramFormatRoundTrip
  (parser/operator_roundtrip_test.go:37), which already fails on format or
  parse errors. Compiler and VM raw bodies move into one helper each that both
  targets call; the structured target fails where the raw one returns.

## Surprises

- The parser never builds ir.LiteralBool (parser.go parseLiteralExpr has no
  bool case; `true`/`false` lex as identifiers), so the formatter's `true`
  reparses as an identifier. The generator does not emit LiteralBool.
- reflect.DeepEqual in the round trip separates nil from empty slices: the
  parser leaves an empty list/map nil but an empty call argument list non-nil
  (parser.go:190, :373, :492). The generator follows that.
- `except` after an `ANY` arm is a parse error (parser_stmt.go:525-526); the
  generator stops adding arms after ANY.
- Finding 1 (real Barn bug, found by FuzzStructuredParseFormatRoundTrip in its
  first second, saved input a06e2c2e6259249d): the formatter spells
  PropertyExpr{IndexBoundary first} as `^.name`, and the lexer reads `^.` as
  bitwise XOR (lexer.go:208-215), so the program no longer parses.
  Producer: set_verb_code stores FormatMOO output (builtins/verbs.go:622-627).
  Reproduced with bin/moo: set_verb_code(#0, "t", {"return {1}[(^).name];"})
  returns {} and verb_code gives {"return {1}[^.name];"}; set_verb_code of
  that text returns {"Line 1:  syntax error"}. Toast has the same defect in
  its own unparser (unparse.cc:623-635 prints `^.name` because EXPR_FIRST
  outranks EXPR_PROP; parser.y:1050-1051 lexes `^.` as tBITXOR). Barn's
  formatter already departs from Toast's text for the integer case
  (format.go: `(1).name`, Toast `1 .name`) to keep the round trip.
  Q 2026-11-07 chose "Split it out": the formatter change, its test line and
  saved input a06e2c2e6259249d are NOT in this PR. Printing `(^).name` keeps
  the Barn verb working but makes verb_code() differ from Toast; leaving it
  makes the verb break at once, since Barn stores and recompiles formatted
  source (builtins/verbs.go:622-632) while Toast keeps bytecode. That decision
  is a separate issue. The generator never puts a bare `^` directly before a
  property dot (structured.go propertyBase). The old input passes with it.
- Finding 2 (fuzz harness gap, not a production bug): the VM structured
  target stalled at 0 execs/sec within seconds. A 3000-program sweep hung in
  vm.executeLoop on a labeled while loop. `while o (n) endwhile` charges only
  unchecked ticks by design: compiler/lower_control.go:126-130 emits Toast's
  EOP_WHILE_ID tick as TicksUnchecked, matching Toast execute.cc:2286-2288
  (extended-op tick decremented without the limit test, unlike :976-986).
  Barn bounds such loops only by the seconds checkpoint every 1024 ticks
  (vm/vm.go:438-452, :481-491). The fuzz harness builds task.NewState(..., 1)
  and never calls SetExecutionDeadline, so SecondsLeft (task/task.go:814-821)
  stays 1 forever and the loop never stops. bin/moo stops the same program
  after 5 s with E_MAXREC. The raw FuzzSourceExecution shares the gap (and
  PR #616's rewrite keeps NewState(..., 1) without a deadline). Fix in the
  shared executeFuzzSource helper: set an execution deadline.
  vm/testdata/fuzz/FuzzSourceExecution/named-while-unchecked-ticks hangs
  without it (20 s test timeout, goroutine in vm.(*VM).executeLoop) and
  passes in 0.11 s with it.
- The remaining "execs: N (0/sec)" stretches are Go minimizing each new
  interesting input (default -fuzzminimizetime=60s), not hangs: on Linux
  (WSL Debian) a 5 s per-input watchdog wrote nothing during a stall, and with
  -fuzzminimizetime=2s the stall became one new input per ~1 s. The raw
  FuzzSourceExecution shows the same stretches. A 20000-program sweep of the
  generator through the VM helper had no case over 3 s.

## Throughput (Q 2026-11-07: "Measure first")

The 4-minute runs (structured 5730 and 289 execs, raw ~16000) were not a
generator cost. Measured in WSL Debian, `-parallel=2`, 60 s each, ./vm,
rebased onto master:
- Default minimization, load 0.6-2: raw 110 execs, structured 274. Both
  stall at 0/sec for long stretches, the raw target too.
- `-fuzzminimizetime=0s`, load 3.4-3.7: raw 6536 execs (~129/s, 367 new
  interesting), structured 6129 (~115/s, 720 new interesting).
So the stalls are Go minimizing each new interesting input (default 60 s).
The structured target finds about twice as much new coverage, so it pays
that more often. Both run at ~120/s because executeFuzzSource builds a
registry per input. That is shared with the raw target and not changed
here.

## State (2026-11-07)

The coordinator finished the rebase after the agent hit its usage limit:
executeFuzzSource is shared by both VM targets, and the generator no longer
emits a newline string piece or ir.ListRangeExpr (rejected since #622, #620).
Rebased onto master; vet and go test of internal/fuzzseed, parser,
compiler and vm pass. The three structured targets are in the nightly matrix.

## Next step

PR review and merge; the `(^).name` formatter decision is a separate issue.
