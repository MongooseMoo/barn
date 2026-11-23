# SQLite vector search

## Goal

Upgrade Barn's existing modernc SQLite driver and enable sqlite-vec through the existing SQLite builtins. Keep `CGO_ENABLED=0` builds supported. Deliver the change through Barn's PR checks and merge queue.

## State

The driver and libc are upgraded, and Barn registers sqlite-vec when it starts. The SQLite builtin spec has a worked example. `TestSqliteVectorSearch` opens an in-memory database, creates a `vec0` table, inserts two embeddings, and checks the nearest result through the MOO builtins.

The focused regression passed with `CGO_ENABLED=0` on Windows. The Linux amd64 v3 executable also built with `CGO_ENABLED=0`.

PR #1126 is open. Its first MOO conformance run reported one failure, `sqlite_real_columns_use_sqlite_fifteen_digit_text`, with 14,207 passing cases. The managed ToastStunt run passed that case and `capability_admission`. Barn now sets `SQLITE_DBCONFIG_FP_DIGITS` to 15 when opening each SQLite connection. The regression checks the numeric value from both `sqlite_query` and `sqlite_execute`. That regression and the vector search test passed on Windows with `CGO_ENABLED=0`, and the updated Linux amd64 v3 executable built with `CGO_ENABLED=0`.

## Decisions and why

- Use `modernc.org/sqlite v1.63.0` and its required `modernc.org/libc v1.80.1`. The Go proxy lists that SQLite release, whose module requires Go 1.26.0. Barn requires Go 1.27.1.
- Use the driver's bundled `modernc.org/sqlite/vec` package. Its initialization registers the extension on new connections, including the driver connection opened by `sqlite_open`.
- Cover registration and nearest-neighbor results through the existing SQLite builtins. The existing PR checks also cover SQLite limits, read-only statements, and MOO conformance.

## Surprises

Barn reads the driver's private `db` and `tls` fields and uses the generated SQLite `FaLimit` field. The selected version retains those fields. The regression returned `no such module: vec0` on the previous driver.

The conformance row expected `[1, 1, 0, 0, 1, 1]` and got `[0, 0, 1, 1, 1, 1]`. Upstream SQLite documents that the default REAL-to-text precision changed to 17 digits in SQLite 3.52.0 and exposes the per-connection `SQLITE_DBCONFIG_FP_DIGITS` setting. The managed ToastStunt command includes the failing row and `capability_admission`. Its WSL `git rev-parse` substitution could not resolve a Windows-created worktree's Git metadata. The managed harness itself started the documented Toast executable. Resolve Git metadata on Windows when passing a revision to a WSL evidence context.

## Next step

Follow the updated PR checks and merge through the repository's merge queue once green.
