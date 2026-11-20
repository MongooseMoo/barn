# File scan cursor correctness

## Goal

Preserve file_grep's first-match cursor at the matched line instead of scanner
read-ahead EOF, and prevent file_count_lines/file_grep cursor changes before an
irreversible-effect boundary succeeds.

## State

Read existing file I/O, registered-effect wrappers and refusal regressions.
Parent supplied managed stock-oracle evidence and the matching Barn first-match
failure. New focused regressions in builtins/fileio_readline_buffer_test.go
reproduce both cursor read-ahead and missing irreversible admission. Production
edits waited for the parent to verify long-line behavior of the selected
standard reader. The managed stock result now records five passes including
admission, with no failures/skips in toast-file-scan-v3.xml. A 65,536-byte line
is counted and returned whole; first grep leaves the next line unread. Parent
approved direct buffered-fragment counting without preserving Scanner's token
limit. The scoped correction is implemented and the expanded focused
regressions now pass. Other local changes are preserved.

RED command inside verified Debian WSL from this worktree:

```text
go test ./builtins -run '^(TestFileGrepFirstMatchPreservesNextLine|TestFileScansRewindAndConsumeToEOF|TestFileScansDoNotRunWhenEffectBoundaryRefuses)$' -count=1
```

Raw RED excerpts:

```text
--- FAIL: TestFileGrepFirstMatchPreservesNextLine/r-tf
fileio_readline_buffer_test.go:60: result = {19 0 E_NONE unknown  <nil> []}, want 13
--- FAIL: TestFileGrepFirstMatchPreservesNextLine/r-bf
fileio_readline_buffer_test.go:60: result = {19 0 E_NONE unknown  <nil> []}, want 13
--- FAIL: TestFileScansDoNotRunWhenEffectBoundaryRefuses/file_count_lines
fileio_readline_buffer_test.go:108: file_count_lines = {Val:3 Flow:0 Error:E_NONE Reason:unknown Label: ForkInfo:<nil> CallStack:[]}, crossings=0 irreversible=false, want aborted before effect
--- FAIL: TestFileScansDoNotRunWhenEffectBoundaryRefuses/file_grep
fileio_readline_buffer_test.go:108: file_grep = {Val:{{"needle", 2}} Flow:0 Error:E_NONE Reason:unknown Label: ForkInfo:<nil> CallStack:[]}, crossings=0 irreversible=false, want aborted before effect
FAIL github.com/MongooseMoo/barn/builtins 0.133s
```

## Decisions and why

Use a standard buffered reader in the existing grep owner and restore unread
buffered bytes on an early match. Preserve all/no-match EOF behavior and current
case matching. Mark only the two existing cursor-mutating descriptors as
Irreversible; add no stream cache, wrapper, worker or alternate API.

Read the actual standard library documentation for ReadSlice and ReadString.
Count uses buffer-backed fragments and one pending-line boolean, counting only
delimiters and a final unterminated line without allocating a string per line.
ErrBufferFull continues the same line; native other errors remain E_FILE.
Grep uses ReadString and the same trailing LF/CR removal as ScanLines, restoring
reader.Buffered bytes with the existing file Seek before a first-match return.

Additional focused RED command:

```text
go test ./builtins -run '^(TestFileGrepFirstMatchPreservesNextLine|TestFileScansRewindAndConsumeToEOF|TestFileCountLinesHandlesFragmentsAndFinalLines|TestFileGrepReadsLongMatchedLineWithoutConsumingTail|TestFileScansDoNotRunWhenEffectBoundaryRefuses)$' -count=1
```

Raw additional RED excerpts:

```text
--- FAIL: TestFileCountLinesHandlesFragmentsAndFinalLines/long_unterminated
fileio_readline_buffer_test.go:106: result = {None 4 E_FILE unknown  <nil> []}, want 1
--- FAIL: TestFileCountLinesHandlesFragmentsAndFinalLines/long_terminated
fileio_readline_buffer_test.go:106: result = {None 4 E_FILE unknown  <nil> []}, want 1
--- FAIL: TestFileCountLinesHandlesFragmentsAndFinalLines/long_with_tail
fileio_readline_buffer_test.go:106: result = {None 4 E_FILE unknown  <nil> []}, want 2
--- FAIL: TestFileGrepReadsLongMatchedLineWithoutConsumingTail
fileio_readline_buffer_test.go:117: long-line file_grep = {Val:None Flow:4 Error:E_FILE Reason:unknown Label: ForkInfo:<nil> CallStack:[]}, want one match
FAIL github.com/MongooseMoo/barn/builtins 0.232s
```

GREEN output from the identical expanded focused selector:

```text
gofmt_exit=0
diff_check_exit=0
ok github.com/MongooseMoo/barn/builtins 0.247s
```

Regressions cover first-match cursor and next read in existing text/binary
modes, initial rewind, all/no-match EOF, count rewind, empty input, trailing
newline, final unterminated lines, exact-reader-buffer-multiple fragments,
65,536-byte lines, and rejected irreversible boundaries leaving cursor/data
untouched. No case-matching policy or mode filtering change was introduced.
Only file_count_lines and file_grep descriptors changed effect policy.
No measured performance, post-fix managed oracle or isolation claim is made.

## Next step

Return the two-file scoped diff for parent review, managed post-fix oracle and
independent PR CI/Elle. No local broad tests, build, git action or publication
has been performed.
