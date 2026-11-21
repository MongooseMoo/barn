# Check that database code runs on Toast (issue #977)

## Goal

A `barn` inspection command over `--db` that names every verb whose code the
target Toast cannot compile or run as written (a call to a builtin Toast does
not register, or syntax its parser rejects), plus a workflow that runs it on the
pinned Mongoose dump and fails on a nonzero count. Counts only in public logs.

## State

Built, tested locally, committed on branch `check/toast-runnable-code`; not
pushed, no PR (the lead publishes). The workflow has not run on GitHub.

- `barn toast-check --db DB --builtins FILE`: `internal/dbtool/toast_check.go`,
  wired in `cmd/barn/main.go` (`toastCheckCommand`) and `internal/app/app.go`
  (`ToastBuiltins`). Prints one line per finding, then
  `toast-check: verbs=N calls_barn_only_builtin=A unparsable=B calls_unregistered_builtin=C not_checkable=D`.
  Exit 1 when A+B > 0, 2 without `--builtins`.
- `scripts/toast-check.sh TOAST BARN DATABASE DIR`: asks the Toast binary for
  its builtins (emergency mode, `function_info()`), runs the check, prints the
  count line and per-builtin call counts for the Barn-only class only.
- `.github/workflows/toast-check.yml`: builds Toast at
  `profiles/toast/mongoose-0ff6fd7-wsl-testdb.json`'s ref (options.h checksum
  checked), copies the goodput-series.yml fixture and checks its SHA-256, runs
  the script.
- Tests: `internal/dbtool/toast_check_test.go` (5 tests), plus a usage case in
  `cmd/barn/command_test.go` `TestCommandLineMistakesExitTwo`.

### Results

`go test ./internal/dbtool -run 'TestToastCheck' -count=1 -v`:
```
--- PASS: TestToastCheckNamesPlantedOrdCall (0.01s)
--- PASS: TestToastCheckIgnoresPropertyNamedOrd (0.00s)
--- PASS: TestToastCheckClassesThatDoNotFail (0.00s)
--- PASS: TestToastCheckClassesThatFail (0.01s)
--- PASS: TestToastCheckRefusesEmptyBuiltinList (0.00s)
ok  	github.com/MongooseMoo/barn/internal/dbtool	0.193s
```
`go test ./cmd/barn -run 'TestCommandLineMistakesExitTwo$'`: PASS.
gofmt -l: nothing; go vet, staticcheck, golangci-lint on the three packages:
clean ("0 issues.").

`bin/barn toast-check` with the 72e3c7f binary's list (.tmp copies):
```
== suite Test.db (1a3f23eb...)
#2:eval: line 5: not checkable: eval
toast-check: verbs=5 calls_barn_only_builtin=0 unparsable=0 calls_unregistered_builtin=0 not_checkable=1
exit 0
== planted.db (suite Test.db + ord() in #0:handle_task_timeout)
#0:handle_task_timeout: line 1: calls ord, which Barn registers and Toast does not
#2:eval: line 5: not checkable: eval
toast-check: verbs=5 calls_barn_only_builtin=1 unparsable=0 calls_unregistered_builtin=0 not_checkable=1
... level=ERROR msg="Barn failed" err="1 verbs Toast cannot run as written"
exit 1
== worktree root Test.db (532fef8f...)
#2:eval: line 5: not checkable: eval
toast-check: verbs=444 calls_barn_only_builtin=0 unparsable=0 calls_unregistered_builtin=0 not_checkable=1
exit 0
```
`scripts/toast-check.sh` in WSL on a copy of the pinned Mongoose dump
(SHA-256 626bd284...25e7 checked `OK`), counts only:
```
== 0ff6fd7 build (SHA-256 1c0e5128...5a53)
Toast registers 247 builtins (emergency mode exit 0)
toast-check: verbs=28267 calls_barn_only_builtin=0 unparsable=0 calls_unregistered_builtin=18 not_checkable=34
script exit 0
== 72e3c7f build (SHA-256 a748a936...d098)
Toast registers 238 builtins (emergency mode exit 0)
toast-check: verbs=28267 calls_barn_only_builtin=22 unparsable=0 calls_unregistered_builtin=18 not_checkable=34
26 calls to parse_ast, ...   5 unparse_ast, 2 validate_ast, 2 verb_ast,
4 validate_string_lines, 4 waif_owner_count
script exit 1
```
Same script on planted.db, both builds: `calls_barn_only_builtin=1`,
`1 calls to ord, which Barn registers and Toast does not`, script exit 1.

The workflow's build step, run by hand in WSL on a fresh clone at 0ff6fd7
(`mkdir build; cd build; cmake -DCMAKE_BUILD_TYPE=Release ..; cmake --build .`):
options.h `OK`, build exit 0, `moo` SHA-256 1c0e5128...5a53, identical to the
installed 0ff6fd7 binary.

## Unknown 1: what Toast does with a call to a builtin it does not register

Source, Mongoose Toast ref `72e3c7f96ce7a41fdeba793aef8818dc4408072e`
(`profiles/toast/mongoose-wsl-mongoose.json`), read with `git show`:

- `src/parser.y:498-515`: `IDENTIFIER '(' arglist ')'` with
  `number_func_by_name` = `FUNC_NOT_FOUND` compiles to
  `call_function("<name>", args...)` and calls
  `warning("Unknown built-in function: ", name)`.
- `src/parser.y:825-831`: `warning()` calls the client's warning handler, or
  `error()` when the client has none.
- `src/db_io.cc:246-251,271-272`: the database reader's client has a warning
  handler (`oklog("PARSER: Warning in ...")`), so load continues.
- `src/parser.y:1324`: `list_parser_client = { my_error, 0, my_getc }`, no
  warning handler, so `set_verb_code`/`eval` turn the warning into a compile
  error and refuse the code.
- `src/execute.cc:3445-3448`: at run time `call_function` raises
  `E_INVARG, "Unknown built-in function", "<name>"`.
- `src/db_file.cc:996-999`: a verb program that does not parse fails the
  whole load (`READ_DB_FILE: Unparsable program`, return 0).

Runs (Toast emergency mode, `/root/src/toaststunt-mongoose/build-release/moo`,
SHA-256 a748a936...d098, git HEAD 72e3c7f9, on disposable copies under
`.tmp/toast-plant` of the suite's Test.db, SHA-256 1a3f23eb...408e):

set_verb_code with `return ord("a");`:
```
=> {"Line 1:  Unknown built-in function: ord"}
=> {}            (verb_code: nothing installed)
```
Database file copy with `if (args == {"probe"}) return ord("a"); endif`
planted as line 1 of `#0:handle_task_timeout`:
```
PARSER: Warning in #0:handle_task_timeout:
           Line 1:  Unknown built-in function: ord
LOADING: Done reading 5 verb programs ...
=> {E_INVARG, "Unknown built-in function", "ord", {{#0, "handle_task_timeout", #3, #0, #3, 2}, ...}}
```
Database file copy with `return 1 +;` planted there instead:
```
*** PARSER: Error in #0:handle_task_timeout:
***            Line 1:  syntax error
*** READ_DB_FILE: Unparsable program #0:2.
*** DB_LOAD: Cannot load database!
exit 1
```

So: an unknown builtin loads with a log warning, raises E_INVARG when it runs,
and the verb cannot be re-saved on Toast as written. Syntax Toast rejects
makes the whole database unloadable.

## Unknown 2: syntax Barn accepts and Toast rejects

Not settled in general. What I have:

- Barn's token set (`parser/token.go:13-90`) is the same as Toast's grammar
  tokens and operators (`parser.y` `%token` and precedence lines, ref 72e3c7f
  lines 93-115): no keyword or operator Barn lexes that Toast lacks. Barn has
  no `true`/`false` literal in its lexer (only `format.go:601` prints one).
- Toast's statement and expression productions (parser.y 72e3c7f, rule heads
  at lines 141-780) against Barn's parse functions: I did not do a full
  production-by-production proof.
- Spot check, 112 probe programs (`.tmp/syntax-probe/probes.txt`), each
  compiled with `set_verb_code` on a scratch verb, Barn (`barn eval --file`)
  vs Mongoose Toast emergency mode, on copies of the suite's Test.db. Barn
  accepted none that Toast rejected. Toast accepted three that Barn rejects
  (the other direction):
  ```
  => {"P009", {}}|=> {"P009", {"Line 1:  syntax error"}}    é = 1;
  => {"P010", {}}|=> {"P010", {"Line 1:  syntax error"}}    x = #0.:foo;
  => {"P064", {}}|=> {"P064", {"Line 1:  syntax error"}}    x = a.:b = 3;
  ```
  (Toast left, Barn right; `.:` is Toast's waif-property syntax, parser.y
  72e3c7f line 405-416.) One message differs on a program both reject (P069
  `x = 1e;`: Toast adds "Malformed floating-point literal").
- So the machine does not try a syntax check against Toast. It reports a verb
  whose stored source does not parse on Barn as its own class; Toast refuses
  to load a database holding such a verb (unknown 1).

## Decisions and why

- Builtin list from the Toast binary, not its source. The source grep on
  72e3c7f gives 243 names; the built binary's `function_info()` gives 238. The
  five extra (anon, background_test, finished_tasks, malloc_stats, read_stdin)
  are compiled out, so a source list would hide calls to them.
- Emergency mode is one `moo -e -p 0 in.db out.db < input` line in
  `scripts/toast-check.sh`, on a copy of
  `engine/testdata/toast-command-environment.db` (any loadable db gives the
  same set; checked against the suite Test.db: same 238 names). I did not
  route it through `scripts/bench_differ.py`/`fuzz_differential.py`: their
  lane machinery (corpus probes, chunking, timeouts, WSL relay) is not needed
  for one fixed input, and none of their code is copied.
- Target Toast is 0ff6fd7, `profiles/toast/mongoose-0ff6fd7-wsl-testdb.json`:
  `docs/mongoose-conformance.md:62-66` calls it "the commit the live game
  runs". `mongoose-wsl-mongoose.json` (72e3c7f) lacks parse_ast, unparse_ast,
  validate_ast, verb_ast, validate_string_lines and waif_owner_count, which
  the live dump calls 43 times in 22 verbs; checking against it would report
  verbs that, per that doc, run on the live Toast today.
- Calls to a name neither engine registers are their own class and do not
  fail the check: they fail on Barn as on Toast, so going back loses nothing.
  The live dump has 18 such verbs; failing on them would make the workflow
  red for pre-existing code Barn did not introduce. Barn's compiler refuses a
  direct call to an unknown name (`compiler/compiler.go:133-137`).
- `eval(...)` and `call_function` with a non-literal first argument are
  listed as not checkable and do not fail the check (34 verbs in the dump);
  their callee is not known from the code.
- Calls are found by walking the IR from `parser.ParseProgramWithDiagnostics`
  (a reflection walk over exported fields for `*ir.BuiltinCallExpr`), not by
  regex. Names compare in any letter case, as Toast's
  `number_func_by_name` uses `strcasecmp` (functions.cc:187-196, 72e3c7f).
- Objects come from `store.All()` sorted by id, so anonymous objects' verbs
  are included (`GrepVerbs` walks numbered objects only).
- The check reads code as Barn holds it after load: `dbtool.LoadStore` runs
  `LowerUnknownBuiltinCalls`, so a call Barn does not register is seen as
  `call_function("name", ...)` and its line number is in the lowered source.

## Surprises

- The worktree's root `Test.db` is not the suite's Test.db (it reads 15578
  objects) and Mongoose Toast segfaulted (exit 139) loading it in emergency
  mode. Not investigated; I used the suite's Test.db instead.

- Barn rejects syntax Toast accepts (`.:` waif properties, a non-ASCII
  identifier); see unknown 2. The live dump has 0 verbs Barn cannot parse.
  Not filed.

## Next step

Push the branch, open the PR, watch the `Toast check` workflow's first run on
a self-hosted runner (it needs the workstation's fixture path and Toast's
build dependencies). Unverified: the workflow itself has never run.
