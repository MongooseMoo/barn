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

Started 2026-11-20. Reading issue and sources.

Resumed 2026-11-21 by a new agent (previous agent hit a usage limit after the skeleton).

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

## Decisions and why

## Surprises

## Next step

Read issue #986, CLAUDE.md, AGENTS.md, plan, builtins/sqlite.go, Toast sqlite.cc.
