# Parser and compiler fuzzing

## Goal

Implement the existing design in issue #414, run the fuzzers to find real
failures, preserve reproductions, and open a PR. Q requested this on 2026-11-01
with https://nullprogram.com/blog/2025/02/05/ as practical fuzzing guidance.

## State

Q approved implementation with "make it so" and explicitly requested fixing
Test.db extraction first. Working in isolated branch `test/parser-fuzzing-414`.
Three targets, the shared baseline/corpus seed loader, and the offline importer
are implemented. Imported 11006 exact-byte unique sources from Test.db,
LambdaCore, two ToastCore snapshots and Mongoose into ignored `.fuzz/corpus`.
The first baseline run found integer-token formatting changes the semantic IR;
the formatter repair and its fuzz regression now pass focused replay. The
first parser mutation found a false positive from case-preserving comparison;
the corrected comparison and retained reproduction also pass focused replay.
Compiler and VM ten-minute campaigns completed successfully. The revised
parser ten-minute campaign found the numeric-property-dot bug described below;
that repair and the related unary postfix precedence repair pass focused
round-trip replay, and the next parser campaign is running. Barn PR #567 is
open. The unrelated main-checkout work is untouched.

Database reader repair is published as lambdamoo-db-py PR #34 on branch
`fix/recycled-anon-record`. Its full CI passed. The real Test.db now extracts
all 444 declared programs, including the anonymous object in recycled slot
15529. That dependency remains open/unmerged; use its worktree for the corpus
import until integrated. Its note is in that worktree's agent-notes directory.

## Decisions and why

The smallest implementation is issue #414's three native Go fuzz targets,
one shared seed loader, an offline corpus importer using the existing Python
database reader, and concise run/reproduction documentation: roughly 300-450
lines before discovered bug fixes and regression inputs. Q explicitly asked
to use the richer local ToastStunt/database-tooling resources on 2026-11-01.

1. In parser tests, fuzz arbitrary source. Rejection is a normal result. For
   accepted input, require checked formatting to succeed, formatted source to
   parse, semantic IR to match with positions removed and identifier/variable
   target names compared case-insensitively, and canonical formatting
   to be stable. Exercise both formatter modes. Reuse `withoutPositions` from
   `parser/format_test.go`; refactor existing round-trip assertions only as
   needed to avoid copying their logic.
2. In external compiler tests, compile arbitrary source through the existing
   builtin registry's compiler and require every accepted program to pass
   `bytecode.VerifyProgram`. Compilation diagnostics are normal results. Use a
   fresh compiler/registry per input so the source cache cannot hide work or
   carry input-dependent state across cases.
3. In VM tests, parse and compile arbitrary input, then execute accepted
   bytecode in a fresh empty store and session with a small tick budget. Use
   the existing descriptor registry and standalone test session setup, a real
   task context/state, and the production `Run` frame installation. Do not
   recover panics in the target. MOO errors, tick exhaustion, and returned
   suspend/fork flows are valid outcomes; do not launch a server or scheduler.
   Use the existing core capability selection to exclude optional external
   I/O families. Close the per-input session. This lane tests bounded initial
   execution, not scheduling or external services.
4. Add one `internal/fuzzseed` helper for all three targets. Preserve #414's
   tracked Test.db baseline, using the existing Barn database reader. Extend
   local campaigns with every verb from LambdaCore, ToastCore and Mongoose,
   extracted by an offline importer through `lambdamoo_db.reader.load` and
   its existing model, not another textdump parser. The Python reader currently
   rejects Barn's Test.db (details below), so it cannot replace the baseline
   reader without resolving that discrepancy. Preserve empty programs and
   anonymous-object verbs; never treat absent programs as empty programs.
   Deduplicate by exact source bytes with the existing fingerprint convention;
   preserve Latin-1 byte values, whitespace, comments, stamps and identifier
   spelling. Save local source digest and object/verb index provenance so a
   finding can be traced back without printing arbitrary source. Do not use
   `match.normalized`: removing whitespace and editor stamps would remove
   lexer inputs. Load the optional imported corpus through this one helper.
   Keep full private corpora local; the PR includes the importer, documentation,
   existing baseline and compact suitable reproductions, not world dumps.
   Add small valid and invalid seeds informed by ToastStunt's parser regression
   tests and parser.y: operators at equal precedence, assignment/catch/ternary
   boundaries, scatter bindings, index markers, comments, strings, numeric
   limits and nesting limits. These are fixed seed cases, not a new grammar
   generator. Reuse existing Barn seed cases where available. Whole-world verb
   count does not establish syntax or coverage diversity; report actual fuzz
   coverage progress and failures, not a claimed coverage percentage from counts.
5. Document exact per-target fuzz and reproduction commands and Go's corpus
   persistence. No new dependencies, custom fuzz engine, grammar generator,
   scheduled workflow, CI fuzz time budget, or copied database fixture.
   Existing PR CI will replay seeds and committed minimized findings.
6. Run each target for at least ten minutes locally as issue #414 requires.
   These are the requested fuzz experiments, not local reruns of CI to gate a
   push. Preserve raw terminal results and minimized failing inputs. Confirm
   a reproduction before claiming a bug. Fix failures of the internal
   invariants with committed corpus entries; any uncertain MOO compatibility
   behavior first requires the documented managed WSL Toast oracle.
7. Commit and push only this task's changes, open an independent PR against
   master, and wait for its CI through terminal results. Fix red checks in
   scope. Q requested a PR, not a merge. Record durations, findings, and any
   remaining limitations in the PR and this note.

Primary sources read: issue #414; parser/AGENTS.md; parser/parser.go;
parser/diagnostics.go; parser/format.go; parser/format_test.go;
parser/operator_roundtrip_test.go; parser/database_census_test.go;
ir/validate.go; compiler/compiler.go;
compiler/source_compilation_contract_test.go; bytecode/verify_test.go;
vm/vm.go; vm/registry.go; vm/registry_required_test.go;
vm/bytecode_execution_test.go; vm/session_test.go; builtins/session.go;
builtins/host.go; builtins/registry.go; builtins/descriptors.go;
config/capabilities.go; db/format/reader.go; Makefile;
.github/workflows/ci.yml; and the article and official Go fuzz documentation.

## Surprises

The first mutation found `for A in(0) if(A(str))endif endfor` changing to STR.
This is intentional canonical identifier spelling, required by the existing
formatter contract and its WAIF/ANON golden test. I initially compared preserved
source spelling as semantic identity. Correct the shared test comparison for
IdentifierExpr and VariableTarget names using the same case-insensitive
identity the compiler defines in lower_locals.go; preserve all literal bytes,
operator/node shapes and other fields. Keep the minimized input as a harness
regression. This is not a product formatter bug.

### Corpus investigation, 2026-11-01

Read local ToastStunt `src/parser.y`, `test/tests/test_moocode_parsing.rb`,
`test/tests/lib/fuzz.rb`, and `test/tests/test_things_that_used_to_crash_the_server.rb`.
Its Ruby fuzz helper mutates characters in a supplied string; it is not a
database corpus extractor. `parser.y` supplies concrete syntax/precedence
evidence for seed selection, not a proof of current oracle acceptance.

Read lambdamoo-db-py AGENTS.md, README.md, pyproject.toml, reader.py,
database.py, inspection.py, exporter.py, lazy.py and match.py. The CLI supports
read-only info/code queries; its normal code display is not a byte-preserving
bulk corpus export. The existing reader preserves Latin-1. `code_fingerprint`
distinguishes absent/empty programs and hashes stored lines with LF endings.
Its normalization helper strips indentation, blank lines and editor stamps;
that is useful for similarity analysis but inappropriate for exact fuzz input.

Ran public reader/fingerprint API across all programmed verbs of these files:

| Source | Programs including empty | Exact-byte unique | New after earlier rows |
| --- | ---: | ---: | ---: |
| lambdamoo-db-py/LambdaCore-latest.db | 1727 | 1702 | 1702 |
| lambdamoo-db-py/toastcore.db | 1949 | 1927 | 494 |
| lambdamoo-db-py/toast2.db | 1950 | 1928 | 4 |
| toaststunt/mongoose.db | 10251 | 10102 | 8742 |

Combined exact-byte unique program count: 10942. This is source diversity, not
measured parser-path coverage. Counts came from `reader.load`, `all_verbs`,
`code is not None`, and `inspection.code_fingerprint`; no source was exported.
The CLI's "with code" counts omit empty programs, explaining the one-program
difference for the first three files. The Mongoose snapshot has 661 anonymous
objects according to `moodb info`; source loading included its full model.

The first aggregate probe stopped at the baseline with this raw failure:

```
lambdamoo_db.reader.ParseError: Parse Error: C:/Users/Q/code/barn/Test.db:217702 : duplicate object record id
```

`reader.py` checks each parsed ID against a shared `_object_ids` set in
`_read_object_id`; that is the source of the diagnostic. Whether the dump is
malformed or the reader rejects a valid layout has not been established.
Do not describe this as a confirmed database or Barn parser bug. Continued the
inventory of independent source databases without suppressing that failure.
No fuzz campaigns or semantic coverage measurements have run.

The remembered parser IR ownership is outdated: current source constructs
`ir.Program`, not `verb.Program`. Current source has fuzz targets elsewhere,
but none under parser or compiler. Issue #414 already specifies this work and
explicitly excludes grammar generation and timed CI fuzzing. Do not expand it
to AFL++ infrastructure: the supplied article targets C/C++, while Barn can
use Go's built-in coverage-guided fuzzer.

## Next step

Run the three focused fuzz campaigns for at least ten minutes each, preserving
failures. Finish documentation and publish the Barn PR, then await its CI.
Before final reporting, update both living notes with actual PR/CI/campaign
results. No merge is authorized by this task.

## Campaign evidence

Windows Go 1.27.1, two workers per target, BARN_FUZZ_CORPUS pointing at the
ignored imported corpus. Each baseline contains 11037 unique sources before
retained findings. Commands use `-run '^$' -fuzz '^TARGET$' -fuzztime=10m
-parallel=2 -timeout=20m` in the explicit target package.

```
compiler: fuzz elapsed 10m1s, execs 38927, new interesting 56, PASS
ok github.com/MongooseMoo/barn/compiler 689.385s
vm: fuzz elapsed 10m1s, execs 52458, new interesting 66, PASS
ok github.com/MongooseMoo/barn/vm 685.949s
```

Parser baseline failure before the integer-token fix:

```
semantic IR changed after formatting
formatted: return --9223372036854775808;
```

The formatter used signed output for a negative literal token created by
modulo-64-bit numeric lexing; the emitted minus became an extra UnaryExpr on
reparse. Emit the unsigned token spelling. Ordinary negative expressions still
retain their existing unary-minus spelling. Regression: wrapped-integer.

After this fix the first parser mutation campaign stopped at 84765 executions,
36 seconds of coordinator time, on canonical identifier spelling; retained
input 556fa3d8dab268b1 is a comparison regression, not a product bug. Corrected
focused replay of both inputs completed with:

```
ok github.com/MongooseMoo/barn/parser 1.278s
```

The next campaign stopped at 253351 executions, 58 seconds of coordinator
time, on `0 .(0)` becoming `0.(0)`. The lexer treats the latter's `0.` as a
float, so the formatted source fails to parse. Preserve parentheses around
integer postfix bases. In the same path, unary bases require parentheses at
postfix precedence; otherwise `(-1)[1]` becomes `-1[1]` with a different tree.
The existing round-trip corpus now includes static/dynamic property reads,
property assignment and unary bases. Retained mutation: dc03b69b4dd7883e.

```
ok github.com/MongooseMoo/barn/parser 1.191s
```
