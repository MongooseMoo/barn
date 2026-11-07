# Fix parser Toast divergences D1-D3

## Goal
Fix three Barn-vs-Toast parser/lexer divergences found by the differential (PR #617):
- D1: range inside a list literal (`return {0..0};`) -- Toast syntax error, Barn compiles.
- D2: `@` splice as binary-operator operand (`return {1/@{}};`) -- Toast syntax error, Barn compiles.
- D3: string literal spanning source lines -- Toast "Missing quote" + "syntax error", Barn accepts.

For each: Toast source citation, conformance test (moo-conformance-tests, own PR), Barn fix (own PR), Go unit test.

## State
Taken over 2026-11-06 in isolated branch fix/parser-list-range from origin/master.
Required instructions and differential discovery note read. D1 production fix
removes the list-range source production. Compiler and formatter fixtures now
use valid indexed ranges. Conformance branch test/parser-list-range is independent
from origin/main. D2 and D3 use fix/parser-splice-operand and fix/parser-string-lines.

## Decisions and why
- Do not merge or post comments/issues. Open three independent Barn PRs and three conformance data PRs.
- D1 is list-literal parsing; D2 is splice placement; D3 is string lexing. Keep
  three independent Barn branches unless source proves D1/D2 share one rule.
- Plan: assert eval rejection and exact Toast diagnostics, prove managed Toast
  green then Barn master red, fix each production owner with Go regressions,
  replay compiler seeds and real verb corpus, then publish and watch CI.
- Conformance changes stay YAML plus task documentation; no harness changes.

## Surprises
- The compiler and formatter tests included invalid list-range source; replace
  those fixtures with indexed ranges rather than retaining the unsupported syntax.
- A preliminary corpus probe used config.Core and rejected network/SQLite calls.
  Discarded that setup; strict acceptance uses config.DefaultCapabilities().

## Findings / commands log
- Toast parser.y:630-634 defines list literals using arglist; 700-726 permits
  expressions or @-prefixed expressions as entries. A standalone range is not
  an expression; indexing ranges are expr '[' expr tTO expr ']' at 446-452.
- Binary division is expr '/' expr at 530; @ is not an expression production.
- String lexing at parser.y:1032-1045 reports Missing quote for newline/EOF,
  including after a backslash. my_getc:1314-1318 emits newline between eval args.
- Live Toast source pin matches the documented stock oracle. Binary SHA-256:
  72fb1cf96cb303647a8ee72808e7c1ff62a491ecf44f547e6757e71ba2402bde.

## Next step
Open PR, watch CI, fix red. Merge remains forbidden.

## Verification evidence
Bundled conformance Test.db SHA-256:
1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e.
Managed commands run in WSL Debian from this worktree:
`make conformance-toast CONFORMANCE_SUITE=../moo-conformance-parser-rejections CONFORMANCE_PATHS=../moo-conformance-parser-rejections/src/moo_conformance/_tests/language/parser_list_range.yaml K=parser_list_range CONFORMANCE_ARGS=-v`
Then the same suite/paths/selector via `make conformance-k` on master and fixed Barn.
Every session includes capability_admission and uses disposable fixture copies.

```text
Toast: 4 passed in 8.11s
Barn master: 2 failed, 2 passed in 7.82s
range_in_list_rejected expected [0, ['Line 1:  syntax error']], got [1, [0]]
range_in_indexed_list_rejected expected [0, ['Line 1:  syntax error']], got [1, 0]
Fixed Barn: 4 passed in 10.25s
go test ./compiler -count=1
ok github.com/MongooseMoo/barn/compiler 2.326s
go test ./compiler -run FuzzCompileVerifiable -count=1
ok github.com/MongooseMoo/barn/compiler 2.408s
strict real corpus: compiled and verified 11006 real verb sources
--- PASS: TestRealVerbCorpus (69.42s)
```

Strict corpus probe is a single local test at `.tmp/parser_corpus_test.go`, reused
from all three worktrees without copying. It reads the existing private corpus
at ../barn-parser-fuzzing-414/.fuzz/corpus, verifies every filename SHA-256, uses
the full VM registry including eval/pass, fails on any compile diagnostic, and
verifies every emitted program. Command: set BARN_FUZZ_CORPUS to that directory,
then `go test C:/Users/Q/code/barn-parser-list-range/.tmp/parser_corpus_test.go -count=1 -v`.
No corpus or probe is staged. Full CI remains pending at this checkpoint.

## PR CI repair
Barn PR #620; conformance PR #172. CI found VM opcode tests still compiling
list-range source. Preserve the same quota and integer-endpoint assertions by
assembling OP_LIST_RANGE bytecode and executing it through VM.Run; source
compilation is no longer the producer for that opcode. Share the existing test
execution helper rather than duplicating VM setup.

```text
Build & unit tests fail 1m23s
--- FAIL: TestListRangeChecksQuotaBeforeAllocation
bytecode_execution_test.go:237: compile failed: [Line 1:  syntax error]
--- FAIL: TestListRangeUsesExclusivePendingQuota
--- FAIL: TestListRangeTerminatesAtIntegerEndpoints
go test ./vm -run '^TestListRange' -count=1
ok github.com/MongooseMoo/barn/vm 1.540s
```
