# Fix F26 — file_readlines ignores binary mode

**Finding:** `reports/review-builtins-io.md` M1. `builtinFileReadlines`
(`builtins/fileio.go`) read lines with `bufio.Scanner` + `scanner.Text()`
unconditionally, ignoring the handle's mode. The other fileio readers
(`file_read`, `file_readline`) branch on `h.binary`: binary →
`encodeBinaryBytes` (`~XX`), text → `filterTextMode`. `file_readlines` skipped
both, so binary-mode lines were returned raw instead of `~XX`-encoded.

## Toast authority (source)

- `src/fileio.cc:578` `bf_file_readlines` slurps each line via `file_get_line`
  (which is `getline()`, `src/fileio.cc:491`) and stores
  `str_dup((type->in_filter)(line, len))` (`src/fileio.cc:631`). `getline`
  keeps the trailing `\n`, so the filter sees the newline.
- `file_readline` uses the identical filter on the same getline output
  (`src/fileio.cc:537`).
- The `in_filter` is set per file type at `src/fileio.cc:181-183`:
  binary → `raw_bytes_to_binary`, text → `raw_bytes_to_clean`.
- `src/utils.cc:672` `stream_add_raw_bytes_to_binary`: for each byte, if
  `c != '~' && (isgraph(c) || c == ' ')` emit it literally, else
  `stream_printf(s, "~%02X", c)`. So non-printables — **including `\n` (0x0A)**
  — become `~XX`.
- `src/utils.cc:636` `stream_add_raw_bytes_to_clean` (text): keeps only
  `isgraph(c) || c == ' '`, drops everything else (incl. `\n`).

**Oracle (committed Toast test):**
`test/tests/test_fileio.rb:171` `test_that_reading_text_in_binary_mode_is_ok`
reads `"one two three four five\n"` in binary mode and asserts
`'one two three four five~0A'`. This proves the trailing `\n` is `~0A`-encoded
in binary mode. `file_readlines` uses the same filter on the same getline
output, so binary lines carry the encoded newline too.

## How Barn's other readers do it

`builtinFileRead` (`fileio.go`): `if h.binary { encodeBinaryBytes(data) } else
{ filterTextMode(data) }`. `builtinFileReadline`: `encodeBinaryBytes(buf)` for
binary (buf includes the `\n`), `filterTextMode(TrimRight(buf,"\r\n"))` for text.

## The fix

Replaced the `bufio.Scanner` loop in `builtinFileReadlines` with a
`bufio.NewReader` + `ReadBytes('\n')` loop (getline semantics — keeps the
trailing newline, and handles a final line with no newline). Each selected line
is passed through the same mode-dependent transform as the other readers:
binary → `encodeBinaryBytes`, text → `filterTextMode`. Line-range selection
(`start`/`end`), `E_INVARG`/`E_TYPE`/`E_FILE` errors, and the
seek-save/seek-restore behavior are preserved.

## Test contract

`builtins/review_io_test.go` `TestReview_IO_FileReadlinesBinaryMode` writes
`"ab\x01c\nline2\n"` and reads line 1 in both modes:

- **binary (`r-bf`)** → `"ab~01c~0A"` (0x01 → `~01`, trailing `\n` → `~0A`),
  matching Toast.
- **text (`r-tf`)** → `"abc"` (non-printables dropped), matching Toast.

> Correction vs. scout draft: the scout (no oracle; WSL down) asserted binary
> `"ab~01c"`. That omits the trailing-newline encoding. Per Toast's committed
> `test_fileio.rb:171`, binary mode `~0A`-encodes the newline, so the correct
> value is `"ab~01c~0A"`. Either way the assertion fails against the old
> `scanner.Text()` code (which stripped the newline and skipped encoding).

## Gate output

- `go test ./builtins/ -run 'FileReadlines|FileIO|FileRead' -v` → PASS.
- `go vet ./builtins/` → clean.
- `go test ./builtins/...` → only pre-existing, unrelated red tests fail
  (`TestReview_Data_IsMemberStrCaseSensitiveBug`,
  `TestReview_Data_PcreMatchEmptySubject`, `TestReview_IO_QueuedTasksSortOrder`
  = H4); no new failures.

### Before / after (F26 test)

- BEFORE (old `scanner.Text()`): `FAIL` —
  `file_readlines in binary mode returned "ab\x01c", want "ab~01c~0A"`.
- AFTER (fix): `PASS`.

## Commit

`eaaa40f2018a63e5cb9557819cfd957626901a50` on
`review/branch-stocktake-2026-06-25`.
