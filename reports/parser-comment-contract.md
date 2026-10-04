# MOO comment syntax and compilation diagnostics

Issue #287. Replace the lexer’s `//` skipping with whitespace and consecutive
`/* ... */` comments. Comments end at the first `*/`; `/` remains division and
quoted markers remain string contents. Delete the obsolete line-comment helper.
Unterminated comments return an illegal token at the opening position and retain
their explicit lexical diagnostic through ParseProgram, returning no program.
Ordinary parser errors keep their existing generic syntax-error presentation.
The parser remains independent of Barn runtime types.

Durable tests live in the companion moo-conformance-tests branch
`test/287-parser-comment-contract`, based on
`7f05b7070f2954a200f7e8c2a06c814ef7981c4e`. The tests create their own receiver
and verb, assert compiled execution, and prove failed compilation preserves the
existing verb. No existing expectations were weakened or skipped.

Managed Debian WSL oracle: `/root/src/toaststunt/build-release/moo`, stock
ToastStunt reference `aecc51e9449c6e7c95272f0f044b5ba38948459e`.
Disposable fixtures are owned by the managed harness. Source Test.db SHA256:
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
Final `language/parser_comment_contract.yaml` SHA256:
`0740d861d22d2ca6f22e0e5dfddeedd7e1536ddab2602947f39065643b30e820`.
WSL Python environment: `/root/.cache/barn-287-conformance-venv`.

## Oracle and regression evidence

The initial nine scenarios were verified on Toast before reading or changing
Barn’s lexer. The unchanged suite then failed five cases on baseline Barn
`2d3bbb3dbf442033e32a16ce8ba451093df9a1d2`:

```text
Toast: test_capability_admission PASSED; 10 passed in 18.03s
Barn:  test_capability_admission PASSED; 5 failed, 5 passed in 6.59s
leading_trailing_block: expected [1, 7], actual [0, 99]
consecutive_blocks: expected [1, 7], actual [0, 99]
inline_division: expected [1, 6], actual [0, 99]
nested_looking_first_close: expected [1, 7], actual [0, 99]
reject_line_comment: expected [1, 99], actual [0, 7]
```

A temporary managed oracle discovery test returned diagnostics for the source
lines `return 7;`, `/*`, `unfinished`. Its empty expected list intentionally
exposed the actual diagnostic:

```text
expected value [], but got ['Line 2:  End of program while in a comment']
```

The final durable case asserts that exact result; the temporary discovery test
was removed. The specific message and opening line are verified oracle behavior,
not an inferred generic EOF error. The original nine scenarios remain unchanged.

```text
Final managed Toast suite: 11 passed in 22.68s
Final managed Barn suite:  11 passed in 7.51s
```

Each focused managed session selects the canonical capability_admission test.
The Toast command, from the conformance worktree in Debian with the environment
above exported, is:

```sh
uv run --frozen moo-conformance \
  src/moo_conformance/_tests/language/parser_comment_contract.yaml \
  --server-command='/root/src/toaststunt/build-release/moo {db} {db}.out -p {port}' \
  --server-db=src/moo_conformance/_db/Test.db \
  --server-db-dir=src/moo_conformance/_db/startup --moo-host=127.0.0.1 \
  -k 'capability_admission or parser_comment_contract' -v --tb=short
```

The Barn command uses the documented managed invocation in CLAUDE.md with the
conformance project and fixture paths pointing to the companion worktree,
the same selector, `bin/barn` built from verified `./cmd/barn`, the
`barn-linux-testdb-outbound-on` profile/config, and
`profiles/toast/stock-wsl-testdb.json`. The harness starts and stops both servers;
no manual server or tracked-database execution was used.

## Internal validation

New lexer/parser regressions fail on baseline and pass after the change. They
cover consecutive/comment-only trivia, division, quoted markers, non-nesting,
rejected line comments, unterminated comments in multiple parser contexts,
byte offset/line/column across multiline comments, and progress to EOF after
the illegal token. Unterminated comments never return a partial semantic program.

```text
go test ./parser -run '^TestBlockComment' -count=1: ok 0.554s
go test ./...: successful
go test -race ./parser -count=1: ok 96.454s
go vet ./...: successful
staticcheck ./...: successful
COMMENT_FULL_RACE_STATIC_OK
```

The first full WSL Go run failed only the repository-hygiene Git probe because
the linked worktree’s .git file contains a Windows path. Setting the matching
Linux GIT_DIR/GIT_WORK_TREE paths fixed that probe; the complete Go run then
passed. No test or Git configuration was weakened to bypass it.

Merge the Barn implementation before the companion conformance PR so master’s
required conformance gate remains clean. Both changes require their own final
head CI and normal merge history.

Kept implementation: `6607faa65ea69f757b7acbb9aa4e3044b39c7d02`.
Companion tests: https://github.com/MongooseMoo/moo-conformance-tests/pull/118,
head `4c6d705a48ba868eda7f478a634f6f9c32cd1ecc` (one YAML file).
