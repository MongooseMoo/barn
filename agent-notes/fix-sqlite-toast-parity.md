# fix/sqlite-toast-parity

## Goal

Resolve MongooseMoo/barn #986: make Barn's `sqlite_*` builtins return what
Toast returns for the 17 probes in the issue, each pinned first by a
Toast-passing conformance test in moo-conformance-tests
(branch `test/sqlite-toast-parity`).

Done = per-item Toast-green conformance tests committed in the suite worktree,
Barn fixes committed locally on `fix/sqlite-toast-parity`. Not included: push,
PR, any GitHub post (team lead publishes).

## State

Started 2026-11-20; resumed 2026-11-21 by a new agent (the first hit a usage
limit after the skeleton).

All 17 items are pinned by Toast-green conformance tests (stock and Mongoose
0ff6fd7, both green) and fixed in Barn. Committed locally, nothing pushed:

- Suite, branch test/sqlite-toast-parity: "Pin ToastStunt SQLite builtin
  result shapes" (builtins/sqlite_toast_parity.yaml, items 1, 2, 4-17), then
  "Pin sqlite_execute binding INT parameters as 32-bit"
  (builtins/sqlite_execute_int_bind.yaml, item 3 only).
- Barn, branch fix/sqlite-toast-parity: "Run SQLite builtins through the C
  API the way Toast does" (items 1, 2, 4-17), a merge of origin/master, then
  "Bind sqlite_execute INT parameters as 32-bit like Toast" (item 3 only:
  builtins/sqlite_raw.go bind case and one test row), then the note update.

Run records are in the run sections below.

### Barn-green runs, 2026-11-21 (merged tree, same selection and command as Barn-red)

- Fix A (all items except 3; bin/barn-linux-fixA), loadavg 9.87:
  `2 failed, 24 passed in 346.10s`. Failed: exactly the two
  sqlite_execute_int_bind.yaml tests (item 3, not yet changed).
- Fix B (A plus the item 3 commit; bin/barn-linux-fixB), loadavg 12.63:
  `26 passed, 14091 deselected in 337.96s`.

Both worktrees merged with their remotes 2026-11-21 (barn: merge commit of
origin/master; suite: already up to date). Suite submodule toastcore initialised.

### Causes per item (Toast = upstream `git show origin/master:src/sqlite.cc`, Barn = builtins/sqlite.go before the fix)

Stock and Mongoose 0ff6fd7 have byte-identical src/sqlite.cc and
src/include/sqlite.h; numbers.cc differs only in PROMOTE_NUMBERS comparison
code, not parse_number/parse_float; pcre_moo.cc differs only in pcre_match /
pcre_replace leak and isprint fixes, not sqlite_regexp. Both WSL binaries link
the same /lib/x86_64-linux-gnu/libsqlite3.so.0 (Debian 3.40.1-2+deb12u2) and
libpcre2-8.so.0 (ldd).

1. Handle reuse: Toast deallocate_handle resets next_sqlite_handle to 1 when the
   map empties (sqlite.cc:135-136). Barn never reset nextID (sqlite.go:458-459).
2. Column text parsing: Toast reads every column as text (sqlite3_exec argv,
   sqlite3_column_text, sqlite.cc:153-188, 428) and converts it with
   string_to_moo_type (sqlite.cc:48-81): "#N" -> OBJ when PARSE_OBJECTS and
   parse_number(str+1) succeeds; parse_number = strtoimax base 10, trailing
   ' ' only (numbers.cc:81-97); parse_float = skip ' ', '-', strtod, trailing
   ' ' only (numbers.cc:116-135; strtod takes hex, inf, nan). Barn returned
   the driver's native Go values (sqlite.go:155-175).
3. INT parameter: Toast sqlite3_bind_int (C int, low 32 bits) at
   sqlite.cc:394. Barn passed int64 through database/sql (sqlite.go:142-143).
4. Unbound types: Toast's bind switch has STR/INT/FLOAT/OBJ only
   (sqlite.cc:388-406); other types leave the parameter NULL. Barn bound
   v.String() for every other type (sqlite.go:148-152).
5. sqlite_limit float category: Toast leaves category -1 for non-INT/STR, then
   E_INVARG (sqlite.cc:600-617). Barn returned E_TYPE (sqlite.go:353-354).
6. Limit defaults: Toast returns sqlite3_limit() of the linked library
   (sqlite.cc:623). Debian's libsqlite3 3.40.1 is built with
   SQLITE_MAX_VARIABLE_NUMBER=250000 (Toast reports 250000); modernc 3.50.4
   is compiled with SQLITE_MAX_VARIABLE_NUMBER = 32766 and
   SQLITE_MAX_FUNCTION_ARG = 1000 (Toast's library: 127). So Toast's value is
   determined by the linked SQLite build, not Toast code. Barn returned a
   hard-coded table and never touched the connection (sqlite.go:50-65, 657-663).
7. LIMIT_LENGTH: same cause as 6: Barn never called sqlite3_limit.
8. Open options: Toast stores arg 2 as the option bits (sqlite.cc:261-262;
   PARSE_TYPES 2, PARSE_OBJECTS 4, SANITIZE_STRINGS 8, sqlite.h:13-15) and
   sqlite_info reports them (sqlite.cc:352-354). Barn ignored arg 2 and
   reported constants (sqlite.go:543-549).
9. Idle interrupt: Toast calls sqlite3_interrupt (sqlite.cc:647); SQLite
   clears the flag when the next statement starts with none active. Barn kept
   a pendingInterrupt that cancelled the next statement (sqlite.go:106-109, 683).
10. sqlite_handles order: Toast iterates std::unordered_map<int,...>
   (sqlite.cc:325-326), libstdc++ order. Barn sorted ascending (sqlite.go:518).
11. execute tail: Toast sqlite3_prepare_v2 compiles only the first statement
   (sqlite.cc:375). Barn's driver Exec ran every statement.
12. RETURNING: Toast collects rows from any statement (exec callback, step
   loop). Barn guessed row-returning statements by keyword prefix
   (sqlite.go:127-138), so INSERT ... RETURNING went to Exec.
13. Multi-statement query: Toast sqlite3_exec runs every statement and calls
   back per row (sqlite.cc:496). Barn's driver Query returned only the last
   statement's rows.
14. Empty execute: Toast prepares "" to a NULL statement, sqlite3_step(NULL)
   is SQLITE_MISUSE, and sqlite3_errmsg is "not an error" (sqlite.cc:409-416).
   Barn Exec'd and returned {}.
15. REGEXP: Toast registers REGEXP on every opened connection when built with
   PCRE2 (sqlite.cc:276-278; sqlite_regexp pcre_moo.cc:454-489: argv[0]
   pattern, argv[1] string, sqlite3_value_text, get_pcre(pattern, 0) i.e.
   case-sensitive, result_int(match >= 0); NULL pattern/string give the error
   texts "SQLite REGEXP called with invalid pattern." / "... invalid string.").
   Whether REGEXP exists is a Toast build option (PCRE2_FOUND); both WSL
   builds have it. Barn registered nothing.
16. 0.1 + 0.2: the REAL column goes through SQLite's 15-significant-digit text
   ("0.3", same in 3.40.1 and modernc 3.50.4, checked by casting to TEXT in
   both libraries) then parse_float, so Toast returns 0.3. Barn returned the
   driver's float64. Determined by SQLite's REAL->TEXT conversion (library)
   plus Toast's text path (code).
17. inf.0: SQLite renders infinity as "Inf" (both libraries), strtod parses it,
   and Toast prints floats with "%.*g", DBL_DIG then ".0" when the text has no
   '.' or 'e' (list.cc:461-466), so glibc's "inf" becomes "inf.0". Barn's
   floattext.Format spelled "Inf" (internal/floattext/floattext.go). Toast
   code determines the spelling; the library only supplies "Inf".

### Toast-green run 1 (stock), 2026-11-21

Command (WSL root, scratchpad script `sqlite-parity-run.sh stock -q -rA`):
`uv run --project <suite> --frozen moo-conformance -m "admission or conformance"
--server-command "/root/src/toaststunt/build-release/moo {db} {db}.new -p {port}"
--server-db=<suite>/src/moo_conformance/_db/Test.db --server-db-dir=<suite>/src/moo_conformance/_db/startup
--moo-host=127.0.0.1 --oracle-profile-manifest=<barn>/profiles/toast/stock-wsl-testdb.json
--target-profile-manifest=<same> --fail-on-unexpected-skip --strict-markers
-k "capability_admission or sqlite_toast_parity or sqlite_execute_int_bind"`
loadavg at start 14.26 (CI runners busy). Result: `2 failed, 25 passed`.
The two failures were my guesses, not Toast-green yet:
- 15 open handles: Toast returned `[15, 14, 1, 2, ..., 13]`, not descending.
  This is libstdc++'s unordered_map: 13 buckets on first insert, rehash to 29
  at the 14th element (rehash pushes each node to the front, reversing the
  list), each insert into an empty bucket goes to the list front. The order
  depends on the bucket count left by every earlier sqlite_open in the server's
  life, so a conformance test of it is history-dependent; the test was
  removed and the observation is pinned in a Barn Go test instead.
- LIMIT_LENGTH 3: Toast also fails `SELECT 'abc'` ("string or blob too big":
  the result column name `'abc'` is 5 bytes), and `sqlite_execute(db,
  "SELECT ?", {"abcdef"})` returns `{{"NULL"}}` (bind_text fails with
  SQLITE_TOOBIG, which sqlite.cc ignores, so the parameter stays NULL).
  Expectation corrected to what Toast does.

### Toast-green run 2 (stock), 2026-11-21

Same command after removing the 15-handle test and correcting LIMIT_LENGTH.
loadavg 13.39. Result: `26 passed, 14091 deselected in 299.43s`
(capability_admission + 23 sqlite_toast_parity + 2 sqlite_execute_int_bind).

### Toast-green run (Mongoose 0ff6fd7), 2026-11-21

Same selection, `--server-command "/root/src/toaststunt-mongoose-0ff6fd7/build-release/moo {db} {db}.new -p {port}"`,
oracle and target manifest `profiles/toast/mongoose-0ff6fd7-wsl-testdb.json`.
loadavg 12.10. Result: `26 passed, 14091 deselected in 250.38s`.
Stock versus Mongoose answer: no difference on any item; the tests pin what
both builds do.

Suite commits (worktree moo-conformance-tests-sqlite-parity, branch
test/sqlite-toast-parity): "Pin ToastStunt SQLite builtin result shapes"
(sqlite_toast_parity.yaml), then "Pin sqlite_execute binding INT parameters
as 32-bit" (sqlite_execute_int_bind.yaml, item 3 alone).

### Barn fix (all items except 3)

- builtins/sqlite_raw.go (new): Toast's parse_number / parse_float with a
  glibc-strtod scanner; string_to_moo_type; sqlite_query through
  Xsqlite3_exec with a row callback; sqlite_execute through
  prepare_v2 + bind (STR, INT, FLOAT, OBJ only) + step loop reading
  column_text; REGEXP registered per connection with sqlite3_create_function,
  matching via Barn's pcre cache case-sensitively (get_pcre(pattern, 0));
  sqlite3_interrupt on the stored sqlite3*.
- Limits: Barn keeps the values Toast's library holds (3.40.1 Debian
  defaults and hard bounds, LIMIT_LENGTH at least 1) and writes the same
  value into the connection's aLimit, bounded by modernc's compiled
  SQLITE_MAX_* constants. Direct write rather than sqlite3_limit because
  modernc 3.50.4 raises LIMIT_LENGTH below 30 to 30 (SQLITE_MIN_LENGTH),
  where 3.40.1 keeps 3. Never above modernc's bound: SQLite sizes the
  expression node's variable-number field by SQLITE_MAX_VARIABLE_NUMBER.
  Consequence: sqlite_limit reports 250000 for LIMIT_VARIABLE_NUMBER, but a
  statement using more than 32766 variables fails on Barn and works on Toast.
- builtins/sqlite_handle_order.go (new): libstdc++ unordered_map order.
- Handle numbering restarts at 1 when the last handle closes; a failed open
  still uses a number (Toast allocates before sqlite3_open).
- sqlite_open option bits stored and reported by sqlite_info.
- sqlite_limit: non-INT/STR category is E_INVARG; arg 3 type is checked first.
- internal/floattext: infinities and NaN print "inf.0", "-inf.0", "nan.0",
  "-nan.0" (list.cc:461-466 printf "%.*g" plus ".0").
- compat_sqlite_test.go: the obj-column assertion expected the string "#0";
  Toast returns the object #0, so the Barn unit test was corrected.

### Barn-red run (pre-fix binary), 2026-11-21

Binary: linux cross-build of the branch after merging origin/master, before
any change (bin/barn-linux-prefix). Command: the same selection with
`--server-command "<barn> serve --db {db} --listen tcp://127.0.0.1:{port}
--config=<barn>/profiles/barn/outbound-on.conf
--profile-id=barn-linux-testdb-outbound-on --profile-manifest={manifest}"`
and `--oracle-profile-manifest=<barn>/profiles/toast/stock-wsl-testdb.json`.
loadavg 23.47. Result: `24 failed, 2 passed in 319.51s`. Passed:
capability_admission and sqlite_execute_extra_parameters_are_ignored (Barn
already ignored extra parameters; kept as coverage, no Barn change for it).
Failed: every other test in both files.

### Barn checks after the fix, 2026-11-21 (merged tree)

`gofmt -l builtins internal/floattext`: no output. `go vet ./builtins
./internal/floattext`: ok. `staticcheck ./builtins ./internal/floattext`: ok.
`golangci-lint run ./builtins/... ./internal/floattext/...`: `0 issues.`
`go test ./builtins -count=1`: `ok 47.566s`. `go test ./internal/floattext
./types ./parser -count=1`: ok (floattext has no test files).

### floattext change and its coverage

internal/floattext.Format follows Toast's unparse_value float case,
src/list.cc:461-466: `snprintf(buffer, 40, "%.*g", DBL_DIG, v.v.fnum)`, then
".0" appended when the text has neither '.' nor 'e'. Only the non-finite
branch changed: glibc prints infinity as "inf"/"-inf" and NaN as
"nan"/"-nan" (sign bit), so Toast shows "inf.0", "-inf.0", "nan.0",
"-nan.0"; Barn printed "Inf", "-Inf", "NaN". The finite branch (FormatFloat
'g' 15 plus ".0") is unchanged, so no finite float prints differently.
Coverage: before this change no Barn Go test asserted a non-finite spelling
(grep of *_test.go for "Inf"/"NaN" strings found only parser/format_error_test.go,
which tests that non-finite literals are refused when formatting code, a
separate path at parser/format.go:576). Now covered by
TestToastParseNumberAndFloat (builtins/sqlite_toast_parity_test.go) and the
conformance tests sqlite_infinity_prints_as_inf_point_zero and
sqlite_query_parses_infinity_spellings_as_floats. In Barn a MOO float can be
non-finite only from SQLite: tofloat() refuses inf/nan (builtins/types.go:176)
and arithmetic overflow raises E_FLOAT. NaN printing is not pinned against
Toast (no test produces a NaN float on Toast yet; SQLite turns NaN into NULL,
but the text 'nan' from a column would parse to NaN).

## Decisions and why

- Statement execution goes through the raw C API on the modernc connection
  (the same unexported-field access sqlite_readonly.go already uses) instead
  of database/sql, because every item from 2 to 16 is a consequence of
  Toast's exact C calls (sqlite3_exec, prepare_v2 of the first statement,
  column_text). Reproducing them through the driver would mean guessing.
- sqlite_handles order emulates libstdc++'s unordered_map rather than
  sorting descending: descending matches only until the table rehashes at
  the fourteenth handle (observed {15, 14, 1, ..., 13} on Toast). No
  conformance test for more than three handles, because the order depends on
  every sqlite_open since server start and so on test order; the
  fifteen-handle observation is pinned by
  TestSqliteHandleOrderMatchesToastUnorderedMap. Primes above 1109 buckets
  are not from libstdc++'s table (needs 1110 simultaneous handles; Toast's
  default sqlite_max_handles is 20).
- Limits: Barn keeps the values Toast's library would hold and writes them
  into the connection's limit table directly, bounded by modernc's
  compile-time maxima. See "Barn fix" above.
- Item 3 is in its own suite and Barn commits so it can be dropped alone:
  revert "Bind sqlite_execute INT parameters as 32-bit like Toast" and drop
  "Pin sqlite_execute binding INT parameters as 32-bit".

## Surprises

- LIMIT_VARIABLE_NUMBER: sqlite_limit on Barn reports 250000 like Toast, but
  modernc 3.50.4 is compiled with SQLITE_MAX_VARIABLE_NUMBER 32766 and
  SQLite sizes the expression node's variable-number field from that
  constant, so Barn cannot raise the real limit. A statement with more than
  32766 parameters, or a `?NNN` above 32766, works on Toast and fails on Barn
  ("variable number must be between ?1 and ?32766" / too many SQL
  variables). Not pinned by a test; fixing it needs a SQLite build with the
  larger bound.
- modernc 3.50.4 raises any LIMIT_LENGTH below 30 to 30 (SQLITE_MIN_LENGTH);
  3.40.1 keeps values down to 1. Barn bypasses that by writing aLimit
  directly. Whether SQLite 3.50.4 misbehaves anywhere with LIMIT_LENGTH below
  30 is not known; the tests that run with LIMIT_LENGTH 3 pass.
- With LIMIT_LENGTH 3, Toast fails `SELECT 'abc'` because the generated
  column name `'abc'` is 5 bytes, and a too-long bound string silently
  becomes NULL (sqlite.cc ignores bind return codes).
- sqlite_handles iterates a hash table, so its order changes at 14 open
  handles.

### Findings outside the 17, not fixed

- sqlite_limit truncates both the category and the new value to C int
  (sqlite.cc:601, 611: `int category`, `int new_value`), so category
  4294967298 is category 2. Barn uses the 64-bit values. Not tested on Toast.
- Toast refuses sqlite_open with E_QUOTA "Too many database connections
  open." once sqlite_max_handles (default 20, $server_options) handles are
  open (sqlite.cc:95-101, 220-226). Barn has no limit. Not tested.
- Toast's sqlite_close raises E_PERM "Handle can't be closed until all worker
  threads are finished" while a threaded statement runs (sqlite.cc:305-306);
  Barn waits for the statement instead. Not tested.
- REGEXP with an invalid pattern: Toast's message is "PCRE2 compile error at
  offset N: <pcre2 text>"; Barn returns regexp2's error text. Not pinned.

### Generated differential sample: not run

The sampler (scratchpad make_sqlite_sample.py, the #980 agent's sampler with
the filter narrowed to `sqlite_\w+\(`) generated 2136 batches of 2000
random-choice structured programs from this branch's
`cmd/moo-fuzz-source --corpus --generator structured-ascii` and kept none
that called a sqlite_ builtin, so I stopped it. Why the generator's SQLite
programs did not appear is not known (not investigated: internal/fuzzseed
structured.go around the sqliteCall shapes is where to look). The issue's
"1000 generated programs" done-condition is therefore not met by this work.

## Next step

Lead publishes: suite PR first (branch test/sqlite-toast-parity), then the
Barn PR. Then, separately: why the structured generator yields no SQLite
programs from random choices, and the 1000-program differential.

## Suite PR body (draft)

Title: Pin ToastStunt SQLite builtin result shapes (#986)

Adds builtins/sqlite_toast_parity.yaml and builtins/sqlite_execute_int_bind.yaml
for MongooseMoo/barn#986: how ToastStunt's sqlite.cc numbers handles, lists
them, converts column text into MOO values, binds parameters, splits
statements, applies sqlite_open options and limits, handles an idle
sqlite_interrupt, registers REGEXP and prints an infinite float. Results are
compared as toliteral() strings so value types are exact.

Both files pass on stock WSL Toast (03af4fa, 26 passed) and on Mongoose
0ff6fd7 WSL Toast (26 passed) with the focused managed selection including
capability_admission. Barn fails 24 of them before MongooseMoo/barn's fix.

sqlite_execute_int_bind.yaml is a separate commit: Toast binds INT
parameters with sqlite3_bind_int, keeping the low 32 bits, which loses data;
it can be dropped alone.

Limit defaults and the LIMIT_LENGTH behaviour are those of the linked
libsqlite3 (Debian 3.40.1-2+deb12u2 on both oracles).

## Barn PR body (draft)

Title: SQLite builtins: return what Toast returns (#986)

Fixes #986. Barn's sqlite_* builtins now make the C calls Toast's sqlite.cc
makes, on the modernc connection:

- sqlite_query runs every statement with sqlite3_exec; sqlite_execute
  prepares only the first statement. Both read each column as text and
  convert it with Toast's string_to_moo_type / parse_number / parse_float
  (objects, integers, floats incl. hex and inf; RETURNING rows come back).
- Only STR, INT, FLOAT and OBJ parameters are bound; others are NULL.
- sqlite_open's option bits are honoured and reported by sqlite_info.
- REGEXP is registered on each connection.
- sqlite_limit reports Toast's library values (LIMIT_VARIABLE_NUMBER 250000,
  LIMIT_FUNCTION_ARG 127), applies them to the connection, and a non-INT/STR
  category is E_INVARG.
- sqlite_interrupt with nothing running no longer cancels the next statement.
- Handle numbers restart at 1 when all are closed; sqlite_handles uses
  Toast's libstdc++ unordered_map order.
- Infinite and NaN floats print as Toast prints them (inf.0, -inf.0, nan.0,
  -nan.0; list.cc:461-466). Finite floats are unchanged.

The 32-bit INT parameter binding (item 3, loses data like Toast) is its own
commit, "Bind sqlite_execute INT parameters as 32-bit like Toast", so it can
be reverted alone.

Known remaining difference: modernc's SQLite is compiled with a 32766
variable maximum, so a statement with more than 32766 parameters works on
Toast and fails on Barn, although sqlite_limit reports 250000.

Conformance: needs MongooseMoo/moo-conformance-tests branch
test/sqlite-toast-parity (sqlite_toast_parity.yaml, sqlite_execute_int_bind.yaml).
Barn before: 24 failed, 2 passed; after: 26 passed (all items), and 24 passed with only the two item-3 tests failing when the item 3 commit is left out.

Not done: the issue's 1000-program generated SQLite differential (the
generator produced no SQLite programs from random choices; see the note).
