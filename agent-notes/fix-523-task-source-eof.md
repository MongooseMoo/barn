# #523: task source EOF

## Goal
Stop missing source terminators at EOF without changing accepted nonempty final lines.

## State
Base c537d6c952791b2fe49bcb9d7cba90102767a36f, branch fix/523-task-source-eof. Implemented one helper condition/comment change and focused coverage in two existing test files. Local only, no publication.

## Decisions and why
Smallest change: one condition and a comment in readLine, approximately four production lines. Return EOF when ReadString has no bytes left; retain its existing acceptance of a nonempty final unterminated line. Every caller in reader_task.go, reader_vm.go and reader_v4/v5.go checks the error. The queued and VM source loops already wrap it with source context. No caller requires invented empty lines after physical EOF.
Extend existing reader_lines_test.go and reader_task_roundtrip_test.go with final-line behavior and the tracked toastcore first-task truncation. Bound the failing-implementation probe by replacing a second EOF with an error, rather than a timer or unbounded goroutine.

## Surprises
None yet.

## Next step
Parent reviews and publishes this commit/PR; worker proceeds to #524 independently. No full CI suite run locally.

## PR draft
Title: fix(db/format): return EOF after the final task source line

A truncated queued or suspended program could repeatedly append empty lines because readLine suppressed EOF even after all bytes were consumed. Return EOF in that case while preserving the existing acceptance of nonempty final lines without a newline. Both source readers already wrap the error with their context.

Focused verification uses the tracked toastcore fixture's first queued program and a VM frame emitted by the existing writer with that source. Truncating before the required dot now returns contextual EOF. Final non-newline text/dot lines and blank lines retain their previous behavior. Closes #523.

## Evidence

All production readLine callers were inspected via `rg -n -B 1 -A 4 'readLine\(r\)' db/format --glob '*.go' --glob '!**/*_test.go'`: reader_task (source, placeholders, verb aliases, environment names, builtin names, interrupted headers), reader_vm (headers, source, stack metadata, names, PCs) and v4/v5 dummy fields all check errors. No callsite change was needed.

```text
go test ./db/format -run '^(TestReadLinePreservesEOFWithoutLosingFinalLine|TestTaskSourceTruncationReturnsEOF)$' -v
=== RUN   TestReadLinePreservesEOFWithoutLosingFinalLine
--- PASS: TestReadLinePreservesEOFWithoutLosingFinalLine (0.00s)
=== RUN   TestTaskSourceTruncationReturnsEOF
tracked fixture truncated at 14897 bytes: read queued tasks: read queued task 0: read queued task source: EOF
writer frame truncated at 333 bytes: read program: EOF
--- PASS: TestTaskSourceTruncationReturnsEOF (0.29s)
PASS
ok github.com/MongooseMoo/barn/db/format 1.735s
```

The complete original fixture is parsed before truncation in the focused test. Input identity is the tracked toastcore.db from #516 (SHA256 8013b703c61a9894866f836f2b934eada7118cdf0b3cd56181e4bf9205b2f557). This is malformed-input termination evidence, not a native saved-task resumption claim or an all-format compatibility verdict.
