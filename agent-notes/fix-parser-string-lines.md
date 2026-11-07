# D3: strings stop at source line boundaries

## Goal
Close the reported parser divergence with Toast-first eval conformance.

## State
Independent branch from origin/master. Source changes and Go regressions exist.
PR and CI pending. No merge, comments, or issues authorized.

## Decisions and why
Reject newline/EOF even after backslash in readString. Preserve Missing quote then syntax error. eval previously returned only diagnostics[0]; return all compiler diagnostics so the managed result preserves both errors.
Toast authority: /root/src/toaststunt/src/parser.y:1032-1045,1314-1318.
Stock executable SHA-256: 72fb1cf96cb303647a8ee72808e7c1ff62a491ecf44f547e6757e71ba2402bde.

## Surprises
The lexer correction exposed eval's existing truncation to the first diagnostic. The compiler already produced both errors; fixing the result construction was required.

## Verification
Managed commands run in WSL Debian from this worktree:
`make conformance-toast CONFORMANCE_SUITE=../moo-conformance-parser-string-lines CONFORMANCE_ENV=/root/.cache/parser-string-conformance CONFORMANCE_PATHS=../moo-conformance-parser-string-lines/src/moo_conformance/_tests/language/parser_string_lines.yaml K=parser_string_lines CONFORMANCE_ARGS=--tb=short`
Use `make conformance-k` with the same arguments for Barn. All sessions include
capability_admission, bundled Test.db and disposable fixture copies. Test.db
SHA-256: 1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e.

```text
Toast: 4 passed in 11.07s
Barn master: 2 failed, 2 passed in 17.71s; both inputs returned [1, 'a\nb']
Fixed Barn: 4 passed in 7.83s
go test ./compiler -count=1: ok github.com/MongooseMoo/barn/compiler 2.520s
go test ./compiler -run FuzzCompileVerifiable -count=1: ok github.com/MongooseMoo/barn/compiler 2.529s
strict real corpus: compiled and verified 11006 real verb sources
--- PASS: TestRealVerbCorpus (69.34s)
```

Corpus acceptance used one shared local probe at
../barn-parser-list-range/.tmp/parser_corpus_test.go with BARN_FUZZ_CORPUS set to
../barn-parser-fuzzing-414/.fuzz/corpus. It checks source digests, all compile
results with DefaultCapabilities and vm.Descriptors, and VerifyProgram. A first
Core-only probe rejected optional builtins and was discarded as invalid setup.
No private corpus or probe is staged.

## Next step
Open PR and watch CI; fix red. Do not merge.
