# SQLite vector search

## Goal

Upgrade Barn's existing modernc SQLite driver and enable sqlite-vec through the existing SQLite builtins. Keep `CGO_ENABLED=0` builds supported. Deliver the change through Barn's PR checks and merge queue.

## State

The driver and libc are upgraded, and Barn registers sqlite-vec when it starts. The SQLite builtin spec has a worked example. `TestSqliteVectorSearch` opens an in-memory database, creates a `vec0` table, inserts two embeddings, and checks the nearest result through the MOO builtins.

The focused regression passed with `CGO_ENABLED=0` on Windows. The Linux amd64 v3 executable also built with `CGO_ENABLED=0`.

## Decisions and why

- Use `modernc.org/sqlite v1.63.0` and its required `modernc.org/libc v1.80.1`. The Go proxy lists that SQLite release, whose module requires Go 1.26.0. Barn requires Go 1.27.1.
- Use the driver's bundled `modernc.org/sqlite/vec` package. Its initialization registers the extension on new connections, including the driver connection opened by `sqlite_open`.
- Cover registration and nearest-neighbor results through the existing SQLite builtins. The existing PR checks also cover SQLite limits, read-only statements, and MOO conformance.

## Surprises

Barn reads the driver's private `db` and `tls` fields and uses the generated SQLite `FaLimit` field. The selected version retains those fields. The regression returned `no such module: vec0` on the previous driver.

## Next step

Follow the PR checks and merge through the repository's merge queue once green.
