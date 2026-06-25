# Fix F5 — sqlite_open escaped the file sandbox

## The bug
`builtins/sqlite.go` `builtinSqliteOpen` called `sanitizeFilePath()` (the path
*verify* step) but never `resolveFilePath()` (the sandbox-*prefix* step). A
named SQLite database was therefore created at a CWD-relative path instead of
inside the `files/` sandbox that every `fileio` builtin enforces.

## What `resolveFilePath` confines to, and how fileio uses it
- `builtins/fileio.go:56` — `resolveFilePath(rel) = filepath.Join("files", rel)`.
  It prepends the `files/` sandbox root to an already-verified relative path.
- `builtins/fileio.go:64` — `sanitizeFilePath` is the *verify* step: rejects
  absolute paths, `..` traversal, and `/.`; returns a cleaned relative path or
  an error (callers map that error to `E_INVARG`).
- Every fileio builtin pairs them: verify with `sanitizeFilePath`, then open the
  result of `resolveFilePath(sanitized)` (e.g. `builtinFileOpen`
  fileio.go:202-214, `builtinFileStat`/`builtinFileRemove`/`builtinFileList`,
  etc.). sqlite did the verify but skipped the prefix.

## Toast's sandbox behavior (authority)
- `toaststunt/src/sqlite.cc:241` — `bf_sqlite_open` resolves the DB path via
  `path = file_resolve_path(unresolved_path);` and raises `E_INVARG` when it
  returns NULL (sqlite.cc:242-246). `:memory:` and `""` skip resolution
  (sqlite.cc:233-236).
- `toaststunt/src/fileio.cc:318-335` — `file_resolve_path` does BOTH: calls
  `file_verify_path` (NULL => caller raises) AND prepends `file_subdir`,
  stripping one leading `/` (fileio.cc:327-331). So Barn's `sanitizeFilePath`
  == `file_verify_path`, and Barn's `resolveFilePath` == the `file_subdir`
  prefix. sqlite must use both, exactly as fileio does.

## sqlite path entry points confined
`sqlite_open` is the ONLY sqlite builtin that takes a filesystem path; all
others (`sqlite_close/query/execute/info/handles/limit/interrupt/
last_insert_row_id`) operate on integer handles. Fix applied to
`builtinSqliteOpen` (builtins/sqlite.go): after `sanitizeFilePath`, call
`ensureFilesRoot()` then `resolveFilePath(sanitized)` (skipped for `:memory:`,
matching Toast). The stored `handle.path` is now the resolved sandbox path,
matching Toast's `handle->path = str_dup(path)` (sqlite.cc:279).

## Tests
- `go test ./builtins/ -run TestReview_IO_SqliteSandboxEscape -v` => PASS.
- Added `TestSqliteOpenRefusesSandboxEscape` (builtins/compat_sqlite_test.go):
  `sqlite_open("../escape.db")` and `"../../etc/escape.db"` => `E_INVARG`,
  matching fileio / Toast. (A leading-slash path like `/etc/x.db` is *confined*,
  not rejected — Toast strips the leading `/` and sandboxes it — so it is
  correctly excluded from the escape set.)

## Before / after builtins failure list
No new failures. The only change: `TestReview_IO_SqliteSandboxEscape` went
FAIL -> PASS. Still-red (pre-existing intentional review tests for other
findings, unchanged): TestReview_AddVerbUsesProgNotPlayerForPerm,
TestReview_VerbCodeAllowsOwnerWithoutReadBit,
TestReview_Data_AbsMinInt64Overflow, _CapitalizeDeprecatedTitle,
_IsMemberStrCaseSensitiveBug, _PcreMatchEmptySubject, _SetaddUniqueConsistency,
_SortReverseIgnored, _UniqueStrCaseInsensitive,
TestReview_IO_CryptSHA256RoundsSilentlyCapped, _FileReadlinesBinaryMode,
_QueuedTasksSortOrder.

`go vet ./builtins/` clean.

## Commit
`1c6b2138c0476d4cdeb8586c1e1e257353382c47` on branch
`review/branch-stocktake-2026-06-25`.
