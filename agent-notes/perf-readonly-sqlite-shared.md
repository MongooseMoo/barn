# perf/readonly-sqlite-shared

## Goal

An inline (thread mode 0) SQLite statement that SQLite reports as read-only
must not take the exclusive commit gate or end the attempt's retryability.
Mongoose runs one such SELECT on every `say`
(`#6128:lookup_cache_version`: total_changes, pragma_data_version,
pragma_schema_version), and the gate it took serialized the server for the
rest of each say slice. Writing statements keep the old behaviour exactly.
Candidate for the goodput campaign; it is measured before it is proposed.

## State

- Implemented and unit-tested; real-workload measurements: see Next step and
  the campaign progress file.
- `builtins/sqlite_readonly.go`: `sqliteStatementShareable` asks SQLite. Every
  statement in the text must prepare, `sqlite3_stmt_readonly` must be true,
  the connection must be in autocommit mode, and the authorizer must report no
  transaction, savepoint, attach, detach or pragma action.
- `builtins/sqlite.go`: an optimistic attempt (boundary machinery present, not
  past it, not holding the gate) runs a shareable statement at once and
  records the store's external write epoch; anything else takes the boundary
  as before. Every statement that is not shareable, inline or threaded, moves
  the epoch after it runs, before the handle is released.
- `db/store/store_external_reads.go`: the epoch, `NoteExternalRead`, and its
  validation (conflict kind `external`). `HasWrites` is true for a
  transaction with an external read, so every commit site validates it; a
  transaction with no object writes validates all its reads in that case
  (`commitTaskRowsOnly`). Hot-read renewal carries the external read.

## Decisions and why

- The guarantee: a task that read SQLite optimistically commits only if no
  non-read-only SQLite statement finished between its read and its commit's
  validation; it is then serialized at that validation, where its store reads
  are also checked. The epoch moves after a write is made, and the reader
  loads it before reading, so a reader that overlapped a write fails whether
  or not it saw the write. A task writer holds the exclusive gate from before
  its write to its commit, so a reader that saw the write validates after the
  writer's store commit.
- Ask SQLite, not the text. `sqlite3_stmt_readonly` is true for BEGIN,
  COMMIT, ROLLBACK, SAVEPOINT, RELEASE, ATTACH and DETACH
  (https://www.sqlite.org/c3ref/stmt_readonly.html), which change connection
  state that a retried attempt would change again and other tasks see; a read
  in an open transaction can see rows a ROLLBACK removes. Hence the
  autocommit and authorizer conditions. Any pragma is treated as a writer; the
  pragma table-valued functions in the Mongoose statement do not raise the
  pragma action, so it stays shareable.
- modernc.org/sqlite v1.39.1 exports none of these calls and no raw handle.
  The classifier reads the driver connection's unexported `db` and `tls`
  fields by reflection inside `sql.Conn.Raw` and calls `modernc.org/sqlite/lib`
  directly; if the fields are missing it answers "not shareable" (today's
  behaviour) and `TestSQLiteStatementShareable` goes red. `modernc.org/libc`
  became a direct requirement in go.mod.
- One epoch for the whole runtime, not per handle: ATTACH and two spellings of
  one path let different handles reach one file.
- Readers take no gate at all. A reader holding the handle while waiting for
  the gate, or holding the gate while waiting for the handle, can deadlock
  against a writer that holds the exclusive gate and waits for the handle.
  The epoch plus commit validation orders readers without either.
- The boundary validates an external read but the renewed transaction does
  not carry it: after the boundary the attempt holds the gate, and a threaded
  write (which takes no gate) would otherwise fail the final commit after the
  irreversible effect, an uncatchable E_INVARG. The same is why an attempt
  holding the gate records no external read.
- Writes by processes outside Barn move no epoch; out of scope, as they were
  for the gate.
- A transaction whose only "write" is an external read now makes
  `HasWrites` true, so `load_server_options` in such a task defers its
  session-wide swap to the commit, as it does after a real write.

## Surprises

- Passing Go memory as the C out-parameters of `sqlite3_prepare_v2` gave
  wrong results (INSERT classified read-only); SQLite-allocated memory fixed
  it.
- `TestRenewalAccountsForEveryTxnField` caught that hot-read renewal had to
  carry the new fields.

## Next step

Real-workload measurement, 16 players, 3 runs before and after plus one census
run each; results in the campaign progress file
(`scratchpad/plan/fix-readonly-sqlite.md`). Not pushed; no PR.
