# db/reader.go split report

## Summary

`db/reader.go` (1879 lines, 41 top-level declarations) split into 8 topical files in the same package. Pure relocation; no signature or body changes.

## Final line counts

| File | Lines |
|---|---|
| db/reader.go | 124 |
| db/reader_v4.go | 317 |
| db/reader_v5.go | 83 |
| db/reader_v17.go | 105 |
| db/reader_object.go | 362 |
| db/reader_value.go | 344 |
| db/reader_task.go | 367 |
| db/reader_helpers.go | 243 |
| **Total** | **1945** |

Net delta vs. original 1888 lines = +57 lines, accounted for by the 7 new `package db` + import blocks added across the new files.

## Function placement

Every function listed in the target layout was found in the original file and placed in the file the prompt specified. No extras, no omissions. All 41 declarations from the original file are present exactly once across the 8 files (verified by grep over `^(func|type)\b`).

`reader.go` retains the four type declarations (`Database`, `waifLoadData`, `QueuedTask`, `SuspendedTask`) plus `NewStoreFromDatabase`, `LoadDatabase`, `parseDatabase`, `recordStartupRepair`.

## Imports per file

- `reader.go`: `barn/types`, `bufio`, `fmt`, `log`, `os`, `strings`
- `reader_v4.go`: `barn/types`, `bufio`, `fmt`, `strconv`, `strings`
- `reader_v5.go`: `bufio`, `fmt`
- `reader_v17.go`: `barn/types`, `bufio`, `fmt`
- `reader_object.go`: `barn/types`, `bufio`, `fmt`, `strconv`, `strings`
- `reader_value.go`: `barn/types`, `bufio`, `fmt`, `strconv`, `strings`
- `reader_task.go`: `barn/types`, `bufio`, `fmt`, `strconv`, `strings`
- `reader_helpers.go`: `barn/types`, `bufio`, `fmt`, `io`, `strconv`, `strings`

## Verification

```
go build ./db/...   # clean
go vet ./db/...     # clean
go test ./db/...    # ok  barn/db  1.634s
```

(`go build ./...` has pre-existing failures in `barn/builtins` from concurrent registry-lift work; out of scope for this workstream which only touches `db/`. Baseline `go test ./db/...` was clean before the split and remains clean after.)

## Commit

(filled in below after commit)
