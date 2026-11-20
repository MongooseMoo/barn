# D2: splice argument entries

## Goal
Close the reported parser divergence with Toast-first eval conformance.

## State
Independent branch from origin/master. Source changes and Go regressions exist.
PR and CI pending. No merge, comments, or issues authorized.

## Decisions and why
Move splice parsing out of ParseExpression into parseArgument, used by list, call, catch/except codes, and the argument-list prefix of optional scatter assignments. Preserve assignment expressions within a splice. Replace invalid standalone-splice formatter fixtures with valid list splices.
Toast authority: /root/src/toaststunt/src/parser.y:530-533,707-726,742-750.
Stock executable SHA-256: 72fb1cf96cb303647a8ee72808e7c1ff62a491ecf44f547e6757e71ba2402bde.

## Surprises
Strict corpus initially found two scatter-prefix regressions. Toast confirmed the reduced rest-before-optional case; parseScatterAssign must call the same argument parser.

## Verification
Managed commands run in WSL Debian from this worktree:
`make conformance-toast CONFORMANCE_SUITE=../moo-conformance-parser-splice CONFORMANCE_ENV=/root/.cache/parser-splice-conformance CONFORMANCE_PATHS=../moo-conformance-parser-splice/src/moo_conformance/_tests/language/parser_splice_operand.yaml K=parser_splice_operand CONFORMANCE_ARGS=--tb=short`
Use `make conformance-k` with the same arguments for Barn. All sessions include
capability_admission, bundled Test.db and disposable fixture copies. Test.db
SHA-256: 1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e.

```text
Toast: 4 passed in 11.08s; expanded scatter coverage: 5 passed in 9.21s
Barn master: 2 failed, 2 passed in 17.42s; both division cases raised E_TYPE
Fixed Barn initial: 4 passed in 10.13s
Fixed Barn final (including scatter coverage): 5 passed in 10.00s
Final strict real corpus: compiled and verified 11006 real verb sources
--- PASS: TestRealVerbCorpus (3.96s)
go test ./compiler -count=1: ok github.com/MongooseMoo/barn/compiler 2.489s
go test ./compiler -run FuzzCompileVerifiable -count=1: ok github.com/MongooseMoo/barn/compiler 2.491s
```

Corpus acceptance used one shared local probe at
../barn-parser-list-range/.tmp/parser_corpus_test.go with BARN_FUZZ_CORPUS set to
../barn-parser-fuzzing-414/.fuzz/corpus. It checks source digests, all compile
results with DefaultCapabilities and vm.Descriptors, and VerifyProgram. A first
Core-only probe rejected optional builtins and was discarded as invalid setup.
No private corpus or probe is staged.

## Next step
Open PR and watch CI; fix red. Do not merge.
